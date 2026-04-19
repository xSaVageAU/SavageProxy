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

	play, ok := protocolData["play"].(map[string]interface{})
	if !ok {
		panic("no play")
	}

	toClient, ok := play["toClient"].(map[string]interface{})
	if !ok {
		panic("no toClient")
	}

	types, ok := toClient["types"].(map[string]interface{})
	if !ok {
		panic("no types")
	}

	for key, val := range types {
		if fmt.Sprintf("%v", key) == "packet" {
            fields := val.([]interface{})[1].([]interface{})[0].(map[string]interface{})["type"].([]interface{})[1].(map[string]interface{})["mappings"]
			for id, mapping := range fields.(map[string]interface{}) {
				fmt.Printf("%s: %s\n", id, mapping)
			}
		}
	}
}
