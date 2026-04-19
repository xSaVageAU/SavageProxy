package proxy

import (
	"bytes"
	"crypto/rsa"
	"sync"
	"time"

	"savage-proxy/internal/protocol"

	mcnet "github.com/Tnze/go-mc/net"
	"github.com/Tnze/go-mc/net/packet"
)

const (
	HandshakeTimeout = 10 * time.Second
)

type Session struct {
	Conn            *mcnet.Conn
	CreatedAt       time.Time
	State           int32
	ProtocolVersion int32

	// Multi-Backend Management
	backendMu   sync.RWMutex
	backendConn *mcnet.Conn
	backendEID  int32 // Current entity ID on backend

	ForwardingSecret string

	// Auth Data (Our "Mimicry Passport")
	PrivKey *rsa.PrivateKey
	Player  struct {
		Name       string
		UUID       string
		Properties []Property
	}

	// Internal
	closeOnce   sync.Once
	
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

// GetBackend uniquely retrieves the current active backend connection.
func (s *Session) GetBackend() *mcnet.Conn {
	s.backendMu.RLock()
	defer s.backendMu.RUnlock()
	return s.backendConn
}

// SetBackend atomicially updates the active backend server.
func (s *Session) SetBackend(conn *mcnet.Conn) {
	s.backendMu.Lock()
	defer s.backendMu.Unlock()
	s.backendConn = conn
}

func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		s.backendMu.Lock()
		if s.backendConn != nil {
			s.backendConn.Close()
		}
		s.backendMu.Unlock()
		s.Conn.Close()
	})
	return nil
}

func (s *Session) WriteClient(p packet.Packet) error {
	return s.Conn.WritePacket(p)
}

func (s *Session) SendMessage(message string) {
	buf := new(bytes.Buffer)
	buf.WriteByte(0x0A) // Root
	buf.WriteByte(0x08) // String
	buf.Write([]byte{0x00, 0x04})
	buf.Write([]byte("text"))
	msgBytes := []byte(message)
	buf.WriteByte(byte(len(msgBytes) >> 8))
	buf.WriteByte(byte(len(msgBytes) & 0xFF))
	buf.Write(msgBytes)
	buf.WriteByte(0x00) // End
	buf.WriteByte(0)    // Overlay
	
	s.WriteClient(packet.Packet{
		ID:   protocol.CB_SYSTEM_CHAT,
		Data: buf.Bytes(),
	})
}
