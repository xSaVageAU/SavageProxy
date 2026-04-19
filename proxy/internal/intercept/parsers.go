package intercept

import (
	"bytes"
	"log"

	"github.com/Tnze/go-mc/net/packet"
)

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
		log.Printf("[ProxyBrigadier] WARNING: Unknown parser ID %d — assuming 0 property bytes. Graph may be corrupted!", parserID)
		return nil
	}
}

// skipNumberParser skips a brigadier number parser (float/double/int/long).
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
