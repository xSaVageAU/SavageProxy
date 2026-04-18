package proxy

import (
	"fmt"
	"io"
	"log"
	"net"

	mcnet "github.com/Tnze/go-mc/net"
	"github.com/Tnze/go-mc/net/packet"
)

func (s *Session) ConnectBackend(addr string) error {
	baseConn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to dial backend %s: %v", addr, err)
	}

	s.BackendConn = mcnet.WrapConn(baseConn)

	// 1. Handshake (Proxy -> Backend)
	err = s.BackendConn.WritePacket(packet.Marshal(0x00,
		packet.VarInt(s.ProtocolVersion),
		packet.String("127.0.0.1"),
		packet.UnsignedShort(25566),
		packet.VarInt(2), // Next State: Login
	))
	if err != nil {
		return fmt.Errorf("failed to send backend handshake: %v", err)
	}

	// 2. Login Start (Proxy -> Backend)
	// For 1.20.2+, LoginStart expects: Username (String), UUID (UUID)
	parsedUUID := s.parseUUID(s.Player.UUID)
	err = s.BackendConn.WritePacket(packet.Marshal(0x00,
		packet.String(s.Player.Name),
		packet.UUID(parsedUUID),
	))
	if err != nil {
		return fmt.Errorf("failed to send backend login start: %v", err)
	}

	// 3. Wait for LoginSuccess or PluginRequest
	for {
		var p packet.Packet
		if err := s.BackendConn.ReadPacket(&p); err != nil {
			return fmt.Errorf("failed to read backend response: %v", err)
		}

		switch p.ID {
		case 0x02: // LoginSuccess
			log.Printf("[%s] Backend Login Success", s.Conn.Socket.RemoteAddr())
			return nil
		case 0x03: // SetCompression
			var threshold packet.VarInt
			if err := p.Scan(&threshold); err == nil {
				s.BackendConn.SetThreshold(int(threshold))
				log.Printf("[%s] Backend set compression threshold to %d", s.Conn.Socket.RemoteAddr(), threshold)
			}
		case 0x04: // LoginPluginRequest (Forwarding Challenge)
			log.Printf("[%s] Backend requested LoginPlugin (Forwarding). Ignoring for now.", s.Conn.Socket.RemoteAddr())
			// We should probably respond with a "Success: false" if we aren't handling it yet
			// For now, let's just ignore to see if the server allows offline join.
		default:
			log.Printf("[%s] Received unexpected packet from backend during login: %X", s.Conn.Socket.RemoteAddr(), p.ID)
		}
	}
}

func (s *Session) Bridge() {
	errChan := make(chan error, 2)

	// Pipe: Client -> Backend
	go func() {
		for {
			var p packet.Packet
			if err := s.Conn.ReadPacket(&p); err != nil {
				errChan <- err
				return
			}
			if err := s.BackendConn.WritePacket(p); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// Pipe: Backend -> Client
	go func() {
		for {
			var p packet.Packet
			if err := s.BackendConn.ReadPacket(&p); err != nil {
				errChan <- err
				return
			}
			if err := s.Conn.WritePacket(p); err != nil {
				errChan <- err
				return
			}
		}
	}()

	err := <-errChan
	if err != nil && err != io.EOF {
		log.Printf("[%s] Bridge error: %v", s.Conn.Socket.RemoteAddr(), err)
	}
}
