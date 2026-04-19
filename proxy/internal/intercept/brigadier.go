package intercept

import (
	"bytes"
	"fmt"
	"log"

	"github.com/Tnze/go-mc/net/packet"
)

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
