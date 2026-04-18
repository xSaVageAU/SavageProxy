package proxy

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"

	"github.com/Tnze/go-mc/net/packet"
)

// CreateForwardingData generates the signed payload for the "proxy:player_info" channel
func (s *Session) CreateForwardingData() ([]byte, error) {
	buf := new(bytes.Buffer)

	// 1. Forwarding Version (1 for now)
	if _, err := packet.VarInt(1).WriteTo(buf); err != nil {
		return nil, err
	}

	// 2. Player Address
	remoteAddr := s.Conn.Socket.RemoteAddr().String()
	if _, err := packet.String(remoteAddr).WriteTo(buf); err != nil {
		return nil, err
	}

	// 3. Player UUID
	pUUID := s.parseUUID(s.Player.UUID)
	if _, err := buf.Write(pUUID[:]); err != nil {
		return nil, err
	}

	// 4. Player Name
	if _, err := packet.String(s.Player.Name).WriteTo(buf); err != nil {
		return nil, err
	}

	// 5. Profile Properties
	if _, err := packet.VarInt(len(s.Player.Properties)).WriteTo(buf); err != nil {
		return nil, err
	}
	for _, prop := range s.Player.Properties {
		if _, err := packet.String(prop.Name).WriteTo(buf); err != nil {
			return nil, err
		}
		if _, err := packet.String(prop.Value).WriteTo(buf); err != nil {
			return nil, err
		}
		if prop.Signature != "" {
			if _, err := packet.Boolean(true).WriteTo(buf); err != nil {
				return nil, err
			}
			if _, err := packet.String(prop.Signature).WriteTo(buf); err != nil {
				return nil, err
			}
		} else {
			if _, err := packet.Boolean(false).WriteTo(buf); err != nil {
				return nil, err
			}
		}
	}

	data := buf.Bytes()

	// 6. Sign the data with HMAC-SHA256
	mac := hmac.New(sha256.New, []byte(s.ForwardingSecret))
	mac.Write(data)
	signature := mac.Sum(nil)

	// Final Payload: Signature + Data
	finalBuf := new(bytes.Buffer)
	finalBuf.Write(signature)
	finalBuf.Write(data)

	return finalBuf.Bytes(), nil
}
