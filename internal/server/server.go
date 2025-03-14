package server

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"synchronize/internal/model"
	"synchronize/pkg/protocol"
)

type Client struct {
	ID         string
	Connection *websocket.Conn
	SendQueue  chan []byte
}

type Server struct {
	clients    map[string]*Client
	avatars    map[string]*model.Avatar
	upgrader   websocket.Upgrader
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mutex      sync.RWMutex
	sequence   uint64
	tickRate   time.Duration
}

func NewServer(tickRate time.Duration) *Server {
	return &Server{
		clients:    make(map[string]*Client),
		avatars:    make(map[string]*model.Avatar),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		tickRate:   tickRate,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (s *Server) Start(addr string) error {
	go s.run()
	go s.gameTick()

	http.HandleFunc("/ws", s.handleWebSocket)

	http.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Server is running!"))
	})

	log.Printf("Starting server on %s", addr)

	return http.ListenAndServe(addr, nil)
}

func (s *Server) run() {
	for {
		select {
		case client := <-s.register:
			s.registerClient(client)
		case client := <-s.unregister:
			s.unregisterClient(client)
		case message := <-s.broadcast:
			s.broadcastMessage(message)
		}
	}
}

func (s *Server) gameTick() {
	ticker := time.NewTicker(s.tickRate)
	defer ticker.Stop()

	for {
		<-ticker.C
		s.updateState()
		s.broadcastState()
	}
}

// handleWebSocket upgrades HTTP connections to WebSocket
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error upgrading websocket connection: ", err)
		return
	}

	clientID := uuid.New().String()
	client := &Client{
		ID:         clientID,
		Connection: conn,
		SendQueue:  make(chan []byte, 256),
	}

	s.register <- client

	go s.readMessages(client)
	go s.writeMessages(client)
}

func (s *Server) readMessages(client *Client) {
	defer func() {
		s.unregister <- client
		client.Connection.Close()
	}()

	for {
		_, message, err := client.Connection.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Error reading message: %v", err)
			}
			break
		}

		s.handleMessage(client, message)
	}
}

func (s *Server) writeMessages(client *Client) {
	defer client.Connection.Close()

	for {
		message, ok := <-client.SendQueue
		if !ok {
			client.Connection.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}

		err := client.Connection.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			log.Printf("Error writing message: %v", err)
			return
		}
	}
}

func (s *Server) handleMessage(client *Client, data []byte) {
	var msg protocol.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("Error decoding message: %v", err)
		return
	}

	switch msg.Type {
	case protocol.TypeInput:
		var inputMsg protocol.InputMessage
		if err := json.Unmarshal(msg.Data, &inputMsg); err != nil {
			log.Printf("Error decoding input message: %v", err)
			return
		}
		s.handleInputMessage(client.ID, inputMsg, msg.Timestamp, msg.Sequence)
	}
}

func (s *Server) handleInputMessage(clientID string, inputMsg protocol.InputMessage, timestamp time.Time, sequence uint64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	avatar, exists := s.avatars[clientID]
	if !exists {
		return
	}

	if sequence > avatar.InputSequence {
		avatar.Update(inputMsg.Direction, timestamp, sequence)
	}
}

func (s *Server) registerClient(client *Client) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.clients[client.ID] = client

	s.avatars[client.ID] = &model.Avatar{
		ID:            client.ID,
		Position:      model.Position{X: float64(rand.Int63n(125-75) + 75), Y: float64(rand.Int63n(60-30) + 30)},
		Direction:     model.DirectionNone,
		Speed:         1.0,
		LastUpdated:   time.Now(),
		LastInput:     time.Now(),
		InputSequence: 0,
	}

	log.Printf("Client connected: %s", client.ID)

	s.sendInitialState(client)
}

func (s *Server) unregisterClient(client *Client) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, ok := s.clients[client.ID]; ok {
		delete(s.clients, client.ID)
		close(client.SendQueue)
	}

	delete(s.avatars, client.ID)
	log.Printf("Client disconnected: %s", client.ID)
}

// updateGameState updates all avatars based on their current state
func (s *Server) updateState() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now()

	for _, avatar := range s.avatars {
		avatar.Position = avatar.PredictPosition(now)
		avatar.LastUpdated = now
	}
}

// broadcastState sends the current state of all avatars to all connected clients
func (s *Server) broadcastState() {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	avatarsCopy := make(map[string]model.Avatar, len(s.avatars))
	for id, avatar := range s.avatars {
		avatarsCopy[id] = *avatar
	}

	stateMsg := protocol.StateMessage{
		Avatars: avatarsCopy,
	}

	stateData, err := json.Marshal(stateMsg)
	if err != nil {
		log.Printf("Error marshaling state data: %v", err)
		return
	}

	s.sequence++

	msg := protocol.Message{
		Type:      protocol.TypeState,
		Data:      stateData,
		Timestamp: time.Now(),
		Sequence:  s.sequence,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error encoding state message: %v", err)
		return
	}

	s.broadcast <- data
}

func (s *Server) sendInitialState(client *Client) {
	avatarsCopy := make(map[string]model.Avatar, len(s.avatars))
	for id, avatar := range s.avatars {
		avatarsCopy[id] = *avatar
	}

	stateMsg := protocol.StateMessage{
		Avatars: avatarsCopy,
	}

	stateData, err := json.Marshal(stateMsg)
	if err != nil {
		log.Printf("Error marshaling initial state data: %v", err)
		return
	}

	msg := protocol.Message{
		Type:      protocol.TypeState,
		Timestamp: time.Now(),
		Sequence:  s.sequence,
		Data:      stateData,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error encoding initial state message: %v", err)
		return
	}

	client.SendQueue <- data
}

func (s *Server) broadcastMessage(message []byte) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for _, client := range s.clients {
		select {
		case client.SendQueue <- message:
		default:
			close(client.SendQueue)
			delete(s.clients, client.ID)
		}
	}
}
