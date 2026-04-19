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

	playToClient := protocolData["play"].(map[string]interface{})["toClient"].(map[string]interface{})["types"].(map[string]interface{})["packet"]
    mappings := playToClient.([]interface{})[1].([]interface{})[0].(map[string]interface{})["type"].([]interface{})[1].(map[string]interface{})["mappings"]
    for id, name := range mappings.(map[string]interface{}) {
        if name == "start_configuration" || name == "packet_start_configuration" {
            fmt.Printf("Play ToClient: ID %s is %s\n", id, name)
        }
    }

	playToServer := protocolData["play"].(map[string]interface{})["toServer"].(map[string]interface{})["types"].(map[string]interface{})["packet"]
    mappingsS := playToServer.([]interface{})[1].([]interface{})[0].(map[string]interface{})["type"].([]interface{})[1].(map[string]interface{})["mappings"]
    for id, name := range mappingsS.(map[string]interface{}) {
        if name == "configuration_acknowledged" || name == "packet_configuration_acknowledged" || name == "acknowledge_configuration" {
            fmt.Printf("Play ToServer: ID %s is %s\n", id, name)
        }
    }
}
