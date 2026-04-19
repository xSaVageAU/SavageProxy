package proxy

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"strings"

	mcnet "github.com/Tnze/go-mc/net"
	"github.com/Tnze/go-mc/net/packet"
)

// ============================================================
// 26.1.1 PACKET ID CONSTANTS
// ============================================================
// These must match the actual protocol version in use.
// Adjust if connecting to a different MC version.
//
// Known verified IDs (confirmed by user testing):
//   Serverbound 0x07 = chat_command (works for /savage)
//   Clientbound 0x79 = system_chat  (works for SendMessage)
//
// Derived IDs (MUST BE VERIFIED — may need adjustment):
//   If new packets were added relative to 1.21.11 in the
//   clientbound direction, all IDs after the insertion point shift.
const (
	// Serverbound Play
	SB_CHAT_COMMAND    int32 = 0x07 // Unsigned chat command
	SB_TAB_COMPLETE    int32 = 0x0E // Tab complete / command suggestions request

	// Clientbound Play
	CB_TAB_COMPLETE        int32 = 0x0F // Tab complete / command suggestions response
	CB_DECLARE_COMMANDS    int32 = 0x10 // Brigadier command graph
	CB_SYSTEM_CHAT         int32 = 0x79 // System chat message
)

func (s *Session) ConnectToBackend(address string) error {
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
		packet.UUID(s.parseUUID(s.Player.UUID)),
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
			
			// SYNC BACKEND FIRST
			s.BackendConn.SetThreshold(int(threshold))
			
			log.Printf("[%s] Relaying compression threshold: %d (Enabling after send)", s.Conn.Socket.RemoteAddr(), threshold)
			
			// RELAY UNCOMPRESSED TO CLIENT (Crucial!)
			if err := s.Conn.WritePacket(p); err != nil {
				return err
			}

			// NOW ENABLE FOR CLIENT
			s.Conn.SetThreshold(int(threshold))

		case 0x04: // Login Plugin Request
			var (
				messageID packet.VarInt
				channel   packet.Identifier
			)
			// Manual scan because tail of packet is data
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
				// Relay unknown plugin request to client
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

func (s *Session) Bridge() {
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

			// ==================================================
			// SERVERBOUND PACKET INTERCEPTION
			// ==================================================

			// Intercept chat commands (both unsigned 0x07 patterns)
			if p.ID == SB_CHAT_COMMAND {
				var cmd packet.String
				if err := p.Scan(&cmd); err == nil {
					commandText := string(cmd)
					
					if IsProxyCommand(commandText) {
						log.Printf("[ProxyCommand] Intercepted /%s from %s", commandText, s.Conn.Socket.RemoteAddr())
						s.HandleProxyCommand(commandText)
						continue // Swallow — don't forward to backend
					}
				}
			}

			// Track and potentially intercept tab-complete requests
			if p.ID == SB_TAB_COMPLETE {
				var id packet.VarInt
				var text packet.String
				if err := p.Scan(&id, &text); err == nil {
					s.LastTabRequestID = int(id)
					s.LastTabRequestText = string(text)

					// If the user is typing a proxy command, respond immediately
					resp := HandleProxyTabCompletion(int(id), string(text), CB_TAB_COMPLETE)
					if resp != nil {
						log.Printf("[ProxyTab] Responding to tab request for '%s' from proxy", text)
						s.WriteClient(*resp)
						continue // Don't forward to backend
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
	declareCmdInjectAttempted := false // Track if we've tried injecting commands
	for {
		var p packet.Packet
		if err := s.BackendConn.ReadPacket(&p); err != nil {
			if err != io.EOF {
				log.Printf("[%s] Backend read error: %v", s.Conn.Socket.RemoteAddr(), err)
			}
			return
		}

		// ==================================================
		// CLIENTBOUND PACKET INTERCEPTION (steady-state)
		// ==================================================

		// Inject proxy commands into Brigadier graph (runs once per session)
		if !declareCmdInjectAttempted && p.ID == CB_DECLARE_COMMANDS {
			log.Printf("[%s] Intercepted declare_commands (0x%02X). Injecting proxy commands...", s.Conn.Socket.RemoteAddr(), p.ID)
			declareCmdInjectAttempted = true
			if err := InjectProxyCommands(&p); err != nil {
				log.Printf("[%s] Brigadier injection FAILED: %v", s.Conn.Socket.RemoteAddr(), err)
			}
		}

		// ==================================================
		// CLIENTBOUND PACKET INTERCEPTION (steady-state)
		// ==================================================

		// Intercept tab_complete response — merge proxy suggestions
		if p.ID == CB_TAB_COMPLETE {
			reqText := s.LastTabRequestText
			if len(reqText) > 0 {
				MergeProxySuggestions(&p, reqText)
			}
		}

		// Clientbound packets are safely relayed.
		if err := s.WriteClient(p); err != nil {
			return
		}
	}
}

// probeDeclareCommands checks if a packet looks like a declare_commands packet
// by examining its structure without fully parsing it.
func probeDeclareCommands(data []byte) bool {
	if len(data) < 10 {
		return false
	}

	r := bytes.NewReader(data)

	// Read the first VarInt — should be a reasonable node count
	var count packet.VarInt
	if _, err := count.ReadFrom(r); err != nil {
		return false
	}

	// Sanity check: a real Brigadier graph has 50-10000 nodes
	if count < 30 || count > 10000 {
		return false
	}

	// Read the first node's flags byte
	flags, err := r.ReadByte()
	if err != nil {
		return false
	}

	nodeType := flags & 0x03

	// The first node should typically be the root (type 0) OR
	// the root could be at any index, but most servers put it first.
	// However, root type = 0 is the most reliable indicator when combined
	// with a large node count.
	// Root nodes have many children and no name.
	if nodeType == 0 {
		// Extra validation: root should have a decent number of children
		var childCount packet.VarInt
		if _, err := childCount.ReadFrom(r); err != nil {
			return false
		}
		// Root typically has 20+ children (commands like /tp, /give, etc.)
		return childCount >= 5
	}

	// If the first node isn't root, it could still be declare_commands
	// (root is at the end). Check if the overall structure is reasonable.
	// The data size should be proportional to the node count.
	bytesPerNode := float64(len(data)) / float64(count)
	return bytesPerNode >= 3 && bytesPerNode <= 200
}

// HandleProxyCommand executes a proxy-owned command.
func (s *Session) HandleProxyCommand(commandText string) {
	parts := strings.Fields(commandText)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "savage":
		s.SendMessage("§b§l[SavageProxy] §fNative 26.1.1 Engine §aActive")
		s.SendMessage("§7Proxy Command executed beautifully!")
	default:
		s.SendMessage("§c§l[SavageProxy] §fUnknown proxy command: /" + parts[0])
	}
}
