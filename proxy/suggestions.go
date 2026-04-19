package proxy

import (
	"bytes"
	"io"
	"log"
	"strings"

	"github.com/Tnze/go-mc/net/packet"
)

// ============================================================
// TAB-COMPLETION (COMMAND SUGGESTIONS) HANDLING
// ============================================================

// HandleProxyTabCompletion checks if a tab-complete request targets a proxy command.
// If so, it builds the response packet and returns it. Otherwise returns nil.
// requestText is the full text the client sent (e.g., "/sav" or "/savage ").
func HandleProxyTabCompletion(transactionID int, requestText string, packetID int32) *packet.Packet {
	if len(requestText) == 0 || requestText[0] != '/' {
		return nil
	}

	text := requestText[1:] // strip leading slash
	parts := strings.SplitN(text, " ", 2)
	prefix := parts[0]

	// Check if we're still typing the command name (no space yet)
	if len(parts) == 1 && !strings.HasSuffix(requestText, " ") {
		// User is typing a command name — find matching proxy commands
		var matches []string
		for _, cmd := range ProxyCommands {
			if strings.HasPrefix(cmd.Name, strings.ToLower(prefix)) {
				matches = append(matches, cmd.Name)
			}
		}

		if len(matches) == 0 {
			return nil // No proxy matches, let the backend handle it
		}

		// Build a tab_complete response with our matches
		return buildTabCompleteResponse(transactionID, 1, len(prefix), matches, packetID)
	}

	// Check if we're inside a known proxy command's subcommand space
	for _, cmd := range ProxyCommands {
		if strings.EqualFold(prefix, cmd.Name) && len(cmd.Subcommands) > 0 {
			subPrefix := ""
			if len(parts) > 1 {
				subPrefix = parts[1]
			}

			var matches []string
			for _, sub := range cmd.Subcommands {
				if strings.HasPrefix(sub, strings.ToLower(subPrefix)) {
					matches = append(matches, sub)
				}
			}

			if len(matches) > 0 {
				start := len(prefix) + 2 // "/cmd " = 1(slash) + len(cmd) + 1(space)
				return buildTabCompleteResponse(transactionID, start, len(subPrefix), matches, packetID)
			}
		}
	}

	return nil
}

// buildTabCompleteResponse constructs a clientbound tab_complete packet.
func buildTabCompleteResponse(transactionID, start, length int, matches []string, packetID int32) *packet.Packet {
	buf := new(bytes.Buffer)

	packet.VarInt(transactionID).WriteTo(buf)
	packet.VarInt(start).WriteTo(buf)
	packet.VarInt(length).WriteTo(buf)
	packet.VarInt(len(matches)).WriteTo(buf)

	for _, match := range matches {
		packet.String(match).WriteTo(buf)
		// hasTooltip = false (Optional NBT, we write 0x00 to indicate "not present")
		buf.WriteByte(0x00)
	}

	return &packet.Packet{
		ID:   packetID,
		Data: buf.Bytes(),
	}
}

// MergeProxySuggestions appends proxy command suggestions into an existing
// tab_complete response packet from the backend.
// Returns true if the packet was modified.
func MergeProxySuggestions(p *packet.Packet, requestText string) bool {
	if len(requestText) == 0 || requestText[0] != '/' {
		return false
	}

	text := requestText[1:]
	parts := strings.SplitN(text, " ", 2)
	prefix := parts[0]

	// Only merge at the top-level command name stage
	if len(parts) > 1 || strings.HasSuffix(requestText, " ") {
		return false
	}

	// Find matching proxy commands
	var newMatches []string
	for _, cmd := range ProxyCommands {
		if strings.HasPrefix(cmd.Name, strings.ToLower(prefix)) {
			newMatches = append(newMatches, cmd.Name)
		}
	}

	if len(newMatches) == 0 {
		return false
	}

	// Parse the existing response
	r := bytes.NewReader(p.Data)

	var transactionID, start, length, matchCount packet.VarInt
	if _, err := transactionID.ReadFrom(r); err != nil {
		return false
	}
	if _, err := start.ReadFrom(r); err != nil {
		return false
	}
	if _, err := length.ReadFrom(r); err != nil {
		return false
	}
	if _, err := matchCount.ReadFrom(r); err != nil {
		return false
	}

	// Copy the rest of the existing matches (raw bytes) — we can't safely parse
	// tooltips since they're anonymous NBT. Just copy them as-is.
	existingMatchBytes := make([]byte, r.Len())
	io.ReadFull(r, existingMatchBytes)

	// Rebuild the packet
	buf := new(bytes.Buffer)
	transactionID.WriteTo(buf)
	start.WriteTo(buf)
	length.WriteTo(buf)

	newTotal := packet.VarInt(int(matchCount) + len(newMatches))
	newTotal.WriteTo(buf)

	// Write existing matches raw
	buf.Write(existingMatchBytes)

	// Append our new matches
	for _, match := range newMatches {
		packet.String(match).WriteTo(buf)
		buf.WriteByte(0x00) // No tooltip (optional NBT absent)
	}

	p.Data = buf.Bytes()

	log.Printf("[ProxyBrigadier] Merged %d proxy suggestion(s) into backend response", len(newMatches))
	return true
}
