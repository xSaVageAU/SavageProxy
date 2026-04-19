package proxy

import (
	"fmt"
	"log"
	"time"

	"github.com/Tnze/go-mc/net/packet"
)

func (s *Session) Handshake() error {
	s.Conn.Socket.SetDeadline(time.Now().Add(HandshakeTimeout))

	var p packet.Packet
	if err := s.Conn.ReadPacket(&p); err != nil {
		return fmt.Errorf("failed to read handshake packet: %v", err)
	}

	if p.ID != 0x00 {
		return fmt.Errorf("unexpected packet ID: %X (expected 0x00)", p.ID)
	}

	var (
		protocolVersion packet.VarInt
		serverAddress   packet.String
		serverPort      packet.UnsignedShort
		nextState       packet.VarInt
	)

	if err := p.Scan(&protocolVersion, &serverAddress, &serverPort, &nextState); err != nil {
		return fmt.Errorf("failed to scan handshake packet: %v", err)
	}

	s.State = int32(nextState)
	s.ProtocolVersion = int32(protocolVersion)

	log.Printf("[%s] Handshake: Version=%d, Host=%s:%d, Next=%d",
		s.Conn.Socket.RemoteAddr(), protocolVersion, serverAddress, serverPort, nextState)

	s.Conn.Socket.SetDeadline(time.Time{})
	return nil
}
