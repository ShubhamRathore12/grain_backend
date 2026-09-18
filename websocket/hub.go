package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"grain_backend/middleware"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// A WebSocket upgrade is not subject to CORS, so without this check any
	// website could open a live telemetry stream using a logged-in operator's
	// cookie. Same allowlist as the HTTP layer. A missing Origin header (native
	// or server-side client) is allowed through; those callers still need a
	// valid session token.
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		if middleware.IsOriginAllowed(origin) {
			return true
		}
		log.Printf("ws: rejected upgrade from disallowed origin %q", origin)
		return false
	},
}

// Client represents a connected WebSocket client. UserID records which
// authenticated account owns the socket so streams can be attributed and, when
// per-machine stream scoping lands, filtered.
type Client struct {
	ID     string
	UserID int
	Conn   *websocket.Conn
	Send   chan []byte
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
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("PANIC in WebSocket hub Run, restarting: %v", rec)
			go h.Run() // self-heal: a crashed hub would silently stop all broadcasts
		}
	}()
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			h.Clients[client] = true
			count := len(h.Clients)
			h.mu.Unlock()
			log.Printf("WebSocket client connected. Total clients: %d", count)

		case client := <-h.Unregister:
			h.mu.Lock()
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.Send)
			}
			count := len(h.Clients)
			h.mu.Unlock()
			log.Printf("WebSocket client disconnected. Total clients: %d", count)

		case message := <-h.Broadcast:
			// Full write lock: slow clients are deleted from the map here, and a
			// delete under a read lock is a concurrent map write — a fatal error
			// that recover() cannot catch and would crash the whole process.
			h.mu.Lock()
			for client := range h.Clients {
				select {
				case client.Send <- message:
				default:
					// Client buffer full, drop the connection.
					close(client.Send)
					delete(h.Clients, client)
				}
			}
			h.mu.Unlock()
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

// HandleWebSocket authenticates the caller, then upgrades the connection.
//
// The live-data socket used to accept anyone. It is mounted behind
// middleware.AuthenticateToken now, and the claims are read here so a
// disconnected session cannot linger on an open stream (S-01, S-05).
func (h *WebSocketHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetUserFromContext(r)
	if !ok {
		// Reject before the upgrade so the client sees a normal 401 rather than a
		// socket that opens and immediately closes.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"success":false,"message":"Authentication required"}`))
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		ID:     time.Now().UTC().Format("20060102150405.000000") + "-" + claims.Username,
		UserID: claims.UserID,
		Conn:   conn,
		Send:   make(chan []byte, 256),
	}
	log.Printf("ws: client connected user=%d (%s)", claims.UserID, claims.Username)

	// Queue the initial message into the buffered channel BEFORE registering the
	// client with the hub. Once registered, a concurrent broadcast could close
	// client.Send, and writing to a closed channel panics. The client isn't in
	// the hub map yet here, so nothing can close Send during this write.
	initialMsg := map[string]interface{}{
		"type":      "connected",
		"data":      map[string]string{"status": "connected"},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	initialData, _ := json.Marshal(initialMsg)
	client.Send <- initialData

	h.Register <- client

	// Handle client read/write
	go h.writePump(client)
	go h.readPump(client)
}

func (h *WebSocketHub) readPump(client *Client) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("PANIC in WebSocket readPump: %v", rec)
		}
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
		if rec := recover(); rec != nil {
			log.Printf("PANIC in WebSocket writePump: %v", rec)
		}
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
