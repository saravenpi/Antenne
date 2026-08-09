package httpapi

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/saravenpi/poste/internal/db"
	"github.com/saravenpi/poste/internal/models"
)

// audioExts is the allowlist of extensions accepted for track uploads. It keeps
// arbitrary file types out of the media store; ffmpeg still validates the
// contents downstream.
var audioExts = map[string]bool{
	".mp3": true, ".flac": true, ".wav": true, ".ogg": true, ".oga": true,
	".opus": true, ".m4a": true, ".mp4": true, ".aac": true, ".wma": true,
	".aiff": true, ".aif": true, ".alac": true, ".webm": true, ".mkv": true,
}

func allowedAudioExt(name string) bool {
	return audioExts[strings.ToLower(filepath.Ext(name))]
}

// looksLikeImage sniffs the first bytes of an upload and reports whether they
// are an image, rewinding the reader so the caller can still store the file.
func looksLikeImage(f io.ReadSeeker) bool {
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return false
	}
	return strings.HasPrefix(http.DetectContentType(buf[:n]), "image/")
}

// trackCoverURL builds the public cover URL for a track, or "" when it has no
// art. It appends a short version token derived from the cover filename so that
// replacing a cover (which writes a fresh filename) busts the browser/CDN cache
// even though the path is stable.
func trackCoverURL(id uuid.UUID, cover string) string {
	if cover == "" {
		return ""
	}
	v := strings.TrimSuffix(cover, filepath.Ext(cover))
	if len(v) > 8 {
		v = v[len(v)-8:]
	}
	return "/api/tracks/" + id.String() + "/cover?v=" + v
}

// decorateTracks fills the transient CoverURL on tracks that have art.
func decorateTracks(tracks []models.Track) {
	for i := range tracks {
		tracks[i].CoverURL = trackCoverURL(tracks[i].ID, tracks[i].Cover)
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

	if !allowedAudioExt(header.Filename) {
		writeErr(w, http.StatusBadRequest, "unsupported audio format")
		return
	}

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
	track.CoverURL = trackCoverURL(track.ID, track.Cover)
	writeJSON(w, http.StatusCreated, track)
}

// updateTrackReq patches a track's editable metadata. Pointers distinguish
// "not provided" from "set to empty".
type updateTrackReq struct {
	Title  *string `json:"title"`
	Artist *string `json:"artist"`
}

// handleUpdateTrack edits a track's title/artist after upload (e.g. for files
// with missing or wrong tags).
func (s *Server) handleUpdateTrack(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req updateTrackReq
	if err := decode(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
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
	updates := map[string]any{}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			writeErr(w, http.StatusBadRequest, "title required")
			return
		}
		updates["title"] = title
		track.Title = title
	}
	if req.Artist != nil {
		artist := strings.TrimSpace(*req.Artist)
		updates["artist"] = artist
		track.Artist = artist
	}
	if len(updates) > 0 {
		if err := s.db.Model(&models.Track{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			writeErr(w, http.StatusInternalServerError, "db error")
			return
		}
		_ = s.SyncPlaylist()
	}
	track.CoverURL = trackCoverURL(track.ID, track.Cover)
	writeJSON(w, http.StatusOK, track)
}

// handleUploadCover replaces a track's cover art with an uploaded image, so
// tracks without embedded art (or with the wrong art) can get a proper cover.
func (s *Server) handleUploadCover(w http.ResponseWriter, r *http.Request) {
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
	if err := r.ParseMultipartForm(16 << 20); err != nil { // covers are small
		writeErr(w, http.StatusBadRequest, "invalid upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()

	if !looksLikeImage(file) {
		writeErr(w, http.StatusBadRequest, "cover must be an image")
		return
	}

	name := header.Filename
	if filepath.Ext(name) == "" {
		name = "cover.jpg"
	}
	cover, err := s.store.Save(file, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not store cover")
		return
	}
	old := track.Cover
	if err := s.db.Model(&models.Track{}).Where("id = ?", id).Update("cover", cover).Error; err != nil {
		_ = s.store.Delete(cover)
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if old != "" && old != cover {
		_ = s.store.Delete(old)
	}
	track.Cover = cover
	track.CoverURL = trackCoverURL(track.ID, track.Cover)
	writeJSON(w, http.StatusOK, track)
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
	if err := decode(w, r, &req); err != nil {
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
