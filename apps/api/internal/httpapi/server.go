package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"sort"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/saravenpi/antenne/internal/audio"
	"github.com/saravenpi/antenne/internal/auth"
	"github.com/saravenpi/antenne/internal/config"
	"github.com/saravenpi/antenne/internal/models"
	"github.com/saravenpi/antenne/internal/store"
	"gorm.io/gorm"
)

// Server holds the HTTP dependencies for the control plane and stream endpoints.
type Server struct {
	cfg    config.Config
	db     *gorm.DB
	auth   *auth.Service
	store  *store.Store
	engine *audio.Engine
}

func NewServer(cfg config.Config, db *gorm.DB, authSvc *auth.Service, st *store.Store, engine *audio.Engine) *Server {
	return &Server{cfg: cfg, db: db, auth: authSvc, store: st, engine: engine}
}

// Router builds the full chi router: public stream + player data, and the
// admin-only control plane.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{s.cfg.ClientOrigin},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	// Public
	r.Post("/api/auth/login", s.handleLogin)
	r.Get("/api/now-playing", s.handleNowPlaying)
	r.Handle("/stream/*", s.streamHandler())

	// Admin-only control plane
	r.Group(func(r chi.Router) {
		r.Use(s.auth.Middleware)
		r.Get("/api/tracks", s.handleListTracks)
		r.Post("/api/tracks", s.handleUploadTrack)
		r.Delete("/api/tracks/{id}", s.handleDeleteTrack)
		r.Put("/api/playlist", s.handleReorderPlaylist)
		r.Post("/api/live/stop", s.handleLiveStop)
	})

	// WebSocket mic ingest (authorised inside the handler).
	r.Get("/api/live/ingest", s.handleLiveIngest)

	return r
}

// SyncPlaylist loads tracks from the DB (ordered by position) into the engine.
func (s *Server) SyncPlaylist() error {
	var tracks []models.Track
	if err := s.db.Order("position asc").Find(&tracks).Error; err != nil {
		return err
	}
	items := make([]audio.Item, 0, len(tracks))
	for _, t := range tracks {
		items = append(items, audio.Item{
			ID:     t.ID.String(),
			Title:  t.Title,
			Artist: t.Artist,
			Path:   s.store.Path(t.Filename),
		})
	}
	s.engine.Playlist.SetItems(items)
	return nil
}

// streamHandler serves the generated HLS directory (manifest + segments).
func (s *Server) streamHandler() http.Handler {
	fs := http.FileServer(http.Dir(s.cfg.StreamDir))
	return http.StripPrefix("/stream/", noCache(fs))
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if filepath.Ext(r.URL.Path) == ".m3u8" {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// --- small helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// sortItemsByPosition is used when returning the playlist to the admin UI.
func sortTracks(tracks []models.Track) {
	sort.SliceStable(tracks, func(i, j int) bool {
		return tracks[i].Position < tracks[j].Position
	})
}
