package proxy

import (
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"savage-proxy/internal/protocol"

	"github.com/Tnze/go-mc/net/CFB8"
	"github.com/Tnze/go-mc/net/packet"
)

// HandleLogin implements the Mojang Auth & Encryption handshake.
// It returns nil on success, allowing the runner to transition to Bridge mode.
func (s *Session) HandleLogin() error {
	// 1. Receive Login Start (0x00)
	var p packet.Packet
	if err := s.Conn.ReadPacket(&p); err != nil {
		return fmt.Errorf("failed to read login start: %v", err)
	}
	if p.ID != 0x00 {
		return fmt.Errorf("expected LoginStart (0x00), got %X", p.ID)
	}

	var (
		clientName packet.String
		clientUUID packet.UUID
	)
	if err := p.Scan(&clientName, &clientUUID); err != nil {
		return fmt.Errorf("failed to scan login start: %v", err)
	}
	s.Player.Name = string(clientName)
	s.Player.UUID = fmt.Sprintf("%x", clientUUID)
	log.Printf("[%s] %s (%s) is attempting to login", s.Conn.Socket.RemoteAddr(), s.Player.Name, s.Player.UUID)

	// 2. Send Encryption Request (0x01)
	verifyToken := make([]byte, 4)
	rand.Read(verifyToken)

	pubKeyDER, _ := x509.MarshalPKIXPublicKey(&s.PrivKey.PublicKey)
	
	err := s.Conn.WritePacket(packet.Marshal(0x01,
		packet.String(""), // Server ID
		packet.ByteArray(pubKeyDER),
		packet.ByteArray(verifyToken),
		packet.Boolean(true), // ShouldAuthenticate
	))
	if err != nil {
		return fmt.Errorf("failed to send encryption request: %v", err)
	}

	// 3. Receive Encryption Response (0x01)
	if err := s.Conn.ReadPacket(&p); err != nil {
		return fmt.Errorf("failed to read encryption response: %v", err)
	}
	if p.ID != 0x01 {
		return fmt.Errorf("expected EncryptionResponse (0x01), got %X", p.ID)
	}

	var (
		sharedSecretEnc []byte
		verifyTokenEnc  []byte
	)
	if err := p.Scan((*packet.ByteArray)(&sharedSecretEnc), (*packet.ByteArray)(&verifyTokenEnc)); err != nil {
		return fmt.Errorf("failed to scan encryption response: %v", err)
	}

	verifyTokenDec, err := rsa.DecryptPKCS1v15(rand.Reader, s.PrivKey, verifyTokenEnc)
	if err != nil || string(verifyTokenDec) != string(verifyToken) {
		return errors.New("verify token mismatch")
	}

	sharedSecret, err := rsa.DecryptPKCS1v15(rand.Reader, s.PrivKey, sharedSecretEnc)
	if err != nil {
		return fmt.Errorf("failed to decrypt shared secret: %v", err)
	}

	// 4. Verify Session with Mojang
	hash := sha1.New()
	hash.Write([]byte(""))
	hash.Write(sharedSecret)
	hash.Write(pubKeyDER)
	serverHash := protocol.MinecraftHash(hash.Sum(nil))

	if err := s.verifyWithMojang(serverHash); err != nil {
		return fmt.Errorf("mojang verification failed: %v", err)
	}

	// 5. Enable Encryption
	block, err := aes.NewCipher(sharedSecret)
	if err != nil {
		return err
	}
	s.Conn.SetCipher(CFB8.NewCFB8Encrypt(block, sharedSecret), CFB8.NewCFB8Decrypt(block, sharedSecret))

	log.Printf("[%s] Encryption enabled for %s (UUID: %s)", 
		s.Conn.Socket.RemoteAddr(), s.Player.Name, s.Player.UUID)

	return nil
}

func (s *Session) verifyWithMojang(hash string) error {
	url := fmt.Sprintf("https://sessionserver.mojang.com/session/minecraft/hasJoined?username=%s&serverId=%s",
		s.Player.Name, hash)
	
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("not authenticated (status %d)", resp.StatusCode)
	}

	var profile struct {
		ID         string     `json:"id"`
		Name       string     `json:"name"`
		Properties []Property `json:"properties"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return err
	}

	s.Player.UUID = profile.ID
	s.Player.Name = profile.Name
	s.Player.Properties = profile.Properties
	return nil
}

// ParseUUID converts a Mojang-style UUID string (with or without dashes) into bytes.
func (s *Session) ParseUUID(s_uuid string) [16]byte {
	var uuid [16]byte
	data, err := hex.DecodeString(strings.ReplaceAll(s_uuid, "-", ""))
	if err != nil || len(data) != 16 {
		log.Printf("Warning: Failed to parse UUID %s: %v", s_uuid, err)
		return uuid
	}
	copy(uuid[:], data)
	return uuid
}
