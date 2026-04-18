package proxy

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/Tnze/go-mc/net/packet"
)

// HandleStatus implements the Server List Ping protocol
func (s *Session) HandleStatus() error {
	// 1. Receive Status Request (0x00)
	var p packet.Packet
	if err := s.Conn.ReadPacket(&p); err != nil {
		return fmt.Errorf("failed to read status request: %v", err)
	}
	if p.ID != 0x00 {
		return fmt.Errorf("expected StatusRequest (0x00), got %X", p.ID)
	}

	// 2. Send Status Response (0x00)
	statusJSON, err := s.buildStatusJSON()
	if err != nil {
		return fmt.Errorf("failed to build status JSON: %v", err)
	}

	resp := packet.Marshal(0x00, packet.String(statusJSON))
	if err := s.Conn.WritePacket(resp); err != nil {
		return fmt.Errorf("failed to write status response: %v", err)
	}

	// 3. Receive Ping Request (0x01)
	if err := s.Conn.ReadPacket(&p); err != nil {
		return fmt.Errorf("failed to read status ping: %v", err)
	}
	if p.ID != 0x01 {
		return fmt.Errorf("expected StatusPing (0x01), got %X", p.ID)
	}

	// 4. Send Ping Response (0x01)
	// The payload is just what the client sent (usually a 64-bit long)
	if err := s.Conn.WritePacket(p); err != nil {
		return fmt.Errorf("failed to write ping response: %v", err)
	}

	log.Printf("[%s] Status Ping completed", s.Conn.Socket.RemoteAddr())
	return nil
}

func (s *Session) buildStatusJSON() (string, error) {
	status := struct {
		Version struct {
			Name     string `json:"name"`
			Protocol int    `json:"protocol"`
		} `json:"version"`
		Players struct {
			Max    int `json:"max"`
			Online int `json:"online"`
			Sample []struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			} `json:"sample"`
		} `json:"players"`
		Description struct {
			Text string `json:"text"`
		} `json:"description"`
	}{
		Version: struct {
			Name     string `json:"name"`
			Protocol int    `json:"protocol"`
		}{
			Name:     "Savage Proxy 26.1",
			Protocol: 780, // Target version determined in research
		},
		Players: struct {
			Max    int `json:"max"`
			Online int `json:"online"`
			Sample []struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			} `json:"sample"`
		}{
			Max:    1337,
			Online: 0,
			Sample: []struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			}{},
		},
		Description: struct {
			Text string `json:"text"`
		}{
			Text: "§6§lSavage Proxy§r\n§7Foundation Established.",
		},
	}

	data, err := json.Marshal(status)
	return string(data), err
}
