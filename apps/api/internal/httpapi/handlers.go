package httpapi

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
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
	Paused    bool            `json:"paused"`
	Title     string          `json:"title"`
	Artist    string          `json:"artist"`
	TrackID   string          `json:"trackId"`
	Listeners int64           `json:"listeners"`
	Next      *nowPlayingItem `json:"next"`
}

// currentNowPlaying builds the live snapshot (no wall-clock override).
func (s *Server) currentNowPlaying() nowPlayingResp {
	np := s.engine.NowPlaying()
	resp := nowPlayingResp{
		Live:      np.Live,
		Paused:    np.Paused,
		Title:     np.Title,
		Artist:    np.Artist,
		TrackID:   np.TrackID,
		Listeners: int64(s.listeners.count()),
	}
	if title, artist, trackID, ok := s.engine.NextItem(); ok {
		resp.Next = &nowPlayingItem{Title: title, Artist: artist, TrackID: trackID}
	}
	return resp
}

func (s *Server) handleNowPlaying(w http.ResponseWriter, r *http.Request) {
	np := s.engine.NowPlaying()
	resp := nowPlayingResp{
		Live:      np.Live,
		Paused:    np.Paused,
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

	// Prefer explicit form values; otherwise read the file's embedded tags so the
	// playlist shows the real title/artist instead of the raw filename.
	title := r.FormValue("title")
	artist := r.FormValue("artist")
	if title == "" || artist == "" {
		metaTitle, metaArtist := s.store.Metadata(filename)
		if title == "" {
			title = metaTitle
		}
		if artist == "" {
			artist = metaArtist
		}
	}
	if title == "" {
		title = strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	}

	var maxPos int
	s.db.Model(&models.Track{}).Select("COALESCE(MAX(position), -1)").Scan(&maxPos)

	track := models.Track{
		Title:       title,
		Artist:      artist,
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

// handleRescanMetadata re-reads every track's embedded tags via ffprobe and
// updates the stored title/artist when the file provides them. Backs the
// playlist "Nettoyer les métadonnées" button.
func (s *Server) handleRescanMetadata(w http.ResponseWriter, r *http.Request) {
	var tracks []models.Track
	if err := s.db.Order("position asc").Find(&tracks).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	for i := range tracks {
		title, artist := s.store.Metadata(tracks[i].Filename)
		updates := map[string]any{}
		if title != "" && title != tracks[i].Title {
			updates["title"] = title
			tracks[i].Title = title
		}
		if artist != "" && artist != tracks[i].Artist {
			updates["artist"] = artist
			tracks[i].Artist = artist
		}
		if len(updates) > 0 {
			s.db.Model(&models.Track{}).Where("id = ?", tracks[i].ID).Updates(updates)
		}
	}
	_ = s.SyncPlaylist()
	sortTracks(tracks)
	writeJSON(w, http.StatusOK, tracks)
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

// --- Playback control (manual DJ overrides on top of the 24/7 auto playlist) ---

func (s *Server) handlePlaybackNext(w http.ResponseWriter, r *http.Request) {
	s.engine.Playlist.Skip()
	writeJSON(w, http.StatusOK, s.currentNowPlaying())
}

func (s *Server) handlePlaybackPrev(w http.ResponseWriter, r *http.Request) {
	s.engine.Playlist.Prev()
	writeJSON(w, http.StatusOK, s.currentNowPlaying())
}

func (s *Server) handlePlaybackPause(w http.ResponseWriter, r *http.Request) {
	s.engine.Playlist.SetPaused(true)
	writeJSON(w, http.StatusOK, s.currentNowPlaying())
}

func (s *Server) handlePlaybackResume(w http.ResponseWriter, r *http.Request) {
	s.engine.Playlist.SetPaused(false)
	writeJSON(w, http.StatusOK, s.currentNowPlaying())
}

// handlePlayTrack jumps straight to a track (double-click "play now" in the
// régie) and resumes if paused.
func (s *Server) handlePlayTrack(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !s.engine.Playlist.PlayID(id) {
		writeErr(w, http.StatusNotFound, "track not found")
		return
	}
	writeJSON(w, http.StatusOK, s.currentNowPlaying())
}
