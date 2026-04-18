package main

import (
	"log"
	"savage-proxy/proxy"
)

func main() {
	// 1. Load configuration
	configPath := "config.json"
	if err := proxy.LoadConfig(configPath); err != nil {
		log.Printf("Config not found, generating default %s", configPath)
		if err := proxy.SaveDefaultConfig(configPath); err != nil {
			log.Fatalf("Failed to save default config: %v", err)
		}
		log.Fatalf("Please edit %s and restart the proxy.", configPath)
	}

	// 2. Start the proxy server
	server := proxy.NewServer(proxy.GlobalConfig.ListenAddr)
	server.ForwardingSecret = proxy.GlobalConfig.ForwardingSecret
	
	log.Printf("Starting Savage Proxy Foundation on %s...", proxy.GlobalConfig.ListenAddr)
	log.Printf("Backend target: %s", proxy.GlobalConfig.BackendAddr)

	if err := server.Listen(); err != nil {
		log.Fatalf("Critical server error: %v", err)
	}
}
