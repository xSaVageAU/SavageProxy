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


