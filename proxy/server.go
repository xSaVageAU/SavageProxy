package proxy

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"log"
	"net"

	mcnet "github.com/Tnze/go-mc/net"
)

type Server struct {
	Addr             string
	PrivKey          *rsa.PrivateKey
	ForwardingSecret string
}

func NewServer(addr string) *Server {
	// Generate a 1024-bit RSA key for the encryption handshake
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		log.Fatalf("Failed to generate RSA key: %v", err)
	}

	return &Server{
		Addr:             addr,
		PrivKey:          key,
		ForwardingSecret: "", // Will be set from config in main()
	}
}

func (s *Server) Listen() error {
	l, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %v", s.Addr, err)
	}
	defer l.Close()

	log.Printf("Savage Proxy listening on %s", s.Addr)

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(baseConn net.Conn) {
	conn := mcnet.WrapConn(baseConn)
	session := NewSession(conn, s.PrivKey)
	session.ForwardingSecret = s.ForwardingSecret

	defer session.Close()

	log.Printf("New connection from %s", baseConn.RemoteAddr())

	if err := session.Handshake(); err != nil {
		log.Printf("[%s] Handshake error: %v", baseConn.RemoteAddr(), err)
		return
	}

	switch session.State {
	case 1: // Status
		if err := session.HandleStatus(); err != nil {
			log.Printf("[%s] Status error: %v", baseConn.RemoteAddr(), err)
		}
	case 2: // Login
		if err := session.HandleLogin(); err != nil {
			log.Printf("[%s] Login error: %v", baseConn.RemoteAddr(), err)
		}
	default:
		log.Printf("[%s] Unknown next state: %d", baseConn.RemoteAddr(), session.State)
	}
}
