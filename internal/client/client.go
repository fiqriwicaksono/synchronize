package client

import (
	"encoding/json"
	"log"
	"net/url"
	"sync"
	"synchronize/internal/model"
	"synchronize/pkg/protocol"
	"time"

	"github.com/gorilla/websocket"
)

type GameState struct {
	LocalAvatar    *model.Avatar
	ServerPosition model.Position
	OtherAvatars   map[string]*model.Avatar
	mutex          sync.RWMutex
	inputSeq       uint64
}

type Client struct {
	conn             *websocket.Conn
	serverAddr       string
	playerID         string
	state            *GameState
	sendCh           chan []byte
	pendingInputs    []protocol.InputMessage
	pendingInputsMu  sync.Mutex
	lastReconcileSeq uint64
	done             chan struct{}
}

func NewClient(serverAddr string) *Client {
	return &Client{
		serverAddr: serverAddr,
		state: &GameState{
			OtherAvatars:   make(map[string]*model.Avatar),
			ServerPosition: model.Position{X: 0, Y: 0},
		},
		sendCh: make(chan []byte, 100),
		done:   make(chan struct{}),
	}
}

func (c *Client) Connect() error {
	u := url.URL{Scheme: "ws", Host: c.serverAddr, Path: "/ws"}
	log.Printf("Connecting to %s", u.String())

	var err error
	c.conn, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}

	go c.readMessages()
	go c.writeMessages()
	go c.localUpdate()

	return nil
}

func (c *Client) Close() {
	close(c.done)
	if c.conn != nil {
		c.conn.Close()
	}
}

func (c *Client) SendInput(direction model.Direction) {
	c.state.mutex.Lock()
	c.state.inputSeq++
	seq := c.state.inputSeq
	c.state.mutex.Unlock()

	c.applyLocalInput(direction, seq)

	inputMsg := protocol.InputMessage{
		Direction: direction,
	}

	c.pendingInputsMu.Lock()
	c.pendingInputs = append(c.pendingInputs, inputMsg)
	c.pendingInputsMu.Unlock()

	inputData, err := json.Marshal(inputMsg)
	if err != nil {
		log.Printf("Error marshaling input data: %v", err)
		return
	}

	msg := protocol.Message{
		Type:      protocol.TypeInput,
		Timestamp: time.Now(),
		ClientID:  c.playerID,
		Sequence:  seq,
		Data:      inputData,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error encoding input message: %v", err)
		return
	}

	select {
	case c.sendCh <- data:
	default:
		log.Println("Message buffer full, dropping input")
	}
}

func (c *Client) applyLocalInput(direction model.Direction, sequence uint64) {
	c.state.mutex.Lock()
	defer c.state.mutex.Unlock()

	if c.state.LocalAvatar != nil {
		c.state.LocalAvatar.Update(direction, time.Now(), sequence)
	}
}

// GetGameState returns a safe copy of the current game state
func (c *Client) GetGameState() (model.Avatar, model.Position, map[string]model.Avatar) {
	// Default return values
	emptyAvatar := model.Avatar{}
	emptyMap := make(map[string]model.Avatar)
	emptyPosition := model.Position{}

	if c.state == nil {
		log.Println("WARNING: GetGameState called with nil state")
		return emptyAvatar, emptyPosition, emptyMap
	}

	c.state.mutex.RLock()
	defer c.state.mutex.RUnlock()

	// Handle local avatar
	localAvatar := emptyAvatar
	if c.state.LocalAvatar != nil {
		localAvatar = *c.state.LocalAvatar
	}
	serverPos := c.state.ServerPosition

	// Handle other avatars
	otherAvatars := make(map[string]model.Avatar)
	if c.state.OtherAvatars != nil {
		for id, avatar := range c.state.OtherAvatars {
			if avatar != nil {
				otherAvatars[id] = *avatar
			}
		}
	}

	return localAvatar, serverPos, otherAvatars
}

func (c *Client) readMessages() {
	defer c.conn.Close()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading from server: %v", err)
			return
		}

		c.handleServerMessage(message)
	}
}

func (c *Client) writeMessages() {
	ticker := time.NewTicker(time.Second / 30)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.sendCh:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			err := c.conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				log.Printf("Error writing to server: %v", err)
				return
			}

		case <-ticker.C:
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Error sending ping: %v", err)
				return
			}

		case <-c.done:
			return
		}
	}
}

// handleServerMessage processes messages received from the server
func (c *Client) handleServerMessage(data []byte) {
	if data == nil || len(data) == 0 {
		log.Println("Received empty message data")
		return
	}

	log.Printf("Received message of length %d", len(data))

	var msg protocol.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("Error unmarshaling server message: %v", err)
		return
	}

	log.Printf("Message type: %s, sequence: %d", msg.Type, msg.Sequence)

	switch msg.Type {
	case protocol.TypeState:
		if len(msg.Data) == 0 {
			log.Println("Warning: Received state message with empty data")
			return
		}

		var stateMsg protocol.StateMessage
		if err := json.Unmarshal(msg.Data, &stateMsg); err != nil {
			log.Printf("Error unmarshaling state message: %v", err)
			return
		}

		// Call with a defensive try-catch wrapper
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("PANIC in handleStateUpdate: %v", r)
				}
			}()
			c.handleStateUpdate(stateMsg, msg.Sequence)
		}()
	}
}

// handleStateUpdate processes state updates from the server
func (c *Client) handleStateUpdate(stateMsg protocol.StateMessage, serverSequence uint64) {
	log.Printf("Starting state update processing, sequence: %d", serverSequence)

	// Lock for the entire update process
	c.state.mutex.Lock()
	defer c.state.mutex.Unlock()

	// Make sure we have an initialized client state
	if c.state == nil {
		log.Println("ERROR: Client state is nil, creating new state")
		c.state = &GameState{
			OtherAvatars: make(map[string]*model.Avatar),
		}
	}

	// Initialize OtherAvatars if nil
	if c.state.OtherAvatars == nil {
		log.Println("Initializing empty OtherAvatars map")
		c.state.OtherAvatars = make(map[string]*model.Avatar)
	}

	// Defensive check for nil Avatars map in the state message
	if stateMsg.Avatars == nil {
		log.Println("WARNING: Received state update with nil Avatars map")
		return
	}

	log.Printf("State update has %d avatars", len(stateMsg.Avatars))

	// If we're receiving our first state update, determine our player ID
	if c.playerID == "" && len(stateMsg.Avatars) > 0 {
		// Assign first avatar as ours
		for id := range stateMsg.Avatars {
			c.playerID = id
			log.Printf("Assigned player ID: %s", c.playerID)
			break
		}
	}

	// Update local avatar
	if c.playerID != "" {
		avatar, exists := stateMsg.Avatars[c.playerID]
		if exists {
			if c.state.LocalAvatar == nil {
				log.Printf("Creating new local avatar with ID: %s", c.playerID)
				c.state.LocalAvatar = &model.Avatar{
					ID:            c.playerID,
					Position:      avatar.Position,
					Direction:     avatar.Direction,
					Speed:         avatar.Speed,
					LastUpdated:   avatar.LastUpdated,
					LastInput:     avatar.LastInput,
					InputSequence: avatar.InputSequence,
				}
				c.state.ServerPosition = avatar.Position
			} else {
				log.Printf("Reconciling local avatar, server seq: %d, input seq: %d",
					serverSequence, avatar.InputSequence)

				c.reconcileState(avatar, serverSequence)
			}
		} else {
			log.Printf("Warning: My avatar ID %s not found in update", c.playerID)
		}
	}

	// Safe logic for other avatars
	for id, serverAvatar := range stateMsg.Avatars {
		// Skip our own avatar
		if id == c.playerID {
			continue
		}

		log.Printf("Processing other avatar: %s", id)

		// Deep copy the avatar data to prevent any shared references
		avatarCopy := model.Avatar{
			ID:            serverAvatar.ID,
			Position:      serverAvatar.Position,
			Direction:     serverAvatar.Direction,
			Speed:         serverAvatar.Speed,
			LastUpdated:   serverAvatar.LastUpdated,
			LastInput:     serverAvatar.LastInput,
			InputSequence: serverAvatar.InputSequence,
		}

		// Update or create the avatar entry
		existingAvatar, exists := c.state.OtherAvatars[id]
		if !exists || existingAvatar == nil {
			log.Printf("Creating new other avatar: %s", id)
			c.state.OtherAvatars[id] = &model.Avatar{
				ID:            avatarCopy.ID,
				Position:      avatarCopy.Position,
				Direction:     avatarCopy.Direction,
				Speed:         avatarCopy.Speed,
				LastUpdated:   avatarCopy.LastUpdated,
				LastInput:     avatarCopy.LastInput,
				InputSequence: avatarCopy.InputSequence,
			}
		} else {
			// Update existing avatar
			existingAvatar.Position = avatarCopy.Position
			existingAvatar.Direction = avatarCopy.Direction
			existingAvatar.Speed = avatarCopy.Speed
			existingAvatar.LastUpdated = avatarCopy.LastUpdated
			existingAvatar.LastInput = avatarCopy.LastInput
			existingAvatar.InputSequence = avatarCopy.InputSequence
		}
	}

	// Safely clean up avatars that no longer exist
	for id := range c.state.OtherAvatars {
		if _, exists := stateMsg.Avatars[id]; !exists {
			log.Printf("Removing avatar that no longer exists: %s", id)
			delete(c.state.OtherAvatars, id)
		}
	}

	log.Printf("Completed state update processing")
}

func (c *Client) reconcileState(serverAvatar model.Avatar, serverSequence uint64) {
	c.state.ServerPosition = serverAvatar.Position

	if serverSequence < c.lastReconcileSeq {
		log.Printf("Received out-of-order state update, ignoring (got %d, last was %d)",
			serverSequence, c.lastReconcileSeq)
		return
	}

	// Update our tracking of the latest server state processed
	c.lastReconcileSeq = serverSequence
	if serverAvatar.InputSequence >= c.state.inputSeq {
		c.state.LocalAvatar.Position = serverAvatar.Position
		c.state.LocalAvatar.Direction = serverAvatar.Direction
		c.state.LocalAvatar.LastUpdated = serverAvatar.LastUpdated
		c.state.LocalAvatar.LastInput = serverAvatar.LastInput
		c.state.LocalAvatar.InputSequence = serverAvatar.InputSequence
		c.lastReconcileSeq = serverAvatar.InputSequence

		c.pendingInputsMu.Lock()
		c.pendingInputs = nil
		c.pendingInputsMu.Unlock()
		return
	}

	c.pendingInputsMu.Lock()
	var remainingInputs []protocol.InputMessage
	for _, input := range c.pendingInputs {
		if input.Sequence > c.state.inputSeq {
			remainingInputs = append(remainingInputs, input)
		}
	}
	c.pendingInputs = remainingInputs
	c.pendingInputsMu.Unlock()

	c.state.LocalAvatar.Position = serverAvatar.Position
	c.state.LocalAvatar.Direction = serverAvatar.Direction
	c.state.LocalAvatar.LastUpdated = serverAvatar.LastUpdated
	c.state.LocalAvatar.LastInput = serverAvatar.LastInput
	c.state.LocalAvatar.InputSequence = serverAvatar.InputSequence

	now := time.Now()
	for _, input := range remainingInputs {
		c.state.LocalAvatar.Update(input.Direction, now, input.Sequence)
	}
}

func (c *Client) localUpdate() {
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.updateLocalPredictions()
		case <-c.done:
			return
		}
	}
}

func (c *Client) updateLocalPredictions() {
	c.state.mutex.Lock()
	defer c.state.mutex.Unlock()

	now := time.Now()

	if c.state.LocalAvatar != nil && c.state.LocalAvatar.Direction != model.DirectionNone {
		c.state.LocalAvatar.Position = c.state.LocalAvatar.PredictPosition(now)
		c.state.LocalAvatar.LastUpdated = now
	}

	for _, avatar := range c.state.OtherAvatars {
		if avatar.Direction != model.DirectionNone {
			avatar.Position = avatar.PredictPosition(now)
			avatar.LastUpdated = now
		}
	}
}
