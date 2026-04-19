package protocol

import (
	"bytes"
	"io"
)

// ============================================================
// 26.1.1 PACKET ID CONSTANTS
// ============================================================
// These must match the actual protocol version in use (775).
const (
	// Serverbound Play
	SB_CHAT_COMMAND int32 = 0x07 // Unsigned chat command
	SB_TAB_COMPLETE int32 = 0x0E // Tab complete / command suggestions request

	// Clientbound Play
	CB_TAB_COMPLETE     int32 = 0x0F // Tab complete / command suggestions response
	CB_DECLARE_COMMANDS int32 = 0x10 // Brigadier command graph
	CB_SYSTEM_CHAT      int32 = 0x79 // System chat message
)

// This file contains your own native implementation of the Minecraft Protocol.
// By using these helpers, the proxy can construct and send its own packets
// without being dependent on external libraries for internal logic.

// WriteVarInt writes an integer in the Minecraft VarInt format.
func WriteVarInt(w io.Writer, value int32) error {
	uValue := uint32(value)
	for {
		if (uValue & ^uint32(0x7F)) == 0 {
			_, err := w.Write([]byte{byte(uValue)})
			return err
		}
		_, err := w.Write([]byte{byte((uValue & 0x7F) | 0x80)})
		if err != nil {
			return err
		}
		uValue >>= 7
	}
}

// WriteString writes a string prefixed by its length as a VarInt.
func WriteString(w io.Writer, s string) error {
	data := []byte(s)
	if err := WriteVarInt(w, int32(len(data))); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

// EncodePacket creates a raw Minecraft packet with [Length][ID][Data].
// This allows you to "inject" packets directly into the TCP stream.
func EncodePacket(id int32, data []byte) []byte {
	idBuf := new(bytes.Buffer)
	WriteVarInt(idBuf, id)

	payload := idBuf.Bytes()
	payload = append(payload, data...)

	lengthBuf := new(bytes.Buffer)
	WriteVarInt(lengthBuf, int32(len(payload)))

	return append(lengthBuf.Bytes(), payload...)
}
