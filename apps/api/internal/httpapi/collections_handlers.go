package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/saravenpi/antenne/internal/models"
)

// --- Collections (named playlists / folders) ---

type collectionReq struct {
	Name string `json:"name"`
}

func (s *Server) handleListCollections(w http.ResponseWriter, r *http.Request) {
	var cols []models.Collection
	if err := s.db.Order("position asc").Find(&cols).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, cols)
}

func (s *Server) handleCreateCollection(w http.ResponseWriter, r *http.Request) {
	var req collectionReq
	if err := decode(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	var maxPos int
	s.db.Model(&models.Collection{}).Select("COALESCE(MAX(position), -1)").Scan(&maxPos)
	col := models.Collection{Name: name, Position: maxPos + 1}
	if err := s.db.Create(&col).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, col)
}

func (s *Server) handleRenameCollection(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req collectionReq
	if err := decode(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if err := s.db.Model(&models.Collection{}).Where("id = ?", id).Update("name", name).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteCollection(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	// Ungroup its tracks (keep them in the library), then delete the collection.
	s.db.Model(&models.Track{}).Where("collection_id = ?", id).Update("collection_id", nil)
	if err := s.db.Delete(&models.Collection{}, "id = ?", id).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	_ = s.SyncPlaylist()
	w.WriteHeader(http.StatusNoContent)
}

// handleActivateCollection puts a collection on air (only its tracks feed the
// engine). Deactivates any other active collection first.
func (s *Server) handleActivateCollection(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	s.db.Model(&models.Collection{}).Where("active = ?", true).Update("active", false)
	if err := s.db.Model(&models.Collection{}).Where("id = ?", id).Update("active", true).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	_ = s.SyncPlaylist()
	writeJSON(w, http.StatusOK, s.currentNowPlaying())
}

// handleClearActiveCollection returns to playing the whole library.
func (s *Server) handleClearActiveCollection(w http.ResponseWriter, r *http.Request) {
	s.db.Model(&models.Collection{}).Where("active = ?", true).Update("active", false)
	_ = s.SyncPlaylist()
	writeJSON(w, http.StatusOK, s.currentNowPlaying())
}

type moveTrackReq struct {
	CollectionID *string `json:"collectionId"`
}

// handleMoveTrack assigns a track to a collection, or ungroups it when
// collectionId is null/empty.
func (s *Server) handleMoveTrack(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req moveTrackReq
	if err := decode(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	var colID *uuid.UUID
	if req.CollectionID != nil && *req.CollectionID != "" {
		cid, err := uuid.Parse(*req.CollectionID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid collection id")
			return
		}
		colID = &cid
	}
	if err := s.db.Model(&models.Track{}).Where("id = ?", id).
		Updates(map[string]any{"collection_id": colID}).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	_ = s.SyncPlaylist()
	w.WriteHeader(http.StatusNoContent)
}
