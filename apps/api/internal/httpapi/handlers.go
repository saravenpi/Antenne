package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/saravenpi/antenne/internal/db"
	"github.com/saravenpi/antenne/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// --- Auth ---

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	var admin models.Admin
	if err := s.db.Where("username = ?", req.Username).First(&admin).Error; err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)) != nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := s.auth.Issue(admin.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token, "username": admin.Username})
}

// --- Now playing (public) ---

type nowPlayingItem struct {
	Title   string `json:"title"`
	Artist  string `json:"artist"`
	TrackID string `json:"trackId"`
}

type nowPlayingResp struct {
	Live      bool            `json:"live"`
	Title     string          `json:"title"`
	Artist    string          `json:"artist"`
	TrackID   string          `json:"trackId"`
	Listeners int64           `json:"listeners"`
	Next      *nowPlayingItem `json:"next"`
}

func (s *Server) handleNowPlaying(w http.ResponseWriter, r *http.Request) {
	np := s.engine.NowPlaying()
	resp := nowPlayingResp{
		Live:      np.Live,
		Title:     np.Title,
		Artist:    np.Artist,
		TrackID:   np.TrackID,
		Listeners: int64(s.listeners.count()),
	}

	// Optional wall-clock resolution: `at` is unix millis of the listener's
	// playback position, accounting for HLS buffering delay.
	if atStr := r.URL.Query().Get("at"); atStr != "" {
		if at, err := strconv.ParseInt(atStr, 10, 64); err == nil && at > 0 {
			if title, artist, trackID, ok := s.engine.NowPlayingAt(time.UnixMilli(at)); ok {
				resp.Title = title
				resp.Artist = artist
				resp.TrackID = trackID
			}
		}
	}

	if title, artist, trackID, ok := s.engine.NextItem(); ok {
		resp.Next = &nowPlayingItem{Title: title, Artist: artist, TrackID: trackID}
	}

	writeJSON(w, http.StatusOK, resp)
}

// --- Tracks ---

func (s *Server) handleListTracks(w http.ResponseWriter, r *http.Request) {
	var tracks []models.Track
	if err := s.db.Order("position asc").Find(&tracks).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	sortTracks(tracks)
	writeJSON(w, http.StatusOK, tracks)
}

func (s *Server) handleUploadTrack(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(512 << 20); err != nil { // up to 512 MB
		writeErr(w, http.StatusBadRequest, "invalid upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()

	filename, err := s.store.Save(file, header.Filename)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not store file")
		return
	}

	title := r.FormValue("title")
	if title == "" {
		title = header.Filename
	}

	var maxPos int
	s.db.Model(&models.Track{}).Select("COALESCE(MAX(position), -1)").Scan(&maxPos)

	track := models.Track{
		Title:       title,
		Artist:      r.FormValue("artist"),
		Filename:    filename,
		DurationSec: s.store.Duration(filename),
		Position:    maxPos + 1,
	}
	if err := s.db.Create(&track).Error; err != nil {
		_ = s.store.Delete(filename)
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	_ = s.SyncPlaylist()
	writeJSON(w, http.StatusCreated, track)
}

func (s *Server) handleDeleteTrack(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var track models.Track
	if err := s.db.First(&track, "id = ?", id).Error; err != nil {
		if db.IsNotFound(err) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if err := s.db.Delete(&track).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	_ = s.store.Delete(track.Filename)
	_ = s.SyncPlaylist()
	w.WriteHeader(http.StatusNoContent)
}

// --- Playlist order ---

type reorderReq struct {
	Order []string `json:"order"` // track IDs in desired order
}

func (s *Server) handleReorderPlaylist(w http.ResponseWriter, r *http.Request) {
	var req reorderReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	for pos, idStr := range req.Order {
		id, err := uuid.Parse(idStr)
		if err != nil {
			continue
		}
		s.db.Model(&models.Track{}).Where("id = ?", id).Update("position", pos)
	}
	_ = s.SyncPlaylist()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

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

	if err := s.engine.Live.Start(); err != nil {
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
