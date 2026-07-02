package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/saravenpi/antenne/internal/audio"
	"github.com/saravenpi/antenne/internal/auth"
	"github.com/saravenpi/antenne/internal/clips"
	"github.com/saravenpi/antenne/internal/config"
	"github.com/saravenpi/antenne/internal/db"
	"github.com/saravenpi/antenne/internal/httpapi"
	"github.com/saravenpi/antenne/internal/store"
)

func main() {
	cfg := config.Load()

	warnings, err := cfg.Validate()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	for _, w := range warnings {
		log.Printf("⚠ config: %s", w)
	}

	database, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	st, err := store.New(cfg.StorageDir, cfg.FFmpegBin)
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	clipsSvc, err := clips.New(cfg.ClipsDir, cfg.FFmpegBin)
	if err != nil {
		log.Fatalf("clips: %v", err)
	}

	// Audio engine: playlist + live source -> mixer -> HLS (browser) + MP3 (external players).
	playlist := audio.NewPlaylist(cfg.FFmpegBin)
	live := audio.NewLiveSource(cfg.FFmpegBin)
	hls := audio.NewHLSEncoder(cfg.FFmpegBin, cfg.StreamDir, cfg.HLSSegmentS, cfg.HLSListSize)
	var mp3 *audio.MP3Encoder
	if cfg.MP3Enabled {
		mp3 = audio.NewMP3Encoder(cfg.FFmpegBin, cfg.MP3BitrateK)
	}
	engine := audio.NewEngine(playlist, live, hls, mp3, cfg.CrossfadeMs)

	authSvc := auth.New(cfg.JWTSecret)
	srv := httpapi.NewServer(cfg, database, authSvc, st, clipsSvc, engine)

	if err := srv.SyncPlaylist(); err != nil {
		log.Fatalf("playlist sync: %v", err)
	}
	if err := engine.Start(); err != nil {
		log.Fatalf("engine: %v", err)
	}

	addr := ":" + cfg.Port
	httpSrv := &http.Server{
		Addr:    addr,
		Handler: srv.Router(),
		// Bound how long a client may take to send request headers — this is the
		// key defence against Slowloris. ReadTimeout/WriteTimeout are intentionally
		// left unset: uploads (up to 512 MB) and the long-lived HLS/MP3 streams
		// would otherwise be cut off mid-transfer.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown: on SIGINT/SIGTERM, stop accepting connections then tear
	// the engine down so ffmpeg children are killed and reaped (a bare
	// log.Fatalf/os.Exit would skip that and orphan them).
	stopped := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Print("shutting down…")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
		engine.Stop()
		log.Print("shutdown complete")
		close(stopped)
	}()

	log.Printf("Antenne on the air — http://localhost%s", addr)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		engine.Stop()
		log.Fatalf("http: %v", err)
	}
	// ListenAndServe returned ErrServerClosed (Shutdown was called): wait for the
	// teardown to finish before exiting, so ffmpeg is reaped, not orphaned.
	<-stopped
}
