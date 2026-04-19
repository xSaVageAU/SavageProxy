package relay

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"

	"savage-proxy/internal/intercept"
	"savage-proxy/internal/protocol"
	"savage-proxy/internal/proxy"

	mcnet "github.com/Tnze/go-mc/net"
	"github.com/Tnze/go-mc/net/packet"
)

// ConnectToBackend establishes a connection to the destination Minecraft server.
func ConnectToBackend(s *proxy.Session, address string) error {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to dial backend: %v", err)
	}

	s.BackendConn = mcnet.WrapConn(conn)

	// 1. Handshake (state 2 = Login)
	err = s.BackendConn.WritePacket(packet.Marshal(0x00,
		packet.VarInt(s.ProtocolVersion),
		packet.String(address),
		packet.UnsignedShort(25565),
		packet.VarInt(2),
	))
	if err != nil {
		return fmt.Errorf("failed to send backend handshake: %v", err)
	}

	// 2. Login Start
	err = s.BackendConn.WritePacket(packet.Marshal(0x00,
		packet.String(s.Player.Name),
		packet.UUID(s.ParseUUID(s.Player.UUID)),
	))
	if err != nil {
		return fmt.Errorf("failed to send backend login start: %v", err)
	}

	// 3. Login Phase Loop
	for {
		var p packet.Packet
		if err := s.BackendConn.ReadPacket(&p); err != nil {
			return fmt.Errorf("read backend packet failed: %v", err)
		}

		switch p.ID {
		case 0x00: // Disconnect
			var reason packet.String
			if err := p.Scan(&reason); err != nil {
				return fmt.Errorf("backend disconnected with unknown reason")
			}
			return fmt.Errorf("backend disconnected: %s", string(reason))

		case 0x01: // Encryption Request
			return fmt.Errorf("backend is in ONLINE MODE! Please set online-mode=false in server.properties")

		case 0x02: // Login Success
			log.Printf("[%s] Backend login successful, waiting for client ack...", s.Conn.Socket.RemoteAddr())
			if err := s.Conn.WritePacket(p); err != nil {
				return err
			}

			// Mandatory 1.20.2+ state transition: Wait for client's Login Acknowledgement (0x03)
			var ack packet.Packet
			if err := s.Conn.ReadPacket(&ack); err != nil {
				return fmt.Errorf("failed to read client login ack: %v", err)
			}
			if ack.ID != 0x03 {
				return fmt.Errorf("expected LoginAcknowledgement (0x03), got 0x%02X", ack.ID)
			}

			// Relay the acknowledgement to the backend to complete the handshake
			if err := s.BackendConn.WritePacket(ack); err != nil {
				return fmt.Errorf("failed to relay login ack to backend: %v", err)
			}

			log.Printf("[%s] Handshake complete! Entering Bridge mode.", s.Conn.Socket.RemoteAddr())
			return nil

		case 0x03: // Set Compression
			var threshold packet.VarInt
			if err := p.Scan(&threshold); err != nil {
				return fmt.Errorf("invalid compression threshold from backend")
			}

			s.BackendConn.SetThreshold(int(threshold))
			log.Printf("[%s] Relaying compression threshold: %d (Enabling after send)", s.Conn.Socket.RemoteAddr(), threshold)

			if err := s.Conn.WritePacket(p); err != nil {
				return err
			}
			s.Conn.SetThreshold(int(threshold))

		case 0x04: // Login Plugin Request
			var (
				messageID packet.VarInt
				channel   packet.Identifier
			)
			reader := bytes.NewReader(p.Data)
			if _, err := messageID.ReadFrom(reader); err != nil {
				return err
			}
			if _, err := channel.ReadFrom(reader); err != nil {
				return err
			}

			if string(channel) == "proxy:player_info" {
				log.Printf("[%s] Backend requested forwarding info on %s", s.Conn.Socket.RemoteAddr(), channel)
				response, err := s.CreateForwardingData()
				if err != nil {
					return err
				}

				if err := s.BackendConn.WritePacket(packet.Marshal(0x02,
					messageID,
					packet.Boolean(true),
					packet.PluginMessageData(response),
				)); err != nil {
					return fmt.Errorf("failed to send forwarding response: %v", err)
				}
			} else {
				if err := s.Conn.WritePacket(p); err != nil {
					return err
				}
			}

		default:
			log.Printf("[%s] Relaying unknown login packet from backend: 0x%02X", s.Conn.Socket.RemoteAddr(), p.ID)
			if err := s.Conn.WritePacket(p); err != nil {
				return err
			}
		}
	}
}

// StartBridge starts the bidirectional packet relay between the client and the backend server.
func StartBridge(s *proxy.Session) {
	// 1. Client to Backend (Packet Interception)
	go func() {
		defer s.Close()
		for {
			var p packet.Packet
			if err := s.Conn.ReadPacket(&p); err != nil {
				if err != io.EOF {
					log.Printf("[%s] Client read error: %v", s.Conn.Socket.RemoteAddr(), err)
				}
				return
			}

			// Intercept chat commands
			if p.ID == protocol.SB_CHAT_COMMAND {
				var cmd packet.String
				if err := p.Scan(&cmd); err == nil {
					commandText := string(cmd)
					if intercept.IsProxyCommand(commandText) {
						log.Printf("[ProxyCommand] Intercepted /%s from %s", commandText, s.Conn.Socket.RemoteAddr())
						intercept.HandleProxyCommand(s, commandText)
						continue
					}
				}
			}

			// Track and potentially intercept tab-complete requests
			if p.ID == protocol.SB_TAB_COMPLETE {
				var id packet.VarInt
				var text packet.String
				if err := p.Scan(&id, &text); err == nil {
					s.LastTabRequestID = int(id)
					s.LastTabRequestText = string(text)

					resp := intercept.HandleProxyTabCompletion(int(id), string(text), protocol.CB_TAB_COMPLETE)
					if resp != nil {
						log.Printf("[ProxyTab] Responding to tab request for '%s' from proxy", text)
						s.WriteClient(*resp)
						continue
					}
				}
			}

			if err := s.BackendConn.WritePacket(p); err != nil {
				return
			}
		}
	}()

	// 2. Backend to Client (Relay with interception)
	defer s.Close()
	declareCmdInjectAttempted := false
	for {
		var p packet.Packet
		if err := s.BackendConn.ReadPacket(&p); err != nil {
			if err != io.EOF {
				log.Printf("[%s] Backend read error: %v", s.Conn.Socket.RemoteAddr(), err)
			}
			return
		}

		// Inject proxy commands into Brigadier graph
		if !declareCmdInjectAttempted && p.ID == protocol.CB_DECLARE_COMMANDS {
			log.Printf("[%s] Intercepted declare_commands (0x%02X). Injecting proxy commands...", s.Conn.Socket.RemoteAddr(), p.ID)
			declareCmdInjectAttempted = true
			if err := intercept.InjectProxyCommands(&p); err != nil {
				log.Printf("[%s] Brigadier injection FAILED: %v", s.Conn.Socket.RemoteAddr(), err)
			}
		}

		// Intercept tab_complete response
		if p.ID == protocol.CB_TAB_COMPLETE {
			reqText := s.LastTabRequestText
			if len(reqText) > 0 {
				intercept.MergeProxySuggestions(&p, reqText)
			}
		}

		if err := s.WriteClient(p); err != nil {
			return
		}
	}
}
