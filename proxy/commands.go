package proxy

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/Tnze/go-mc/net/packet"
)

// ProxyCommand defines a command owned by the proxy.
type ProxyCommand struct {
	Name        string
	Subcommands []string // Optional subcommand literals (e.g., "reload", "status")
}

// ProxyCommands is the registry of all proxy-owned commands.
var ProxyCommands = []ProxyCommand{
	{Name: "savage"},
}


// ============================================================
// RAW BYTE-LEVEL BRIGADIER GRAPH INJECTION
// ============================================================
// Instead of parsing every node and re-serializing (which breaks
// on unhandled parser types), we walk the raw bytes to:
//   1. Count existing nodes
//   2. Find the root node
//   3. Patch the root's children array
//   4. Append our new literal nodes at the end
//   5. Update the node count
// This is fully parser-agnostic and won't break when Mojang
// adds new argument types.

// InjectProxyCommands modifies a declare_commands packet in-place to add proxy commands.
func InjectProxyCommands(p *packet.Packet) error {
	r := bytes.NewReader(p.Data)

	// 1. Read the node count
	var nodeCount packet.VarInt
	if _, err := nodeCount.ReadFrom(r); err != nil {
		return fmt.Errorf("failed to read node count: %w", err)
	}
	countEndPos := int(r.Size()) - r.Len() // byte position after count VarInt

	// 2. Walk through all nodes to find byte boundaries
	//    We need to find where the root node starts so we can patch its children.
	nodeBoundaries := make([]int, int(nodeCount)+1) // start position of each node
	nodeBoundaries[0] = countEndPos

	for i := 0; i < int(nodeCount); i++ {
		if err := skipNode(r); err != nil {
			return fmt.Errorf("failed to skip node %d/%d: %w", i, int(nodeCount), err)
		}
		nodeBoundaries[i+1] = int(r.Size()) - r.Len()
	}

	// 3. Read root index
	var rootIndex packet.VarInt
	if _, err := rootIndex.ReadFrom(r); err != nil {
		return fmt.Errorf("failed to read root index: %w", err)
	}

	// Sanity check
	if int(rootIndex) >= int(nodeCount) {
		return fmt.Errorf("root index %d out of bounds (count=%d)", rootIndex, nodeCount)
	}

	// 4. Parse ONLY the root node to get its children list
	rootStart := nodeBoundaries[int(rootIndex)]
	rootEnd := nodeBoundaries[int(rootIndex)+1]
	rootBytes := p.Data[rootStart:rootEnd]
	rootReader := bytes.NewReader(rootBytes)

	rootFlags, err := rootReader.ReadByte()
	if err != nil {
		return fmt.Errorf("failed to read root flags: %w", err)
	}

	var childCount packet.VarInt
	if _, err := childCount.ReadFrom(rootReader); err != nil {
		return fmt.Errorf("failed to read root child count: %w", err)
	}

	existingChildren := make([]packet.VarInt, childCount)
	for c := 0; c < int(childCount); c++ {
		if _, err := existingChildren[c].ReadFrom(rootReader); err != nil {
			return fmt.Errorf("failed to read root child %d: %w", c, err)
		}
	}
	// Rest of root node data (redirect, name, etc.) — we keep as raw bytes
	rootRemainder := make([]byte, rootReader.Len())
	rootReader.Read(rootRemainder)

	// 5. Build the new nodes we want to inject
	newNodes := buildProxyNodes(int(nodeCount))
	newNodeCount := len(newNodes)

	// 6. Add new node indices to root's children
	for i := 0; i < newNodeCount; i++ {
		existingChildren = append(existingChildren, packet.VarInt(int(nodeCount)+i))
	}

	// 7. Rebuild the root node with the expanded children list
	newRoot := new(bytes.Buffer)
	newRoot.WriteByte(rootFlags)
	newChildCount := packet.VarInt(len(existingChildren))
	newChildCount.WriteTo(newRoot)
	for _, child := range existingChildren {
		child.WriteTo(newRoot)
	}
	newRoot.Write(rootRemainder)

	// 8. Reconstruct the full packet
	out := new(bytes.Buffer)

	// Updated node count
	totalCount := packet.VarInt(int(nodeCount) + newNodeCount)
	totalCount.WriteTo(out)

	// Write all existing nodes, but replace the root node
	for i := 0; i < int(nodeCount); i++ {
		if i == int(rootIndex) {
			out.Write(newRoot.Bytes())
		} else {
			out.Write(p.Data[nodeBoundaries[i]:nodeBoundaries[i+1]])
		}
	}

	// Write our new injected nodes
	for _, nodeBytes := range newNodes {
		out.Write(nodeBytes)
	}

	// Write root index
	rootIndex.WriteTo(out)

	p.Data = out.Bytes()

	log.Printf("[ProxyBrigadier] Injected %d proxy command(s) into Brigadier graph (total nodes: %d → %d)",
		newNodeCount, int(nodeCount), int(totalCount))

	return nil
}

// buildProxyNodes creates serialized byte slices for each proxy command literal node.
func buildProxyNodes(startIndex int) [][]byte {
	var nodes [][]byte

	for _, cmd := range ProxyCommands {
		buf := new(bytes.Buffer)

		// Flags: 0x01 = Literal node
		buf.WriteByte(0x01)

		// Children count: 0 (leaf node)
		packet.VarInt(0).WriteTo(buf)

		// Name
		packet.String(cmd.Name).WriteTo(buf)

		nodes = append(nodes, buf.Bytes())
	}

	return nodes
}

// skipNode advances the reader past a single Brigadier node without allocating structures.
func skipNode(r *bytes.Reader) (err error) {
	startPos := int(r.Size()) - r.Len()
	
	// Helper to track and format the result state
	defer func() {
		if err != nil {
			log.Printf("[BrigadierDebug] Node @ %d FAILED: %v (remaining bytes: %d)", startPos, err, r.Len())
		}
	}()

	// 1. Flags (1 byte)
	flags, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("flags: %w", err)
	}

	nodeType := flags & 0x03

	// 2. Children (VarInt count + VarInt[] indices)
	var childCount packet.VarInt
	if _, err := childCount.ReadFrom(r); err != nil {
		return fmt.Errorf("child count: %w", err)
	}
	for c := 0; c < int(childCount); c++ {
		var child packet.VarInt
		if _, err := child.ReadFrom(r); err != nil {
			return fmt.Errorf("child %d: %w", c, err)
		}
	}

	// 3. Redirect Node (optional, if flag 0x08 set)
	if (flags & 0x08) != 0 {
		var redir packet.VarInt
		if _, err := redir.ReadFrom(r); err != nil {
			return fmt.Errorf("redirect: %w", err)
		}
	}

	// 4. Name (for Literal=1 and Argument=2)
	var nodeName string
	if nodeType == 1 || nodeType == 2 {
		var name packet.String
		if _, err := name.ReadFrom(r); err != nil {
			return fmt.Errorf("name: %w", err)
		}
		nodeName = string(name)
	}

	// 5. Parser ID + Properties (for Argument=2 only)
	if nodeType == 2 {
		var parserID packet.VarInt
		if _, err := parserID.ReadFrom(r); err != nil {
			return fmt.Errorf("parser id: %w", err)
		}
		
		log.Printf("[BrigadierDebug] Argument Node '%s' uses Parser ID %d", nodeName, parserID)
		
		if err := skipParserProperties(r, int(parserID)); err != nil {
			return fmt.Errorf("parser %d properties (node '%s'): %w", parserID, nodeName, err)
		}
	}

	// 6. Suggestions Type (optional, if flag 0x10 set)
	if (flags & 0x10) != 0 {
		var suggest packet.Identifier
		if _, err := suggest.ReadFrom(r); err != nil {
			return fmt.Errorf("suggestions: %w", err)
		}
	}

	endPos := int(r.Size()) - r.Len()
	typeNames := []string{"ROOT", "LITERAL", "ARGUMENT"}
	typeName := "UNKNOWN"
	if int(nodeType) < len(typeNames) {
		typeName = typeNames[nodeType]
	}
	log.Printf("[BrigadierDebug] Node @ %d-%d: type=%s flags=0x%02X children=%d name='%s'",
		startPos, endPos, typeName, flags, childCount, nodeName)

	return nil
}

// skipParserProperties advances the reader past the properties for a given parser ID.
// This is the COMPLETE registry for protocol version 775 (1.21.11 / 26.1.1).
// Parser IDs are from the minecraft:command_argument_type registry.
func skipParserProperties(r *bytes.Reader, parserID int) error {
	switch parserID {
	case 0: // brigadier:bool — no properties
		return nil

	case 1: // brigadier:float — flags(1) + optional min(4) + optional max(4)
		return skipNumberParser(r, 4)

	case 2: // brigadier:double — flags(1) + optional min(8) + optional max(8)
		return skipNumberParser(r, 8)

	case 3: // brigadier:integer — flags(1) + optional min(4) + optional max(4)
		return skipNumberParser(r, 4)

	case 4: // brigadier:long — flags(1) + optional min(8) + optional max(8)
		return skipNumberParser(r, 8)

	case 5: // brigadier:string — VarInt type enum (0=single, 1=quotable, 2=greedy)
		var v packet.VarInt
		_, err := v.ReadFrom(r)
		return err

	case 6: // minecraft:entity — 1 byte flags
		_, err := r.ReadByte()
		return err

	// 7-16: All void (no properties)
	case 7: // minecraft:game_profile
		return nil
	case 8: // minecraft:block_pos
		return nil
	case 9: // minecraft:column_pos
		return nil
	case 10: // minecraft:vec3
		return nil
	case 11: // minecraft:vec2
		return nil
	case 12: // minecraft:block_state
		return nil
	case 13: // minecraft:block_predicate
		return nil
	case 14: // minecraft:item_stack
		return nil
	case 15: // minecraft:item_predicate
		return nil
	case 16: // minecraft:color
		return nil

	// *** Protocol 775 inserts minecraft:hex_color at 17 ***
	case 17: // minecraft:hex_color — no properties (NEW in 1.21.11)
		return nil
	case 18: // minecraft:component — no properties
		return nil
	case 19: // minecraft:style — no properties
		return nil
	case 20: // minecraft:message — no properties
		return nil
	case 21: // minecraft:nbt — no properties (was nbt_compound_tag)
		return nil
	case 22: // minecraft:nbt_tag — no properties
		return nil
	case 23: // minecraft:nbt_path — no properties
		return nil
	case 24: // minecraft:objective — no properties
		return nil
	case 25: // minecraft:objective_criteria — no properties
		return nil
	case 26: // minecraft:operation — no properties
		return nil
	case 27: // minecraft:particle — no properties
		return nil
	case 28: // minecraft:angle — no properties
		return nil
	case 29: // minecraft:rotation — no properties
		return nil
	case 30: // minecraft:scoreboard_slot — no properties
		return nil

	case 31: // minecraft:score_holder — 1 byte flags
		_, err := r.ReadByte()
		return err

	case 32: // minecraft:swizzle — no properties
		return nil
	case 33: // minecraft:team — no properties
		return nil
	case 34: // minecraft:item_slot — no properties
		return nil
	case 35: // minecraft:item_slots — no properties
		return nil
	case 36: // minecraft:resource_location — no properties
		return nil
	case 37: // minecraft:function — no properties
		return nil
	case 38: // minecraft:entity_anchor — no properties
		return nil
	case 39: // minecraft:int_range — no properties
		return nil
	case 40: // minecraft:float_range — no properties
		return nil
	case 41: // minecraft:dimension — no properties
		return nil
	case 42: // minecraft:gamemode — no properties
		return nil

	case 43: // minecraft:time — 1 int (4 bytes) min value
		return skipBytes(r, 4)

	case 44, 45, 46, 47, 48: // minecraft:resource_or_tag, resource_or_tag_key, resource, resource_key, resource_selector
		// These all have a single Identifier (VarInt-prefixed String) — the registry name
		var s packet.Identifier
		_, err := s.ReadFrom(r)
		return err

	case 49: // minecraft:template_mirror — no properties
		return nil
	case 50: // minecraft:template_rotation — no properties
		return nil
	case 51: // minecraft:heightmap — no properties
		return nil
	case 52: // minecraft:loot_table — no properties
		return nil
	case 53: // minecraft:loot_predicate — no properties
		return nil
	case 54: // minecraft:loot_modifier — no properties
		return nil
	case 55: // minecraft:dialog — no properties (NEW in 1.21.11)
		return nil
	case 56: // minecraft:uuid — no properties (NEW in 1.21.11)
		return nil

	default:
		// For any completely unknown parser, we assume no properties.
		// This is a safety net. If a new parser with properties is added
		// and we hit this, the graph WILL break. Log it so we can fix it.
		log.Printf("[ProxyBrigadier] WARNING: Unknown parser ID %d — assuming 0 property bytes. Graph may be corrupted!", parserID)
		return nil
	}
}

// skipNumberParser skips a brigadier number parser (float/double/int/long).
// Format: 1 byte flags, optional min (typeSize bytes), optional max (typeSize bytes).
func skipNumberParser(r *bytes.Reader, typeSize int) error {
	flags, err := r.ReadByte()
	if err != nil {
		return err
	}
	if (flags & 0x01) != 0 { // has min
		if err := skipBytes(r, typeSize); err != nil {
			return err
		}
	}
	if (flags & 0x02) != 0 { // has max
		if err := skipBytes(r, typeSize); err != nil {
			return err
		}
	}
	return nil
}

// skipBytes discards exactly n bytes from the reader.
func skipBytes(r *bytes.Reader, n int) error {
	for i := 0; i < n; i++ {
		if _, err := r.ReadByte(); err != nil {
			return err
		}
	}
	return nil
}

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
