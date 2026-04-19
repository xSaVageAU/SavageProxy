package proxy

import (
	"bytes"
	"crypto/rsa"
	"log"
	"sync"
	"time"

	"savage-proxy/internal/protocol"

	mcnet "github.com/Tnze/go-mc/net"
	"github.com/Tnze/go-mc/net/packet"
)

// SendMessage sends a system chat message to the player using native packet construction.
func (s *Session) SendMessage(message string) {
	log.Printf("[%s] Sending proxy message: %s", s.Conn.Socket.RemoteAddr(), message)
	buf := new(bytes.Buffer)

	// TAG_Compound (Root)
	buf.WriteByte(0x0A)
	// TAG_String "text"
	buf.WriteByte(0x08)
	buf.Write([]byte{0x00, 0x04})
	buf.Write([]byte("text"))

	msgBytes := []byte(message)
	buf.WriteByte(byte(len(msgBytes) >> 8))
	buf.WriteByte(byte(len(msgBytes) & 0xFF))
	buf.Write(msgBytes)

	buf.WriteByte(0x00) // TAG_End
	buf.WriteByte(0)    // Overlay: false

	s.WriteClient(packet.Packet{
		ID:   protocol.CB_SYSTEM_CHAT,
		Data: buf.Bytes(),
	})
}

const (
	HandshakeTimeout = 10 * time.Second
)

type Session struct {
	Conn            *mcnet.Conn
	CreatedAt       time.Time
	State           int32 // 0: Handshaking, 1: Status, 2: Login
	ProtocolVersion int32

	// Backend Data
	BackendConn      *mcnet.Conn
	ForwardingSecret string

	// Auth Data
	PrivKey *rsa.PrivateKey
	Player  struct {
		Name       string
		UUID       string
		Properties []Property
	}

	// Internal
	WriteMutex sync.Mutex

	LastTabRequestID   int
	LastTabRequestText string
}

type Property struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Signature string `json:"signature"`
}

func NewSession(conn *mcnet.Conn, privKey *rsa.PrivateKey) *Session {
	return &Session{
		Conn:      conn,
		CreatedAt: time.Now(),
		State:     0,
		PrivKey:   privKey,
	}
}

func (s *Session) Close() error {
	if s.BackendConn != nil {
		s.BackendConn.Close()
	}
	return s.Conn.Close()
}

// WriteClient sends a packet to the player in a thread-safe manner.
func (s *Session) WriteClient(p packet.Packet) error {
	s.WriteMutex.Lock()
	defer s.WriteMutex.Unlock()
	return s.Conn.WritePacket(p)
}

