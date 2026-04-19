package intercept

import (
	"log"
	"strings"

	"savage-proxy/internal/proxy"
)

// HandleProxyCommand executes a proxy-owned command.
func HandleProxyCommand(s *proxy.Session, commandText string) {
	parts := strings.Fields(commandText)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "savage":
		log.Printf("[ProxyCommand] %s executed /savage", s.Player.Name)
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
