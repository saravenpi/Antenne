package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/saravenpi/antenne/internal/models"
)

// --- Now playing (public) ---

type nowPlayingItem struct {
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	TrackID  string `json:"trackId"`
	CoverURL string `json:"coverUrl,omitempty"`
}

type nowPlayingResp struct {
	Live      bool            `json:"live"`
	Paused    bool            `json:"paused"`
	Title     string          `json:"title"`
	Artist    string          `json:"artist"`
	TrackID   string          `json:"trackId"`
	CoverURL  string          `json:"coverUrl,omitempty"`
	Listeners int64           `json:"listeners"`
	Next      *nowPlayingItem `json:"next"`
}

// coverURLFor returns the public cover URL for a track ID, or "" when the track
// has no embedded art.
func (s *Server) coverURLFor(trackID string) string {
	if trackID == "" {
		return ""
	}
	var t models.Track
	if err := s.db.Select("id", "cover").First(&t, "id = ?", trackID).Error; err != nil || t.Cover == "" {
		return ""
	}
	return trackCoverURL(t.ID, t.Cover)
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
		resp.Next = &nowPlayingItem{Title: title, Artist: artist, TrackID: trackID, CoverURL: s.coverURLFor(trackID)}
	}
	resp.CoverURL = s.coverURLFor(resp.TrackID)
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
		resp.Next = &nowPlayingItem{Title: title, Artist: artist, TrackID: trackID, CoverURL: s.coverURLFor(trackID)}
	}
	resp.CoverURL = s.coverURLFor(resp.TrackID)

	writeJSON(w, http.StatusOK, resp)
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
