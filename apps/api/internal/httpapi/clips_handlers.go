package httpapi

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/saravenpi/poste/internal/audio"
	"github.com/saravenpi/poste/internal/db"
	"github.com/saravenpi/poste/internal/models"
)

// clipJSON is the API representation of a Clip, adding the public audio URL.
type clipJSON struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	DurationSec float64   `json:"durationSec"`
	CreatedAt   time.Time `json:"createdAt"`
	URL         string    `json:"url"`
}

func toClipJSON(c models.Clip) clipJSON {
	return clipJSON{
		ID:          c.ID,
		Title:       c.Title,
		DurationSec: c.DurationSec,
		CreatedAt:   c.CreatedAt,
		URL:         fmt.Sprintf("/api/clips/%s/audio", c.ID),
	}
}

type createClipReq struct {
	Seconds int    `json:"seconds"`
	Title   string `json:"title"`
}

func (s *Server) handleCreateClip(w http.ResponseWriter, r *http.Request) {
	var req createClipReq
	if err := decode(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	seconds := req.Seconds
	if seconds < 5 {
		seconds = 5
	}
	if seconds > 120 {
		seconds = 120
	}

	pcm := s.engine.ExtractClipPCM(seconds)
	if len(pcm) == 0 {
		writeErr(w, http.StatusServiceUnavailable, "no audio buffered yet")
		return
	}

	filename, err := s.clips.Encode(pcm)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not encode clip")
		return
	}

	// Actual duration from the amount of PCM captured.
	frames := len(pcm) / audio.FrameBytes
	duration := float64(frames) * float64(audio.FrameMs) / 1000.0

	title := req.Title
	if title == "" {
		title = "Clip " + time.Now().Format("15:04")
	}

	clip := models.Clip{
		Title:       title,
		Filename:    filename,
		DurationSec: duration,
	}
	if err := s.db.Create(&clip).Error; err != nil {
		_ = s.clips.Delete(filename)
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, toClipJSON(clip))
}

func (s *Server) handleListClips(w http.ResponseWriter, r *http.Request) {
	var clips []models.Clip
	if err := s.db.Order("created_at desc").Find(&clips).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	out := make([]clipJSON, 0, len(clips))
	for _, c := range clips {
		out = append(out, toClipJSON(c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteClip(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var clip models.Clip
	if err := s.db.First(&clip, "id = ?", id).Error; err != nil {
		if db.IsNotFound(err) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if err := s.db.Delete(&clip).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	_ = s.clips.Delete(clip.Filename)
	w.WriteHeader(http.StatusNoContent)
}

// handleClipAudio serves the clip's MP3 (public, supports range/seek).
func (s *Server) handleClipAudio(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var clip models.Clip
	if err := s.db.First(&clip, "id = ?", id).Error; err != nil {
		if db.IsNotFound(err) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	http.ServeFile(w, r, s.clips.Path(clip.Filename))
}
