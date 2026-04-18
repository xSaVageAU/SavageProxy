package proxy

import (
	"encoding/json"
	"os"
)

type Config struct {
	ListenAddr       string `json:"listen_addr"`
	BackendAddr      string `json:"backend_addr"`
	ForwardingSecret string `json:"forwarding_secret"`
}

var GlobalConfig Config

func LoadConfig(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	return decoder.Decode(&GlobalConfig)
}

func SaveDefaultConfig(path string) error {
	defaultConfig := Config{
		ListenAddr:       ":25565",
		BackendAddr:      "127.0.0.1:25566",
		ForwardingSecret: "savage_secret_key_2026",
	}

	data, err := json.MarshalIndent(defaultConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
