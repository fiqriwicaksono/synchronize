package main

import (
	"log"
	"synchronize/internal/server"
	"time"
)

func main() {
	gameServer := server.NewServer(time.Second / 20)

	err := gameServer.Start(":8080")
	if err != nil {
		log.Fatalf("Failed to start game server: %v", err)
	}
}
