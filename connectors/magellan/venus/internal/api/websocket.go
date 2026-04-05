package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/kite-production/spark/services/venus/internal/store"
	"golang.org/x/net/websocket"
)

// WSHub manages WebSocket connections per channel.
type WSHub struct {
	store *store.Store
	mu    sync.RWMutex
	conns map[string][]*websocket.Conn // channelID -> connections
}

// NewWSHub creates a WebSocket hub and subscribes to new messages.
func NewWSHub(s *store.Store) *WSHub {
	hub := &WSHub{
		store: s,
		conns: make(map[string][]*websocket.Conn),
	}
	s.Subscribe(hub.broadcast)
	return hub
}

// RegisterRoutes adds WebSocket routes to the mux.
func (hub *WSHub) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /ws/channels/{id}", websocket.Handler(func(ws *websocket.Conn) {
		// Extract channel ID from the request path.
		channelID := ws.Request().PathValue("id")
		if channelID == "" {
			_ = ws.Close()
			return
		}

		hub.addConn(channelID, ws)
		defer hub.removeConn(channelID, ws)

		// Keep connection alive — read and discard client messages.
		buf := make([]byte, 1024)
		for {
			if _, err := ws.Read(buf); err != nil {
				return
			}
		}
	}))
}

func (hub *WSHub) addConn(channelID string, ws *websocket.Conn) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	hub.conns[channelID] = append(hub.conns[channelID], ws)
}

func (hub *WSHub) removeConn(channelID string, ws *websocket.Conn) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	conns := hub.conns[channelID]
	for i, c := range conns {
		if c == ws {
			hub.conns[channelID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}
	_ = ws.Close()
}

func (hub *WSHub) broadcast(msg *store.Message) {
	hub.mu.RLock()
	defer hub.mu.RUnlock()

	conns := hub.conns[msg.ChannelID]
	if len(conns) == 0 {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[ws] failed to marshal message: %v", err)
		return
	}

	for _, ws := range conns {
		go func(c *websocket.Conn) {
			if _, err := c.Write(data); err != nil {
				log.Printf("[ws] write failed: %v", err)
			}
		}(ws)
	}
}
