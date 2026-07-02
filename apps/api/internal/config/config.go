package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration for the Antenne server, loaded from
// environment variables (see .env.example).
type Config struct {
	Env          string // "development" (default) or "production"
	Port         string
	ClientOrigin string
	DatabaseURL  string

	// TrustedProxies is a comma/space-separated list of CIDRs or IPs whose
	// X-Forwarded-For / CF-Connecting-IP headers are honoured. Empty means the
	// built-in default (loopback + RFC1918 private ranges).
	TrustedProxies string

	JWTSecret     string
	AdminUsername string
	AdminPassword string

	StorageDir string
	StreamDir  string
	ClipsDir   string

	FFmpegBin   string
	CrossfadeMs int
	HLSSegmentS int
	HLSListSize int

	// Continuous MP3 broadcast (ICY/Icecast-style) for external players.
	MP3Enabled    bool
	MP3BitrateK   int
	ICYMetaInt    int
	StationName   string
	StationGenre  string
	StationURL    string
	StationDesc   string
	StationPublic bool
}

// Load reads configuration from the environment, applying an optional .env file
// and sane defaults for local development.
func Load() Config {
	_ = godotenv.Load()

	c := Config{
		Env:            env("ANTENNE_ENV", "development"),
		Port:           env("PORT", "4000"),
		ClientOrigin:   env("CLIENT_ORIGIN", "http://localhost:5173"),
		TrustedProxies: env("TRUSTED_PROXIES", ""),
		DatabaseURL:    env("DATABASE_URL", "postgres://antenne:antenne@127.0.0.1:5433/antenne?sslmode=disable"),
		JWTSecret:      env("JWT_SECRET", "dev-secret-change-me"),
		AdminUsername:  env("ADMIN_USERNAME", "admin"),
		AdminPassword:  env("ADMIN_PASSWORD", "admin"),
		StorageDir:     env("STORAGE_DIR", "./data/tracks"),
		StreamDir:      env("STREAM_DIR", "./data/stream"),
		ClipsDir:       env("CLIPS_DIR", "./data/clips"),
		FFmpegBin:      env("FFMPEG_BIN", "ffmpeg"),
		CrossfadeMs:    envInt("CROSSFADE_MS", 2000),
		HLSSegmentS:    envInt("HLS_SEGMENT_SEC", 3),
		HLSListSize:    envInt("HLS_LIST_SIZE", 6),
		MP3Enabled:     envBool("MP3_STREAM_ENABLED", true),
		MP3BitrateK:    envInt("MP3_BITRATE_K", 128),
		ICYMetaInt:     envInt("ICY_METAINT", 16000),
		StationName:    env("STATION_NAME", "Antenne"),
		StationGenre:   env("STATION_GENRE", "Various"),
		StationURL:     env("STATION_URL", ""),
		StationDesc:    env("STATION_DESCRIPTION", ""),
		StationPublic:  envBool("STATION_PUBLIC", false),
	}
	return c
}

// IsProd reports whether the server is running in production mode, where
// insecure defaults are rejected rather than merely warned about.
func (c Config) IsProd() bool {
	return strings.EqualFold(c.Env, "production") || strings.EqualFold(c.Env, "prod")
}

// IsWildcardOrigin reports whether CORS is configured to allow any origin. In
// that case credentials must not be reflected (see the CORS setup in httpapi).
func (c Config) IsWildcardOrigin() bool {
	return c.ClientOrigin == "" || c.ClientOrigin == "*"
}

// insecureSecrets are placeholder values shipped in examples/compose that must
// never reach production.
var insecureSecrets = map[string]bool{
	"":                        true,
	"change-me":               true,
	"change-me-in-production": true,
	"dev-secret-change-me":    true,
	"secret":                  true,
	"admin":                   true,
	"password":                true,
}

// Validate enforces safe configuration. In production it returns a fatal error
// when secrets, the admin password, or the CORS origin are unset/default/weak.
// In development the same problems come back as non-fatal warnings so local dev
// stays frictionless.
func (c Config) Validate() (warnings []string, err error) {
	secretWeak := insecureSecrets[c.JWTSecret] || len(c.JWTSecret) < 16
	passWeak := insecureSecrets[c.AdminPassword]
	originWild := c.IsWildcardOrigin()

	if c.IsProd() {
		var problems []string
		if secretWeak {
			problems = append(problems, "JWT_SECRET is unset, a known default, or shorter than 16 chars — set a strong random value")
		}
		if passWeak {
			problems = append(problems, "ADMIN_PASSWORD is unset or a known default — set a strong password")
		}
		if originWild {
			problems = append(problems, "CLIENT_ORIGIN must be an explicit origin (not empty or \"*\") in production")
		}
		if len(problems) > 0 {
			return nil, fmt.Errorf("insecure production configuration:\n  - %s", strings.Join(problems, "\n  - "))
		}
		return nil, nil
	}

	if secretWeak {
		warnings = append(warnings, "JWT_SECRET is weak/default — acceptable for dev, but MUST be set to a strong random value in production")
	}
	if passWeak {
		warnings = append(warnings, "ADMIN_PASSWORD is weak/default — acceptable for dev, but MUST be changed in production")
	}
	if originWild {
		warnings = append(warnings, "CLIENT_ORIGIN is empty or \"*\" — CORS credentials are disabled and any origin is allowed")
	}
	return warnings, nil
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			log.Printf("config: invalid bool for %s=%q, using %v", key, v, fallback)
			return fallback
		}
		return b
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			log.Printf("config: invalid int for %s=%q, using %d", key, v, fallback)
			return fallback
		}
		return n
	}
	return fallback
}
