package main

import (
	"log"
	"savage-proxy/proxy"
)

func main() {
	// Root of our proxy foundation
	server := proxy.NewServer(":25565")
	
	log.Println("Starting Savage Proxy Foundation...")
	if err := server.Listen(); err != nil {
		log.Fatalf("Critical server error: %v", err)
	}
}
