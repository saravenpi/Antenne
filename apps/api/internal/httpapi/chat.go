package httpapi

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// chatClient is a single connected websocket participant.
type chatClient struct {
	conn    *websocket.Conn
	isAdmin bool
	ip      string
	send    chan []byte
}

// chatHub manages connected chat clients and broadcasts events. It also tracks
// per-IP last-message times for slow-mode enforcement.
type chatHub struct {
	mu      sync.Mutex
	clients map[*chatClient]struct{}

	register   chan *chatClient
	unregister chan *chatClient

	rateMu  sync.Mutex
	lastMsg map[string]time.Time
}

func newChatHub() *chatHub {
	return &chatHub{
		clients:    make(map[*chatClient]struct{}),
		register:   make(chan *chatClient),
		unregister: make(chan *chatClient),
		lastMsg:    make(map[string]time.Time),
	}
}

func (h *chatHub) run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = struct{}{}
			h.mu.Unlock()
			h.broadcastListeners()
		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()
			h.broadcastListeners()
		}
	}
}

// listenerCount returns the number of connected clients.
func (h *chatHub) listenerCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// broadcastListeners pushes a listeners event to all clients.
func (h *chatHub) broadcastListeners() {
	payload, _ := json.Marshal(map[string]any{"type": "listeners", "count": h.listenerCount()})
	h.broadcastRaw(payload)
}

// broadcastRaw sends an already-marshalled payload to every client.
func (h *chatHub) broadcastRaw(payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		h.trySend(c, payload)
	}
}

// broadcastEvent marshals two variants of the event: admins receive the payload
// as-is (with the ip field), public clients receive adminOnly fields omitted.
// The caller passes the full event and a stripped public variant.
func (h *chatHub) broadcastEvent(adminPayload, publicPayload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if c.isAdmin {
			h.trySend(c, adminPayload)
		} else {
			h.trySend(c, publicPayload)
		}
	}
}

// trySend enqueues to a client's send channel, dropping the client if its buffer
// is full. Caller must hold h.mu.
func (h *chatHub) trySend(c *chatClient, payload []byte) {
	select {
	case c.send <- payload:
	default:
		// Slow client: drop and unregister.
		close(c.send)
		delete(h.clients, c)
	}
}

// sendOnly delivers a payload to a single client.
func (h *chatHub) sendOnly(c *chatClient, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; ok {
		h.trySend(c, payload)
	}
}

// allow reports whether the IP may post now given a slow-mode interval, and
// records the attempt time on success.
func (h *chatHub) allow(ip string, slowModeSec int) bool {
	if slowModeSec <= 0 {
		return true
	}
	h.rateMu.Lock()
	defer h.rateMu.Unlock()
	now := time.Now()
	if last, ok := h.lastMsg[ip]; ok {
		if now.Sub(last) < time.Duration(slowModeSec)*time.Second {
			return false
		}
	}
	h.lastMsg[ip] = now
	return true
}
