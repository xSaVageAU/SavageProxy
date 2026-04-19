package protocol

import (
	"fmt"
)

// MinecraftHash implements Mojang's "Two's Complement" SHA-1 hex format.
// It is used during the authentication handshake to verify server identity.
func MinecraftHash(data []byte) string {
	var negative bool
	if data[0] >= 0x80 {
		negative = true
		// Two's complement: flip bits and add 1
		carry := true
		for i := len(data) - 1; i >= 0; i-- {
			data[i] = ^data[i]
			if carry {
				data[i]++
				carry = (data[i] == 0)
			}
		}
	}
	res := fmt.Sprintf("%x", data)
	res = fmt.Sprintf("%040s", res) // Pad with zeros
	// Trim leading zeros like Mojang does
	for len(res) > 0 && res[0] == '0' {
		res = res[1:]
	}
	if negative {
		res = "-" + res
	}
	return res
}
