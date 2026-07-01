package main

import (
	"log"
	"net/http"

	"github.com/saravenpi/antenne/internal/audio"
	"github.com/saravenpi/antenne/internal/auth"
	"github.com/saravenpi/antenne/internal/config"
	"github.com/saravenpi/antenne/internal/db"
	"github.com/saravenpi/antenne/internal/httpapi"
	"github.com/saravenpi/antenne/internal/store"
)

func main() {
	cfg := config.Load()

	database, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	st, err := store.New(cfg.StorageDir, cfg.FFmpegBin)
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	// Audio engine: playlist + live source -> mixer -> HLS encoder.
	playlist := audio.NewPlaylist(cfg.FFmpegBin)
	live := audio.NewLiveSource(cfg.FFmpegBin)
	encoder := audio.NewHLSEncoder(cfg.FFmpegBin, cfg.StreamDir, cfg.HLSSegmentS, cfg.HLSListSize)
	engine := audio.NewEngine(playlist, live, encoder, cfg.CrossfadeMs)

	authSvc := auth.New(cfg.JWTSecret)
	srv := httpapi.NewServer(cfg, database, authSvc, st, engine)

	if err := srv.SyncPlaylist(); err != nil {
		log.Fatalf("playlist sync: %v", err)
	}
	if err := engine.Start(); err != nil {
		log.Fatalf("engine: %v", err)
	}
	defer engine.Stop()

	addr := ":" + cfg.Port
	log.Printf("Antenne on the air — http://localhost%s", addr)
	if err := http.ListenAndServe(addr, srv.Router()); err != nil {
		log.Fatalf("http: %v", err)
	}
}
