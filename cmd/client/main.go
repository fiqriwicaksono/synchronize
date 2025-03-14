package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"synchronize/internal/client"
	"synchronize/internal/model"
	"time"

	"github.com/gdamore/tcell/v2"
)

func main() {
	// Add command-line flag for logging
	debugMode := flag.Bool("debug", false, "Enable debug logging")
	logFile := flag.String("logfile", "client.log", "Log file path")
	flag.Parse()

	if *debugMode {
		// Set up logging to file
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening log file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		log.SetOutput(f)
	} else {
		// Disable logging in non-debug mode
		log.SetOutput(io.Discard)
	}

	serverAddr := os.Getenv("SERVER_HOST")
	if serverAddr == "" {
		serverAddr = "localhost:8080"
	}

	log.Printf("debug")
	gameClient := client.NewClient(serverAddr)

	if err := gameClient.Connect(); err != nil {
		log.Fatalf("Failed to connect to game server: %v", err)
	}
	defer gameClient.Close()

	screen, err := tcell.NewScreen()
	if err != nil {
		log.Fatalf("Failed to initialize screen: %v", err)
	}
	if err := screen.Init(); err != nil {
		log.Fatalf("Failed to initialize screen: %v", err)
	}
	defer screen.Fini()

	defStyle := tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)
	screen.SetStyle(defStyle)

	drawTicker := time.NewTicker(time.Second / 30)
	quit := make(chan struct{})

	go func() {
		// Track current key state
		var currentDirection model.Direction = model.DirectionNone

		for {
			ev := screen.PollEvent()
			switch ev := ev.(type) {
			case *tcell.EventKey:
				var newDirection model.Direction

				// Handle key down events
				switch ev.Key() {
				case tcell.KeyEscape, tcell.KeyCtrlC:
					close(quit)
					return
				case tcell.KeyUp:
					newDirection = model.DirectionUp
				case tcell.KeyDown:
					newDirection = model.DirectionDown
				case tcell.KeyLeft:
					newDirection = model.DirectionLeft
				case tcell.KeyRight:
					newDirection = model.DirectionRight
				default:
					// Any other key results in stopping
					newDirection = model.DirectionNone
				}

				// Only send input if direction changed
				if newDirection != currentDirection {
					currentDirection = newDirection
					gameClient.SendInput(currentDirection)
				}

			case *tcell.EventResize:
				screen.Sync()
			}
		}
	}()

	for {
		select {
		case <-drawTicker.C:
			drawGame(screen, gameClient)
		case <-quit:
			return
		}
	}
}

// In cmd/client/main.go
func drawGame(screen tcell.Screen, client *client.Client) {
	screen.Clear()

	// Get current game state including server position
	localAvatar, serverPos, otherAvatars := client.GetGameState()

	// Draw local avatar (as @)
	localX, localY := int(localAvatar.Position.X), int(localAvatar.Position.Y)
	screen.SetContent(localX, localY, '@', nil, tcell.StyleDefault.Foreground(tcell.ColorGreen))

	// Draw server position (as #)
	serverX, serverY := int(serverPos.X), int(serverPos.Y)
	screen.SetContent(serverX, serverY, '#', nil, tcell.StyleDefault.Foreground(tcell.ColorRed))

	// Draw other avatars (as O)
	for _, avatar := range otherAvatars {
		x, y := int(avatar.Position.X), int(avatar.Position.Y)
		screen.SetContent(x, y, 'O', nil, tcell.StyleDefault.Foreground(tcell.ColorBlue))
	}

	// Draw instructions
	drawText(screen, 0, 0, tcell.StyleDefault, "Use arrow keys to move, ESC to quit")

	// Show both positions
	clientPosText := fmt.Sprintf("Client Position: (%d, %d)", localX, localY)
	serverPosText := fmt.Sprintf("Server Position: (%d, %d)", serverX, serverY)
	drawText(screen, 0, 1, tcell.StyleDefault, clientPosText)
	drawText(screen, 0, 2, tcell.StyleDefault, serverPosText)

	// Add a legend
	drawText(screen, 0, 4, tcell.StyleDefault, "@ = Client Prediction  # = Server Position  O = Other Players")

	screen.Show()
}

// drawText draws a string at the specified position
func drawText(screen tcell.Screen, x, y int, style tcell.Style, text string) {
	for i, r := range text {
		screen.SetContent(x+i, y, r, nil, style)
	}
}
