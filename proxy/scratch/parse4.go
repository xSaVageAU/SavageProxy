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

	loginToServer := protocolData["login"].(map[string]interface{})["toServer"].(map[string]interface{})["types"].(map[string]interface{})["packet"]
    mappingsS := loginToServer.([]interface{})[1].([]interface{})[0].(map[string]interface{})["type"].([]interface{})[1].(map[string]interface{})["mappings"]
    for id, name := range mappingsS.(map[string]interface{}) {
		fmt.Printf("Login ToServer: ID %s is %s\n", id, name)
    }
}
