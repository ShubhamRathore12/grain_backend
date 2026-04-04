package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for WebSocket connections
	},
}

// Client represents a connected WebSocket client
type Client struct {
	ID   string
	Conn *websocket.Conn
	Send chan []byte
}

// WebSocketHub manages all WebSocket connections
type WebSocketHub struct {
	Clients    map[*Client]bool
	Broadcast  chan []byte
	Register   chan *Client
	Unregister chan *Client
	mu         sync.RWMutex
}

// NewWebSocketHub creates a new WebSocket hub
func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{
		Clients:    make(map[*Client]bool),
		Broadcast:  make(chan []byte, 100),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
	}
}

// Run starts the WebSocket hub
func (h *WebSocketHub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			h.Clients[client] = true
			h.mu.Unlock()
			log.Printf("WebSocket client connected. Total clients: %d", len(h.Clients))

		case client := <-h.Unregister:
			h.mu.Lock()
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.Send)
			}
			h.mu.Unlock()
			log.Printf("WebSocket client disconnected. Total clients: %d", len(h.Clients))

		case message := <-h.Broadcast:
			h.mu.RLock()
			for client := range h.Clients {
				select {
				case client.Send <- message:
				default:
					// Client buffer full, close connection
					close(client.Send)
					delete(h.Clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// BroadcastToAll sends a message to all connected clients
func (h *WebSocketHub) BroadcastToAll(data interface{}) {
	message, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshaling broadcast message: %v", err)
		return
	}

	select {
	case h.Broadcast <- message:
	default:
		log.Printf("Broadcast channel full, message dropped")
	}
}

// HandleWebSocket handles WebSocket upgrade and connection management
func (h *WebSocketHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		ID:   time.Now().Format("2006-01-02-15-04-05"),
		Conn: conn,
		Send: make(chan []byte, 256),
	}

	h.Register <- client

	// Send initial connection message
	initialMsg := map[string]interface{}{
		"type":      "connected",
		"data":      map[string]string{"status": "connected"},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	initialData, _ := json.Marshal(initialMsg)
	client.Send <- initialData

	// Handle client read/write
	go h.writePump(client)
	go h.readPump(client)
}

func (h *WebSocketHub) readPump(client *Client) {
	defer func() {
		h.Unregister <- client
		client.Conn.Close()
	}()

	for {
		_, message, err := client.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			break
		}
		log.Printf("Received from %s: %s", client.ID, string(message))
	}
}

func (h *WebSocketHub) writePump(client *Client) {
	defer func() {
		client.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.Send:
			if !ok {
				// Channel closed
				client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := client.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("WebSocket write error: %v", err)
				return
			}
		}
	}
}
