package intercept

import (
	"bytes"
	"fmt"
	"log"

	"github.com/Tnze/go-mc/net/packet"
)


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
