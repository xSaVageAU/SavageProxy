package proxy

import (
	"bytes"
	"crypto/rsa"
	"time"

	mcnet "github.com/Tnze/go-mc/net"
)

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

// SendMessage sends a system chat message to the player using native packet construction.
// This bypasses the backend server entirely.
func (s *Session) SendMessage(message string) {
	buf := new(bytes.Buffer)
	
	// System Chat Message (26.1+)
	// Content: String (JSON)
	// Overlay: Boolean (false = chat box, true = action bar)
	
	jsonMsg := `{"text":"` + message + `"}`
	WriteString(buf, jsonMsg)
	buf.WriteByte(0) // false
	
	// Packet ID 0x77 (System Chat Message - Clientbound in 26.1+)
	packetData := EncodePacket(0x77, buf.Bytes())
	
	// We use the underlying Socket to write raw bytes
	s.Conn.Socket.Write(packetData)
}
