package httpapi

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/gorilla/websocket"
	"github.com/saravenpi/poste/internal/audio"
	"github.com/saravenpi/poste/internal/auth"
	"github.com/saravenpi/poste/internal/clips"
	"github.com/saravenpi/poste/internal/config"
	"github.com/saravenpi/poste/internal/models"
	"github.com/saravenpi/poste/internal/store"
	"gorm.io/gorm"
)

// Server holds the HTTP dependencies for the control plane and stream endpoints.
type Server struct {
	cfg       config.Config
	db        *gorm.DB
	auth      *auth.Service
	store     *store.Store
	clips     *clips.Service
	engine    *audio.Engine
	chat      *chatHub
	listeners *listenerTracker

	trustedProxies []*net.IPNet
	upgrader       websocket.Upgrader
	loginLimiter   *loginLimiter
}

func NewServer(cfg config.Config, db *gorm.DB, authSvc *auth.Service, st *store.Store, clipsSvc *clips.Service, engine *audio.Engine) *Server {
	s := &Server{cfg: cfg, db: db, auth: authSvc, store: st, clips: clipsSvc, engine: engine}
	s.chat = newChatHub()
	s.listeners = newListenerTracker()
	s.trustedProxies = parseTrustedProxies(cfg.TrustedProxies)
	s.upgrader = s.newUpgrader()
	// Allow 10 failed login attempts per IP per minute before locking that IP out.
	s.loginLimiter = newLoginLimiter(10, time.Minute)
	go s.chat.run()
	return s
}

// Router builds the full chi router: public stream + player data, and the
// admin-only control plane.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(requestLogger) // redacts ?token= before logging (see sanitizeURI)
	r.Use(middleware.Recoverer)
	r.Use(secureHeaders)

	// CORS: reflect a concrete origin with credentials, but never combine a
	// wildcard with credentials (which is both invalid and unsafe). Auth uses a
	// Bearer header, not cookies, so wildcard-without-credentials is sufficient
	// for the public endpoints.
	corsOpts := cors.Options{
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Authorization", "Content-Type"},
	}
	if s.cfg.IsWildcardOrigin() {
		corsOpts.AllowedOrigins = []string{"*"}
		corsOpts.AllowCredentials = false
	} else {
		corsOpts.AllowedOrigins = []string{s.cfg.ClientOrigin}
		corsOpts.AllowCredentials = true
	}
	r.Use(cors.Handler(corsOpts))

	// Public
	r.With(s.rateLimitLogin).Post("/api/auth/login", s.handleLogin)
	r.Get("/api/now-playing", s.handleNowPlaying)
	r.Get("/api/appearance", s.handleAppearance)
	r.Get("/api/tracks/{id}/cover", s.handleTrackCover)
	r.Get("/api/clips/{id}/audio", s.handleClipAudio)
	r.Get("/api/chat/messages", s.handleChatMessages)
	r.Handle("/stream/*", s.streamHandler())
	r.Get("/stream.mp3", s.handleMP3Stream) // continuous MP3 for external players
	r.Method("HEAD", "/stream.mp3", http.HandlerFunc(s.handleMP3StreamHead))
	r.Get("/stream.pls", s.handleStreamPLS)
	r.Get("/stream.m3u", s.handleStreamM3U)

	// Admin-only control plane
	r.Group(func(r chi.Router) {
		r.Use(s.auth.Middleware)
		r.Get("/api/tracks", s.handleListTracks)
		r.Post("/api/tracks", s.handleUploadTrack)
		r.Post("/api/tracks/rescan", s.handleRescanMetadata)
		r.Patch("/api/tracks/{id}", s.handleUpdateTrack)
		r.Post("/api/tracks/{id}/cover", s.handleUploadCover)
		r.Delete("/api/tracks/{id}", s.handleDeleteTrack)
		r.Put("/api/playlist", s.handleReorderPlaylist)
		r.Post("/api/live/stop", s.handleLiveStop)

		r.Post("/api/playback/next", s.handlePlaybackNext)
		r.Post("/api/playback/previous", s.handlePlaybackPrev)
		r.Post("/api/playback/pause", s.handlePlaybackPause)
		r.Post("/api/playback/resume", s.handlePlaybackResume)
		r.Post("/api/tracks/{id}/play", s.handlePlayTrack)
		r.Put("/api/tracks/{id}/collection", s.handleMoveTrack)

		r.Get("/api/collections", s.handleListCollections)
		r.Post("/api/collections", s.handleCreateCollection)
		r.Post("/api/collections/clear", s.handleClearActiveCollection)
		r.Put("/api/collections/{id}", s.handleRenameCollection)
		r.Delete("/api/collections/{id}", s.handleDeleteCollection)
		r.Post("/api/collections/{id}/activate", s.handleActivateCollection)

		r.Post("/api/clips", s.handleCreateClip)
		r.Get("/api/clips", s.handleListClips)
		r.Delete("/api/clips/{id}", s.handleDeleteClip)

		r.Delete("/api/chat/messages/{id}", s.handleDeleteChatMessage)
		r.Get("/api/chat/bans", s.handleListBans)
		r.Post("/api/chat/bans", s.handleCreateBan)
		r.Delete("/api/chat/bans/{id}", s.handleDeleteBan)
		r.Get("/api/chat/restrictions", s.handleListRestrictions)
		r.Post("/api/chat/restrictions", s.handleCreateRestriction)
		r.Delete("/api/chat/restrictions/{id}", s.handleDeleteRestriction)

		r.Get("/api/settings", s.handleGetSettings)
		r.Put("/api/settings", s.handleUpdateSettings)
	})

	// WebSocket mic ingest (authorised inside the handler).
	r.Get("/api/live/ingest", s.handleLiveIngest)
	// WebSocket chat (public; admin if a valid token is present).
	r.Get("/api/chat/ws", s.handleChatWS)

	return r
}

// SyncPlaylist loads tracks into the engine (ordered by position). When a
// collection is active, only its tracks play; if that collection is empty (or
// none is active) the whole library plays.
func (s *Server) SyncPlaylist() error {
	var tracks []models.Track
	var active models.Collection
	hasActive := s.db.Where("active = ?", true).First(&active).Error == nil

	q := s.db.Order("position asc")
	if hasActive {
		q = q.Where("collection_id = ?", active.ID)
	}
	if err := q.Find(&tracks).Error; err != nil {
		return err
	}
	if hasActive && len(tracks) == 0 {
		if err := s.db.Order("position asc").Find(&tracks).Error; err != nil {
			return err
		}
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

// streamHandler serves the generated HLS directory (manifest + segments) and
// counts distinct listeners by IP on each manifest request.
func (s *Server) streamHandler() http.Handler {
	fs := http.FileServer(http.Dir(s.cfg.StreamDir))
	stripped := http.StripPrefix("/stream/", noCache(fs))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".m3u8") {
			s.listeners.hit(s.clientIP(r))
		}
		stripped.ServeHTTP(w, r)
	})
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

// maxJSONBody caps the size of a JSON request body. It is generous enough for
// the largest legitimate payload (appearance settings carry a downscaled 1600px
// JPEG background as a data URL) while preventing an unbounded body from
// exhausting memory.
const maxJSONBody = 4 << 20 // 4 MB

func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	return json.NewDecoder(r.Body).Decode(v)
}

// secureHeaders sets conservative security response headers. These do not
// restrict resource loading (the client's Content-Security-Policy handles that),
// so they are safe on every API and stream response.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// clientIP resolves the real client IP. Forwarded headers (CF-Connecting-IP,
// X-Forwarded-For) are only honoured when the direct peer (RemoteAddr) is a
// trusted proxy; otherwise a client could spoof them to evade IP bans,
// slow-mode, and login rate limiting. When the peer is not trusted we use the
// raw connection address.
func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !s.trustsProxy(host) {
		return host
	}
	// Behind a trusted proxy: Cloudflare overwrites CF-Connecting-IP with the
	// real client, so prefer it; else fall back to the first X-Forwarded-For hop.
	if cf := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cf != "" {
		return cf
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	return host
}

// trustsProxy reports whether the given peer address belongs to a trusted proxy
// network, meaning its forwarding headers may be believed.
func (s *Server) trustsProxy(host string) bool {
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	for _, n := range s.trustedProxies {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// defaultTrustedProxyCIDRs are trusted when TRUSTED_PROXIES is unset: loopback
// plus RFC1918/ULA private ranges. A reverse proxy (Traefik) on the same private
// Docker network lands here, while direct hits from the public internet do not.
var defaultTrustedProxyCIDRs = []string{
	"127.0.0.0/8", "::1/128",
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
	"fc00::/7",
}

// parseTrustedProxies turns a comma/space-separated list of CIDRs or bare IPs
// into networks. Empty input yields the private-range defaults. Invalid entries
// are logged and skipped.
func parseTrustedProxies(raw string) []*net.IPNet {
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	if len(fields) == 0 {
		fields = defaultTrustedProxyCIDRs
	}
	var nets []*net.IPNet
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if !strings.Contains(f, "/") {
			if strings.Contains(f, ":") {
				f += "/128"
			} else {
				f += "/32"
			}
		}
		_, n, err := net.ParseCIDR(f)
		if err != nil {
			log.Printf("config: ignoring invalid TRUSTED_PROXIES entry %q: %v", f, err)
			continue
		}
		nets = append(nets, n)
	}
	return nets
}

// newUpgrader builds a WebSocket upgrader that enforces same-origin (or the
// configured CLIENT_ORIGIN), guarding against Cross-Site WebSocket Hijacking.
// Requests without an Origin header (native players, curl) are allowed since a
// victim's browser always sends one.
func (s *Server) newUpgrader() websocket.Upgrader {
	wildcard := s.cfg.IsWildcardOrigin()
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			if wildcard {
				return true
			}
			u, err := url.Parse(origin)
			if err != nil {
				return false
			}
			if strings.EqualFold(u.Host, r.Host) {
				return true // same-origin
			}
			return strings.EqualFold(origin, s.cfg.ClientOrigin)
		},
	}
}

// requestLogger mirrors chi's default request logger but redacts the `token`
// query parameter, which the WebSocket endpoints accept — otherwise admin JWTs
// would land in access logs.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		defer func() {
			log.Printf("%s %s %d %dB %s",
				r.Method, sanitizeURI(r.RequestURI), ww.Status(), ww.BytesWritten(), time.Since(start))
		}()
		next.ServeHTTP(ww, r)
	})
}

// sanitizeURI redacts the value of a `token` query parameter so JWTs never reach
// the logs.
func sanitizeURI(uri string) string {
	u, err := url.ParseRequestURI(uri)
	if err != nil {
		return uri
	}
	q := u.Query()
	if q.Has("token") {
		q.Set("token", "REDACTED")
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// sortItemsByPosition is used when returning the playlist to the admin UI.
func sortTracks(tracks []models.Track) {
	sort.SliceStable(tracks, func(i, j int) bool {
		return tracks[i].Position < tracks[j].Position
	})
}
