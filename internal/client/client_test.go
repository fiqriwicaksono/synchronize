package client

import (
	"testing"
	"time"

	"synchronize/internal/model"
)

func TestLocalPrediction(t *testing.T) {
	c := NewClient("localhost:8080")
	c.playerID = "test-player"
	c.state.LocalAvatar = &model.Avatar{
		ID:            "test-player",
		Position:      model.Position{X: 100, Y: 100},
		Direction:     model.DirectionNone,
		Speed:         150,
		LastUpdated:   time.Now(),
		LastInput:     time.Now(),
		InputSequence: 0,
	}

	// Apply input to move right
	c.applyLocalInput(model.DirectionRight, 1)

	// Wait a short time
	time.Sleep(100 * time.Millisecond)

	// Update local prediction
	c.updateLocalPredictions()

	// Position should have changed
	if c.state.LocalAvatar.Position.X <= 100 {
		t.Errorf("Expected X position to increase, got %f", c.state.LocalAvatar.Position.X)
	}
	if c.state.LocalAvatar.Position.Y != 100 {
		t.Errorf("Expected Y position to stay 100, got %f", c.state.LocalAvatar.Position.Y)
	}
}
