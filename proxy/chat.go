package proxy

import (
	"bytes"
	"log"
	"strings"

	"github.com/Tnze/go-mc/net/packet"
)

// SendMessage sends a system chat message to the player using native packet construction.
func (s *Session) SendMessage(message string) {
	log.Printf("[%s] Sending proxy message: %s", s.Conn.Socket.RemoteAddr(), message)
	buf := new(bytes.Buffer)

	// In 1.21.1 (26.1+), System Chat uses a Network NBT Component (Nameless Root).
	// We construct a TAG_Compound containing a single TAG_String named "text".
	buf.WriteByte(0x0A) // Network NBT Root: TAG_Compound

	// Property: "text" (TAG_String)
	buf.WriteByte(0x08)
	buf.Write([]byte{0x00, 0x04}) // Key Length (4)
	buf.Write([]byte("text"))     // Key String

	msgBytes := []byte(message)
	// Value Length (2 bytes)
	buf.WriteByte(byte(len(msgBytes) >> 8))
	buf.WriteByte(byte(len(msgBytes) & 0xFF))
	buf.Write(msgBytes) // Value String

	buf.WriteByte(0x00) // TAG_End (Ends the TAG_Compound)

	// Overlay: Boolean (false = standard chat box)
	buf.WriteByte(0)

	// CB_SYSTEM_CHAT is 0x79 for 26.1.1
	s.WriteClient(packet.Packet{
		ID:   CB_SYSTEM_CHAT,
		Data: buf.Bytes(),
	})
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

// IsProxyCommand checks if a command string (without the leading slash) is a proxy command.
func IsProxyCommand(cmd string) bool {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false
	}
	for _, pc := range ProxyCommands {
		if parts[0] == pc.Name {
			return true
		}
	}
	return false
}
