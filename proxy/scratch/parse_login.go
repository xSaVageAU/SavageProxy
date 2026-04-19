package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	data, err := os.ReadFile("protocol.json")
	if err != nil {
		panic(err)
	}

	var protocolData map[string]interface{}
	if err := json.Unmarshal(data, &protocolData); err != nil {
		panic(err)
	}

	login, ok := protocolData["login"].(map[string]interface{})
	if !ok {
		panic("no login")
	}

	toServer, ok := login["toServer"].(map[string]interface{})
	if !ok {
		panic("no toServer")
	}

	types, ok := toServer["types"].(map[string]interface{})
	if !ok {
		panic("no types")
	}

	packet := types["packet"].([]interface{})
	mappings := packet[1].([]interface{})[0].(map[string]interface{})["type"].([]interface{})[1].(map[string]interface{})["mappings"]
	
	for id, name := range mappings.(map[string]interface{}) {
		if name == "login_acknowledged" || name == "packet_login_acknowledged" || name == "login_acknowledged_packet" {
			fmt.Printf("Login ToServer %s: %s\n", id, name)
		}
	}
}
