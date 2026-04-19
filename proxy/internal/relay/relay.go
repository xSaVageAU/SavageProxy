package relay

import (
	"bytes"
	"fmt"
	"net"
	"strings"

	"savage-proxy/internal/intercept"
	"savage-proxy/internal/protocol"
	"savage-proxy/internal/proxy"

	mcnet "github.com/Tnze/go-mc/net"
	"github.com/Tnze/go-mc/net/packet"
)

// ConnectToBackend establishes the initial connection for a player joining the proxy.
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

const (
	StateHandshaking = 0
	StateLogin       = 1
	StateConfig      = 2
	StatePlay        = 3
)

// SilentHandshake connects to a target server in the background and navigates to the Play state.
func SilentHandshake(s *proxy.Session, address string) (*mcnet.Conn, int32, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, 0, err
	}
	backend := mcnet.WrapConn(conn)
	currentState := StateLogin

	// 1. Handshake
	backend.WritePacket(packet.Marshal(0x00,
		packet.VarInt(s.ProtocolVersion),
		packet.String(address),
		packet.UnsignedShort(25565),
		packet.VarInt(2),
	))

	// 2. Login Start
	backend.WritePacket(packet.Marshal(0x00,
		packet.String(s.Player.Name),
		packet.UUID(s.ParseUUID(s.Player.UUID)),
	))

	for {
		var p packet.Packet
		if err := backend.ReadPacket(&p); err != nil {
			return nil, 0, fmt.Errorf("read error [State %d]: %v", currentState, err)
		}

		if currentState == StateLogin {
			switch p.ID {
			case 0x02: // Login Success
				backend.WritePacket(packet.Marshal(0x03)) // Ack Login
				
				// Server is now in Configuration Phase natively!
				// Return the connection so we can natively proxy the Config Phase!
				return backend, 0, nil
			case 0x03: // Set Compression
				var threshold packet.VarInt
				p.Scan(&threshold)
				backend.SetThreshold(int(threshold))
			case 0x04: // Login Plugin Request
				var (
					messageID packet.VarInt
					channel   packet.Identifier
				)
				reader := bytes.NewReader(p.Data)
				messageID.ReadFrom(reader)
				channel.ReadFrom(reader)
				if string(channel) == "proxy:player_info" {
					response, _ := s.CreateForwardingData()
					backend.WritePacket(packet.Marshal(0x02, messageID, packet.Boolean(true), packet.PluginMessageData(response)))
				}
			}
			continue
		}
	}
}

// StartBridge starts the bidirectional packet relay.
func StartBridge(s *proxy.Session) {
	go func() {
		defer s.Close()
		for {
			var p packet.Packet
			if err := s.Conn.ReadPacket(&p); err != nil {
				return
			}
			
			// If we are transitioning between servers via the Configuration phase
			if s.ConfigPhaseClientSide {
				if p.ID == 0x10 { // Client Acknowledged Configuration (response to our 0x76)
					// Drop it because the new backend server never sent 0x76, it entered Config natively.
					s.ConfigPhaseClientSide = false
					continue
				}
			}

			if p.ID == protocol.SB_CHAT_COMMAND {
				var cmd packet.String
				p.Scan(&cmd)
				cmdText := string(cmd)
				if strings.HasPrefix(cmdText, "savage switch ") {
					target := strings.TrimPrefix(cmdText, "savage switch ")
					go HandleSwitchCommand(s, target)
					continue
				}
				if intercept.IsProxyCommand(cmdText) {
					intercept.HandleProxyCommand(s, cmdText)
					continue
				}
			}
			if backend := s.GetBackend(); backend != nil {
				backend.WritePacket(p)
			}
		}
	}()

	defer s.Close()
	declareCmdInjectAttempted := false
	for {
		backend := s.GetBackend()
		if backend == nil {
			return
		}

		var p packet.Packet
		if err := backend.ReadPacket(&p); err != nil {
			if s.GetBackend() != backend {
				continue
			}
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

func HandleSwitchCommand(s *proxy.Session, target string) {
	s.SendMessage("§b[Proxy] Pre-flight connection to §f" + target + "§b...")
	newConn, _, err := SilentHandshake(s, target)
	if err != nil {
		s.SendMessage("§c[Proxy] Pre-flight failed: §f" + err.Error())
		return
	}
	s.SendMessage("§a[Proxy] Background server ready! Entering Config phase...")
	
	// Set the flag to intercept the single 0x0C Ack packet
	s.ConfigPhaseClientSide = true
	
	// Send Start Configuration to the client (kicks client into Config state)
	// Protocol 775 (1.21.1/26.1.1) Start Configuration is 0x76 (0x69 + 13 shift)
	s.WriteClient(packet.Marshal(0x76))
	
	// Swap backends immediately. Old play packets stop. New config packets start flowing.
	oldBackend := s.GetBackend()
	s.SetBackend(newConn)
	if oldBackend != nil {
		oldBackend.Close()
	}
}
