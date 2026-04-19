package proxy

import (
	"bytes"
	"log"

	"github.com/Tnze/go-mc/net/packet"
)

// BrigadierNode represents a single command node in the graph
type BrigadierNode struct {
	Flags          byte
	Children       []packet.VarInt
	RedirectNode   packet.VarInt
	Name           packet.String // For Literal (1) or Argument (2)
	ParserID       packet.VarInt // For Argument (2)
	PropertiesData []byte        // Raw properties bytes, because doing 54 full parsers is overkill
	SuggestIndex   packet.Identifier
}

// InjectProxyCommands intercepts a declare_commands packet, parses the Brigadier graph,
// appends custom proxy nodes, and overwrites the Root Node to reference them.
func InjectProxyCommands(p *packet.Packet) error {
	// Let's decode the VarInt Count
	r := bytes.NewReader(p.Data)

	var count packet.VarInt
	if _, err := count.ReadFrom(r); err != nil {
		return err
	}

	nodes := make([]BrigadierNode, count)

	for i := 0; i < int(count); i++ {
		node := BrigadierNode{}

		// 1. Flags
		flags, err := r.ReadByte()
		if err != nil {
			return err
		}
		node.Flags = flags

		nodeType := flags & 0x03

		// 2. Children
		var childrenCount packet.VarInt
		if _, err := childrenCount.ReadFrom(r); err != nil {
			return err
		}

		node.Children = make([]packet.VarInt, childrenCount)
		for c := 0; c < int(childrenCount); c++ {
			var child packet.VarInt
			if _, err := child.ReadFrom(r); err != nil {
				return err
			}
			node.Children[c] = child
		}

		// 3. Redirect Node
		if (flags & 0x08) != 0 {
			if _, err := node.RedirectNode.ReadFrom(r); err != nil {
				return err
			}
		}

		// 4. Name
		if nodeType == 1 || nodeType == 2 {
			if _, err := node.Name.ReadFrom(r); err != nil {
				return err
			}
		}

		// 5. Parser and Properties
		if nodeType == 2 {
			if _, err := node.ParserID.ReadFrom(r); err != nil {
				return err
			}

			// Extracting properties requires knowing exactly how many bytes the parser takes.
			// To keep it simple without coding 54 parsers, we will implement a dynamic property scanner
			// that tracks the bytes for standard 26.1.1 Registry mappings.
			propBytes, err := readParserProperties(r, int(node.ParserID))
			if err != nil {
				return err
			}
			node.PropertiesData = propBytes
		}

		// 6. Suggestions Type
		if (flags & 0x10) != 0 {
			if _, err := node.SuggestIndex.ReadFrom(r); err != nil {
				return err
			}
		}

		nodes[i] = node
	}

	var rootIndex packet.VarInt
	if _, err := rootIndex.ReadFrom(r); err != nil {
		return err
	}

	// ==========================================
	// GRAPH INJECTION LOGIC
	// ==========================================

	// Our new node will be placed at the very end of the array
	savageNodeIndex := packet.VarInt(len(nodes))

	savageLiteral := BrigadierNode{
		Flags:    0x01, // 1 = Literal Node
		Name:     packet.String("savage"),
		Children: []packet.VarInt{},
	}

	nodes = append(nodes, savageLiteral)

	// Now we just need to add our node's index to the Root Node's children list!
	nodes[rootIndex].Children = append(nodes[rootIndex].Children, savageNodeIndex)

	// ==========================================
	// SERIALIZATION
	// ==========================================

	buf := new(bytes.Buffer)
	newCount := packet.VarInt(len(nodes))
	newCount.WriteTo(buf)

	for _, node := range nodes {
		buf.WriteByte(node.Flags)

		childCount := packet.VarInt(len(node.Children))
		childCount.WriteTo(buf)
		for _, child := range node.Children {
			child.WriteTo(buf)
		}

		if (node.Flags & 0x08) != 0 {
			node.RedirectNode.WriteTo(buf)
		}

		nodeType := node.Flags & 0x03
		if nodeType == 1 || nodeType == 2 {
			node.Name.WriteTo(buf)
		}

		if nodeType == 2 {
			node.ParserID.WriteTo(buf)
			buf.Write(node.PropertiesData)
		}

		if (node.Flags & 0x10) != 0 {
			node.SuggestIndex.WriteTo(buf)
		}
	}

	rootIndex.WriteTo(buf)

	// OVERWRITE PACKET DATA
	p.Data = buf.Bytes()
	log.Printf("[ProxyBrigadier] Successfully injected proxy commands into backend graph!")

	return nil
}

// readParserProperties returns the raw bytes belonging to specific Brigadier argument parsers.
func readParserProperties(r *bytes.Reader, parserID int) ([]byte, error) {
	buf := new(bytes.Buffer)

	// WARNING: These IDs are mapped strictly for 1.21.x / 26.1.1 Registry.
	// You will update this engine when MC updates!

	switch parserID {
	case 1: // brigadier:float
		flags, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		buf.WriteByte(flags)
		if (flags & 0x01) != 0 {
			b := make([]byte, 4)
			r.Read(b)
			buf.Write(b) // Min
		}
		if (flags & 0x02) != 0 {
			b := make([]byte, 4)
			r.Read(b)
			buf.Write(b) // Max
		}
	case 2: // brigadier:double
		flags, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		buf.WriteByte(flags)
		if (flags & 0x01) != 0 {
			b := make([]byte, 8)
			r.Read(b)
			buf.Write(b) // Min
		}
		if (flags & 0x02) != 0 {
			b := make([]byte, 8)
			r.Read(b)
			buf.Write(b) // Max
		}
	case 3: // brigadier:integer
		flags, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		buf.WriteByte(flags)
		if (flags & 0x01) != 0 {
			b := make([]byte, 4)
			r.Read(b)
			buf.Write(b) // Min
		}
		if (flags & 0x02) != 0 {
			b := make([]byte, 4)
			r.Read(b)
			buf.Write(b) // Max
		}
	case 4: // brigadier:long
		flags, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		buf.WriteByte(flags)
		if (flags & 0x01) != 0 {
			b := make([]byte, 8)
			r.Read(b)
			buf.Write(b) // Min
		}
		if (flags & 0x02) != 0 {
			b := make([]byte, 8)
			r.Read(b)
			buf.Write(b) // Max
		}
	case 5: // brigadier:string
		t, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		buf.WriteByte(t) // Type (0=Single Word, 1=Quotable, 2=Greedy)
	case 6: // minecraft:entity
		flags, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		buf.WriteByte(flags)
	case 30: // minecraft:score_holder
		flags, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		buf.WriteByte(flags)
	case 42, 43, 44, 45, 46: // Variable String Types (resource_or_tag, etc.)
		var s packet.String
		if _, err := s.ReadFrom(r); err != nil {
			return nil, err
		}
		s.WriteTo(buf) // Store the varint string inside properties chunk!
	default:
		// Unknown or property-less parser
		// Standard Minecraft parsers generally don't carry extra bytes.
	}

	return buf.Bytes(), nil
}
