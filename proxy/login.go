package proxy

import (
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net/http"
	"encoding/json"

	"github.com/Tnze/go-mc/net/CFB8"
	"github.com/Tnze/go-mc/net/packet"
)

// HandleLogin implements the Mojang Auth & Encryption handshake
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
	// In 1.20.2+, LoginStart contains both Name and UUID
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
		packet.String(""), // Server ID (usually empty)
		packet.ByteArray(pubKeyDER),
		packet.ByteArray(verifyToken),
		packet.Boolean(true), // ShouldAuthenticate (Required since 1.20.5+)
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

	// Decrypt the verify token and check it
	verifyTokenDec, err := rsa.DecryptPKCS1v15(rand.Reader, s.PrivKey, verifyTokenEnc)
	if err != nil || string(verifyTokenDec) != string(verifyToken) {
		return errors.New("verify token mismatch")
	}

	// Decrypt the shared secret
	sharedSecret, err := rsa.DecryptPKCS1v15(rand.Reader, s.PrivKey, sharedSecretEnc)
	if err != nil {
		return fmt.Errorf("failed to decrypt shared secret: %v", err)
	}

	// 4. Verify Session with Mojang
	// We need the "Server Hash" = SHA1(ServerID + SharedSecret + PublicKey)
	hash := sha1.New()
	hash.Write([]byte("")) // Server ID
	hash.Write(sharedSecret)
	hash.Write(pubKeyDER)
	serverHash := MinecraftHash(hash.Sum(nil))

	if err := s.verifyWithMojang(serverHash); err != nil {
		return fmt.Errorf("mojang verification failed: %v", err)
	}

	// 5. Enable Encryption
	block, err := aes.NewCipher(sharedSecret)
	if err != nil {
		return err
	}
	// Initial Vector (IV) for CFB8 is the shared secret itself
	s.Conn.SetCipher(CFB8.NewCFB8Encrypt(block, sharedSecret), CFB8.NewCFB8Decrypt(block, sharedSecret))

	log.Printf("[%s] Encryption enabled for %s (UUID: %s)", 
		s.Conn.Socket.RemoteAddr(), s.Player.Name, s.Player.UUID)

	// 6. Send Login Success (0x02)
	// In modern versions, this contains UUID, Name, and Property count
	err = s.Conn.WritePacket(packet.Marshal(0x02,
		packet.UUID(s.parseUUID(s.Player.UUID)),
		packet.String(s.Player.Name),
		packet.VarInt(0), // Properties count
	))
	if err != nil {
		return fmt.Errorf("failed to send login success: %v", err)
	}

	return nil
}

// MinecraftHash implements Mojang's "Two's Complement" SHA-1 hex format
func MinecraftHash(data []byte) string {
	var negative bool
	if data[0] >= 0x80 {
		negative = true
		// Two's complement: flip bits and add 1
		carry := true
		for i := len(data) - 1; i >= 0; i-- {
			data[i] = ^data[i]
			if carry {
				data[i]++
				carry = (data[i] == 0)
			}
		}
	}
	res := fmt.Sprintf("%x", data)
	res = fmt.Sprintf("%040s", res) // Pad with zeros if needed (though %x usually doesn't)
	// Trim leading zeros like Mojang does
	for len(res) > 0 && res[0] == '0' {
		res = res[1:]
	}
	if negative {
		res = "-" + res
	}
	return res
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
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return err
	}

	s.Player.UUID = profile.ID
	s.Player.Name = profile.Name
	return nil
}

func (s *Session) parseUUID(s_uuid string) [16]byte {
	// Simple UUID parser (Mojang returns UUID without dashes)
	var uuid [16]byte
	fmt.Sscanf(s_uuid, "%32x", &uuid) // Might not be perfect but ok for placeholder
	return uuid
}
