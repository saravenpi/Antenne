package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/saravenpi/antenne/internal/db"
	"github.com/saravenpi/antenne/internal/models"
)

// --- WebSocket ---

const (
	chatSendBuffer = 32
	maxNameLen     = 40
	maxBodyLen     = 500
)

// chatInMsg is the client->server message payload.
type chatInMsg struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Body string `json:"body"`
}

func (s *Server) handleChatWS(w http.ResponseWriter, r *http.Request) {
	isAdmin := false
	if _, err := s.auth.Authorize(r); err == nil {
		isAdmin = true
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &chatClient{
		conn:    conn,
		isAdmin: isAdmin,
		ip:      clientIP(r),
		send:    make(chan []byte, chatSendBuffer),
	}
	s.chat.register <- client

	// Writer goroutine drains the send channel.
	go func() {
		defer conn.Close()
		for payload := range client.send {
			if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		}
	}()

	defer func() { s.chat.unregister <- client }()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg chatInMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.Type != "message" {
			continue
		}
		s.handleChatIncoming(client, msg)
	}
}

// clientError enqueues an error event to a single client.
func (s *Server) clientError(c *chatClient, text string) {
	payload, _ := json.Marshal(map[string]string{"type": "error", "error": text})
	s.chat.sendOnly(c, payload)
}

func (s *Server) handleChatIncoming(c *chatClient, msg chatInMsg) {
	name := strings.TrimSpace(msg.Name)
	body := strings.TrimSpace(msg.Body)

	if len(name) < 1 || len(name) > maxNameLen {
		s.clientError(c, "Nom invalide (1 à 40 caractères).")
		return
	}
	if len(body) < 1 || len(body) > maxBodyLen {
		s.clientError(c, "Message invalide (1 à 500 caractères).")
		return
	}

	// Ban check.
	if s.isBanned(c.ip) {
		s.clientError(c, "Tu es banni du chat.")
		return
	}

	// Slow mode.
	settings := s.loadSettings()
	if !s.chat.allow(c.ip, settings.SlowModeSec) {
		s.clientError(c, "Doucement, attends quelques secondes.")
		return
	}

	// Banned words filter.
	body = filterBannedWords(body, parseBannedWords(settings.BannedWords))

	// Persist.
	message := models.ChatMessage{Name: name, Body: body, IP: c.ip}
	if err := s.db.Create(&message).Error; err != nil {
		s.clientError(c, "Erreur serveur.")
		return
	}

	adminPayload, publicPayload := chatMessagePayloads(message)

	if s.isRestricted(c.ip) {
		// Shadow-ban: echo back to sender only.
		if c.isAdmin {
			s.chat.sendOnly(c, adminPayload)
		} else {
			s.chat.sendOnly(c, publicPayload)
		}
		return
	}

	s.chat.broadcastEvent(adminPayload, publicPayload)
}

// chatMessagePayloads builds admin (with ip) and public (without ip) variants of
// a message event.
func chatMessagePayloads(m models.ChatMessage) (admin, public []byte) {
	base := map[string]any{
		"id":        m.ID,
		"name":      m.Name,
		"body":      m.Body,
		"createdAt": m.CreatedAt,
	}
	public, _ = json.Marshal(map[string]any{"type": "message", "message": base})

	adminMsg := map[string]any{
		"id":        m.ID,
		"name":      m.Name,
		"body":      m.Body,
		"createdAt": m.CreatedAt,
		"ip":        m.IP,
	}
	admin, _ = json.Marshal(map[string]any{"type": "message", "message": adminMsg})
	return admin, public
}

// --- banned words ---

func parseBannedWords(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ','
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if w := strings.TrimSpace(f); w != "" {
			out = append(out, w)
		}
	}
	return out
}

func filterBannedWords(body string, words []string) string {
	for _, w := range words {
		if w == "" {
			continue
		}
		for {
			idx := strings.Index(strings.ToLower(body), strings.ToLower(w))
			if idx < 0 {
				break
			}
			body = body[:idx] + strings.Repeat("*", len(w)) + body[idx+len(w):]
		}
	}
	return body
}

// --- moderation helpers ---

func (s *Server) isBanned(ip string) bool {
	var count int64
	s.db.Model(&models.Ban{}).Where("ip = ?", ip).Count(&count)
	return count > 0
}

func (s *Server) isRestricted(ip string) bool {
	var count int64
	s.db.Model(&models.Restriction{}).Where("ip = ?", ip).Count(&count)
	return count > 0
}

func (s *Server) loadSettings() models.Settings {
	var st models.Settings
	s.db.First(&st)
	return st
}

// --- REST: messages ---

func (s *Server) handleChatMessages(w http.ResponseWriter, r *http.Request) {
	var messages []models.ChatMessage
	if err := s.db.Where("deleted = ?", false).
		Order("created_at desc").Limit(100).Find(&messages).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	// Reverse to oldest -> newest.
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	writeJSON(w, http.StatusOK, messages)
}

func (s *Server) handleDeleteChatMessage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	res := s.db.Model(&models.ChatMessage{}).Where("id = ?", id).Update("deleted", true)
	if res.Error != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if res.RowsAffected == 0 {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	payload, _ := json.Marshal(map[string]string{"type": "delete", "id": id.String()})
	s.chat.broadcastRaw(payload)
	w.WriteHeader(http.StatusNoContent)
}

// --- REST: bans ---

type ipReasonReq struct {
	IP     string `json:"ip"`
	Reason string `json:"reason"`
}

func (s *Server) handleListBans(w http.ResponseWriter, r *http.Request) {
	var bans []models.Ban
	if err := s.db.Order("created_at desc").Find(&bans).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, bans)
}

func (s *Server) handleCreateBan(w http.ResponseWriter, r *http.Request) {
	var req ipReasonReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	ip := strings.TrimSpace(req.IP)
	if ip == "" {
		writeErr(w, http.StatusBadRequest, "ip required")
		return
	}
	ban := models.Ban{IP: ip, Reason: strings.TrimSpace(req.Reason)}
	if err := s.db.Create(&ban).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, ban)
}

func (s *Server) handleDeleteBan(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.db.Delete(&models.Ban{}, "id = ?", id).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- REST: restrictions ---

func (s *Server) handleListRestrictions(w http.ResponseWriter, r *http.Request) {
	var restrictions []models.Restriction
	if err := s.db.Order("created_at desc").Find(&restrictions).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, restrictions)
}

func (s *Server) handleCreateRestriction(w http.ResponseWriter, r *http.Request) {
	var req ipReasonReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	ip := strings.TrimSpace(req.IP)
	if ip == "" {
		writeErr(w, http.StatusBadRequest, "ip required")
		return
	}
	res := models.Restriction{IP: ip, Reason: strings.TrimSpace(req.Reason)}
	if err := s.db.Create(&res).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (s *Server) handleDeleteRestriction(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.db.Delete(&models.Restriction{}, "id = ?", id).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- REST: settings ---

type socialLink struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

// parseSocials decodes the stored JSON list, always returning a non-nil slice so
// the API emits `[]` rather than `null`.
func parseSocials(raw string) []socialLink {
	out := []socialLink{}
	if raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

type settingsJSON struct {
	StationName string       `json:"stationName"`
	BannedWords []string     `json:"bannedWords"`
	SlowModeSec int          `json:"slowModeSec"`
	Background  string       `json:"background"`
	Logo        string       `json:"logo"`
	Socials     []socialLink `json:"socials"`
}

func toSettingsJSON(st models.Settings) settingsJSON {
	return settingsJSON{
		StationName: st.StationName,
		BannedWords: parseBannedWords(st.BannedWords),
		SlowModeSec: st.SlowModeSec,
		Background:  st.Background,
		Logo:        st.Logo,
		Socials:     parseSocials(st.Socials),
	}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, toSettingsJSON(s.loadSettings()))
}

// handleAppearance is the PUBLIC subset of settings the listener page needs.
func (s *Server) handleAppearance(w http.ResponseWriter, r *http.Request) {
	st := s.loadSettings()
	writeJSON(w, http.StatusOK, map[string]any{
		"stationName": st.StationName,
		"background":  st.Background,
		"logo":        st.Logo,
		"socials":     parseSocials(st.Socials),
	})
}

type updateSettingsReq struct {
	StationName *string       `json:"stationName"`
	BannedWords *[]string     `json:"bannedWords"`
	SlowModeSec *int          `json:"slowModeSec"`
	Background  *string       `json:"background"`
	Logo        *string       `json:"logo"`
	Socials     *[]socialLink `json:"socials"`
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	st := s.loadSettings()
	if st.ID == 0 {
		if err := s.db.First(&st).Error; err != nil && !db.IsNotFound(err) {
			writeErr(w, http.StatusInternalServerError, "db error")
			return
		}
	}
	if req.StationName != nil {
		st.StationName = *req.StationName
	}
	if req.BannedWords != nil {
		st.BannedWords = strings.Join(*req.BannedWords, "\n")
	}
	if req.SlowModeSec != nil {
		st.SlowModeSec = *req.SlowModeSec
	}
	if req.Background != nil {
		st.Background = *req.Background
	}
	if req.Logo != nil {
		st.Logo = *req.Logo
	}
	if req.Socials != nil {
		b, _ := json.Marshal(*req.Socials)
		st.Socials = string(b)
	}
	if err := s.db.Save(&st).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, toSettingsJSON(st))
}
