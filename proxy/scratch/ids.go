package main

import (
	"fmt"
	"github.com/Tnze/go-mc/data/packetid"
)

func main() {
	fmt.Printf("StartConfiguration (PlayClientbound): 0x%X\n", packetid.ClientboundPlayStartConfiguration)
	fmt.Printf("AcknowledgeConfiguration (PlayServerbound): 0x%X\n", packetid.ServerboundPlayConfigurationAcknowledged)
}
