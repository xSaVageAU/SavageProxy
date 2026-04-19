package relay

import (
	"bytes"
	"fmt"
	"net"

	"savage-proxy/internal/intercept"
	"savage-proxy/internal/protocol"
	"savage-proxy/internal/proxy"

	mcnet "github.com/Tnze/go-mc/net"
	"github.com/Tnze/go-mc/net/packet"
)

// ConnectToBackend establishes a connection and completes the handshake with a backend server.
func ConnectToBackend(s *proxy.Session, address string) error {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return fmt.Errorf("failed dial: %v", err)
	}
	
	backend := mcnet.WrapConn(conn)

	// 1. Handshake
	if err := backend.WritePacket(packet.Marshal(0x00,
		packet.VarInt(s.ProtocolVersion),
		packet.String(address),
		packet.UnsignedShort(25565),
		packet.VarInt(2),
	)); err != nil {
		return err
	}

	// 2. Login Start
	if err := backend.WritePacket(packet.Marshal(0x00,
		packet.String(s.Player.Name),
		packet.UUID(s.ParseUUID(s.Player.UUID)),
	)); err != nil {
		return err
	}

	// 3. Login Phase Loop
	for {
		var p packet.Packet
		if err := backend.ReadPacket(&p); err != nil {
			return err
		}

		switch p.ID {
		case 0x02: // Success
			if err := s.Conn.WritePacket(p); err != nil {
				return err
			}
			var ack packet.Packet
			if err := s.Conn.ReadPacket(&ack); err != nil {
				return err
			}
			if err := backend.WritePacket(ack); err != nil {
				return err
			}
			
			// We are now in state where we move to configuration/play
			s.SetBackend(backend)
			return nil

		case 0x03: // Set Compression
			var threshold packet.VarInt
			p.Scan(&threshold)
			backend.SetThreshold(int(threshold))
			if err := s.Conn.WritePacket(p); err != nil {
				return err
			}
			s.Conn.SetThreshold(int(threshold))

		case 0x04: // Plugin Request
			var (
				messageID packet.VarInt
				channel   packet.Identifier
			)
			reader := bytes.NewReader(p.Data)
			messageID.ReadFrom(reader)
			channel.ReadFrom(reader)

			if string(channel) == "proxy:player_info" {
				response, err := s.CreateForwardingData()
				if err != nil {
					return err
				}
				if err := backend.WritePacket(packet.Marshal(0x02, messageID, packet.Boolean(true), packet.PluginMessageData(response))); err != nil {
					return err
				}
			} else {
				s.Conn.WritePacket(p)
			}

		default:
			s.Conn.WritePacket(p)
		}
	}
}

// StartBridge starts the bidirectional packet relay between the client and the backend.
func StartBridge(s *proxy.Session) {
	// 1. Client to Backend
	go func() {
		defer s.Close()
		for {
			var p packet.Packet
			if err := s.Conn.ReadPacket(&p); err != nil {
				return
			}
			
			// Handle Command Interception
			if p.ID == protocol.SB_CHAT_COMMAND {
				var cmd packet.String
				p.Scan(&cmd)
				if intercept.IsProxyCommand(string(cmd)) {
					intercept.HandleProxyCommand(s, string(cmd))
					continue
				}
			}
			
			if p.ID == protocol.SB_TAB_COMPLETE {
				var id packet.VarInt
				var text packet.String
				if err := p.Scan(&id, &text); err == nil {
					s.LastTabRequestText = string(text)
				}
			}

			// Safe access to the backend connection
			if backend := s.GetBackend(); backend != nil {
				backend.WritePacket(p)
			}
		}
	}()

	// 2. Backend to Client
	defer s.Close()
	declareCmdInjectAttempted := false
	for {
		// Safe access to the backend connection
		backend := s.GetBackend()
		if backend == nil {
			return
		}

		var p packet.Packet
		if err := backend.ReadPacket(&p); err != nil {
			return
		}

		if !declareCmdInjectAttempted && p.ID == protocol.CB_DECLARE_COMMANDS {
			declareCmdInjectAttempted = true
			intercept.InjectProxyCommands(&p)
		}

		if p.ID == protocol.CB_TAB_COMPLETE {
			intercept.MergeProxySuggestions(&p, s.LastTabRequestText)
		}

		s.WriteClient(p)
	}
}
