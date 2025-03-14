package protocol

import (
	"encoding/json"
	"synchronize/internal/model"
	"time"
)

type MessageType string

const (
	TypeConnect    MessageType = "connect"
	TypeDisconnect MessageType = "disconnect"
	TypeInput      MessageType = "input"
	TypeState      MessageType = "state"
)

type Message struct {
	Type      MessageType     `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	ClientID  string          `json:"clientId,omitempty"`
	Sequence  uint64          `json:"sequence"`
	Data      json.RawMessage `json:"data,omitempty"`
}

type InputMessage struct {
	Direction model.Direction `json:"direction"`
	Sequence  uint64          `json:"sequence"`
}

type StateMessage struct {
	Avatars map[string]model.Avatar `json:"avatars"`
}
