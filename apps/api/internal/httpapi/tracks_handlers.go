package httpapi

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/saravenpi/antenne/internal/db"
	"github.com/saravenpi/antenne/internal/models"
)

// decorateTracks fills the transient CoverURL on tracks that have art.
func decorateTracks(tracks []models.Track) {
	for i := range tracks {
		if tracks[i].Cover != "" {
			tracks[i].CoverURL = "/api/tracks/" + tracks[i].ID.String() + "/cover"
		}
	}
}

// --- Tracks ---

func (s *Server) handleListTracks(w http.ResponseWriter, r *http.Request) {
	var tracks []models.Track
	if err := s.db.Order("position asc").Find(&tracks).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	sortTracks(tracks)
	decorateTracks(tracks)
	writeJSON(w, http.StatusOK, tracks)
}

// handleTrackCover serves a track's extracted album art (public — the listener
// page and régie both display it).
func (s *Server) handleTrackCover(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var track models.Track
	if err := s.db.First(&track, "id = ?", id).Error; err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if track.Cover == "" {
		writeErr(w, http.StatusNotFound, "no cover")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, s.store.Path(track.Cover))
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
	// Optionally file the upload straight into a collection.
	if cidStr := r.FormValue("collectionId"); cidStr != "" {
		if cid, err := uuid.Parse(cidStr); err == nil {
			track.CollectionID = &cid
		}
	}
	if cover, ok := s.store.ExtractCover(filename); ok {
		track.Cover = cover
	}
	if err := s.db.Create(&track).Error; err != nil {
		_ = s.store.Delete(filename)
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	_ = s.SyncPlaylist()
	if track.Cover != "" {
		track.CoverURL = "/api/tracks/" + track.ID.String() + "/cover"
	}
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
	if track.Cover != "" {
		_ = s.store.Delete(track.Cover)
	}
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
		// Look for embedded album art when the track doesn't already have one.
		if tracks[i].Cover == "" {
			if cover, ok := s.store.ExtractCover(tracks[i].Filename); ok {
				updates["cover"] = cover
				tracks[i].Cover = cover
			}
		}
		if len(updates) > 0 {
			s.db.Model(&models.Track{}).Where("id = ?", tracks[i].ID).Updates(updates)
		}
	}
	_ = s.SyncPlaylist()
	sortTracks(tracks)
	decorateTracks(tracks)
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
