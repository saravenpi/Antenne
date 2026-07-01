package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration for the Antenne server, loaded from
// environment variables (see .env.example).
type Config struct {
	Port         string
	ClientOrigin string
	DatabaseURL  string

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
		Port:          env("PORT", "4000"),
		ClientOrigin:  env("CLIENT_ORIGIN", "http://localhost:5173"),
		DatabaseURL:   env("DATABASE_URL", "postgres://antenne:antenne@127.0.0.1:5433/antenne?sslmode=disable"),
		JWTSecret:     env("JWT_SECRET", "dev-secret-change-me"),
		AdminUsername: env("ADMIN_USERNAME", "admin"),
		AdminPassword: env("ADMIN_PASSWORD", "admin"),
		StorageDir:    env("STORAGE_DIR", "./data/tracks"),
		StreamDir:     env("STREAM_DIR", "./data/stream"),
		ClipsDir:      env("CLIPS_DIR", "./data/clips"),
		FFmpegBin:     env("FFMPEG_BIN", "ffmpeg"),
		CrossfadeMs:   envInt("CROSSFADE_MS", 2000),
		HLSSegmentS:   envInt("HLS_SEGMENT_SEC", 3),
		HLSListSize:   envInt("HLS_LIST_SIZE", 6),
		MP3Enabled:    envBool("MP3_STREAM_ENABLED", true),
		MP3BitrateK:   envInt("MP3_BITRATE_K", 128),
		ICYMetaInt:    envInt("ICY_METAINT", 16000),
		StationName:   env("STATION_NAME", "Antenne"),
		StationGenre:  env("STATION_GENRE", "Various"),
		StationURL:    env("STATION_URL", ""),
		StationDesc:   env("STATION_DESCRIPTION", ""),
		StationPublic: envBool("STATION_PUBLIC", false),
	}
	return c
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
