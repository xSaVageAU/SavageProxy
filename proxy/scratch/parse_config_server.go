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

	config, ok := protocolData["configuration"].(map[string]interface{})
	if !ok {
		panic("no config")
	}

	toServer, ok := config["toServer"].(map[string]interface{})
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
		fmt.Printf("Config ToServer %s: %s\n", id, name)
	}
}
