package proxy

import (
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
	BackendConn *mcnet.Conn

	// Auth Data
	PrivKey *rsa.PrivateKey
	Player  struct {
		Name string
		UUID string
	}
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
	return s.Conn.Close()
}
