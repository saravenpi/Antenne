package httpapi

import (
	"net/http"

	"github.com/gorilla/websocket"
)

// --- Live broadcast ---

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// handleLiveIngest upgrades to a WebSocket, authorises via the `token` query
// param, then streams the admin's encoded mic chunks into the live source.
func (s *Server) handleLiveIngest(w http.ResponseWriter, r *http.Request) {
	if _, err := s.auth.Authorize(r); err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Reject a second concurrent ingest: two browser streams fed into the single
	// decoder would interleave and corrupt the broadcast. A rapid double-click on
	// "prendre l'antenne" is the usual cause.
	started, err := s.engine.Live.Start()
	if err != nil {
		return
	}
	if !started {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"already live"}`))
		return
	}
	defer s.engine.Live.Stop()

	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if mt == websocket.BinaryMessage {
			if err := s.engine.Live.Write(data); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleLiveStop(w http.ResponseWriter, r *http.Request) {
	s.engine.Live.Stop()
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}
