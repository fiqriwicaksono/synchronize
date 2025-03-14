package model

import (
	"time"
)

type Direction string

const (
	DirectionNone  Direction = "none"
	DirectionUp    Direction = "up"
	DirectionDown  Direction = "down"
	DirectionLeft  Direction = "left"
	DirectionRight Direction = "right"
)

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Avatar struct {
	ID            string    `json:"id"`
	Position      Position  `json:"position"`
	Direction     Direction `json:"direction"`
	Speed         float64   `json:"speed"`
	LastUpdated   time.Time `json:"lastUpdated"`
	LastInput     time.Time `json:"lastInput"`
	InputSequence uint64    `json:"inputSequence"`
}

func (a *Avatar) PredictPosition(now time.Time) Position {
	elapsed := now.Sub(a.LastInput).Seconds()

	if a.Direction == DirectionNone {
		return a.Position
	}

	newPos := Position{X: a.Position.X, Y: a.Position.Y}

	distance := a.Speed * elapsed

	switch a.Direction {
	case DirectionUp:
		newPos.Y -= distance
	case DirectionDown:
		newPos.Y += distance
	case DirectionLeft:
		newPos.X -= distance
	case DirectionRight:
		newPos.X += distance
	}

	return newPos
}

func (a *Avatar) Update(direction Direction, now time.Time, sequence uint64) {
	a.Direction = direction
	a.LastInput = now
	a.LastUpdated = now
	a.InputSequence = sequence

	a.Position = a.PredictPosition(now)
	a.LastUpdated = now
}
