package proxy

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"

	mcnet "github.com/Tnze/go-mc/net"
	"github.com/Tnze/go-mc/net/packet"
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
				// Relay unknown plugin request to client?
				// Better to just relay it and let the client answer
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

			// Intercept Chat Command (0x06: Chat Command, 0x07: Signed Chat Command in 26.1+)
			if p.ID == 0x06 || p.ID == 0x07 {
				var cmd packet.String
				if err := p.Scan(&cmd); err == nil {
					commandName := string(cmd)
					if commandName == "savage" {
						s.SendMessage("§b§l[SavageProxy] §fNative Protocol Engine §aActive")
						s.SendMessage("§7This message was intercepted locally - ownership achieved.")
						continue 
					}
				}
			}

			if err := s.BackendConn.WritePacket(p); err != nil {
				return
			}
		}
	}()

	// 2. Backend to Client (Relay)
	defer s.Close()
	for {
		var p packet.Packet
		if err := s.BackendConn.ReadPacket(&p); err != nil {
			if err != io.EOF {
				log.Printf("[%s] Backend read error: %v", s.Conn.Socket.RemoteAddr(), err)
			}
			return
		}
		if err := s.Conn.WritePacket(p); err != nil {
			return
		}
	}
}
