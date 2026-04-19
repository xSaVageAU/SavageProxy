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

	playToClient := protocolData["play"].(map[string]interface{})["toClient"].(map[string]interface{})["types"].(map[string]interface{})
    for key, val := range playToClient {
        if key == "packet_start_configuration" || key == "packet_configuration_acknowledged" {
            fmt.Printf("PLAY ToClient %s: %v\n", key, val)
        }
    }
    
    playToServer := protocolData["play"].(map[string]interface{})["toServer"].(map[string]interface{})["types"].(map[string]interface{})
    for key, val := range playToServer {
        if key == "packet_start_configuration" || key == "packet_configuration_acknowledged" {
            fmt.Printf("PLAY ToServer %s: %v\n", key, val)
        }
    }

	configToClient := protocolData["configuration"].(map[string]interface{})["toClient"].(map[string]interface{})["types"].(map[string]interface{})
    for key, val := range configToClient {
        if key == "packet_start_configuration" || key == "packet_configuration_acknowledged" {
            fmt.Printf("CONFIG ToClient %s: %v\n", key, val)
        }
    }

	configToServer := protocolData["configuration"].(map[string]interface{})["toServer"].(map[string]interface{})["types"].(map[string]interface{})
    for key, val := range configToServer {
        if key == "packet_start_configuration" || key == "packet_configuration_acknowledged" {
            fmt.Printf("CONFIG ToServer %s: %v\n", key, val)
        }
    }
}
