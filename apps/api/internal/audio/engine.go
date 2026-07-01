package audio

import (
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// NowPlaying is the public snapshot of what the station is broadcasting.
type NowPlaying struct {
	Live      bool   `json:"live"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	TrackID   string `json:"trackId"`
	Listeners int64  `json:"listeners"`
}

// Engine is the real-time heart of the station. A single goroutine ticks every
// 20 ms, pulls a frame from the live source (when the admin holds the mic) or
// the playlist otherwise, applies a short fade on transitions, and writes the
// mixed PCM to the HLS encoder.
type Engine struct {
	Playlist *Playlist
	Live     *LiveSource
	encoder  *HLSEncoder

	fadeFrames int     // frames over which to fade between sources
	gain       float64 // current playlist gain (1 = full, 0 = fully ducked)

	listeners int64
	stop      chan struct{}
	once      sync.Once
}

// NewEngine wires the sources and encoder together.
func NewEngine(pl *Playlist, live *LiveSource, enc *HLSEncoder, crossfadeMs int) *Engine {
	fade := crossfadeMs / FrameMs
	if fade < 1 {
		fade = 1
	}
	return &Engine{
		Playlist:   pl,
		Live:       live,
		encoder:    enc,
		fadeFrames: fade,
		gain:       1.0,
		stop:       make(chan struct{}),
	}
}

// Start launches the encoder and the real-time mixing loop.
func (e *Engine) Start() error {
	if err := e.encoder.Start(); err != nil {
		return err
	}
	go e.loop()
	log.Print("engine: broadcasting")
	return nil
}

// Stop halts the loop and the encoder.
func (e *Engine) Stop() {
	e.once.Do(func() { close(e.stop) })
	e.encoder.Stop()
}

func (e *Engine) loop() {
	ticker := time.NewTicker(FrameMs * time.Millisecond)
	defer ticker.Stop()

	step := 1.0 / float64(e.fadeFrames)
	for {
		select {
		case <-e.stop:
			return
		case <-ticker.C:
			live := e.Live.Active()

			// Ramp the playlist gain down when live takes over, up when it ends.
			if live && e.gain > 0 {
				e.gain -= step
			} else if !live && e.gain < 1 {
				e.gain += step
			}
			e.gain = clamp(e.gain)

			var frame []byte
			switch {
			case live && e.gain <= 0:
				frame = e.Live.ReadFrame()
			case live:
				// Transition: duck the playlist under the live feed.
				frame = mix(e.Live.ReadFrame(), scaleFrame(e.Playlist.ReadFrame(), e.gain))
			default:
				frame = scaleFrame(e.Playlist.ReadFrame(), e.gain)
			}

			if err := e.encoder.Write(frame); err != nil {
				log.Printf("engine: encoder write failed: %v", err)
				return
			}
		}
	}
}

// NowPlaying returns the current broadcast state.
func (e *Engine) NowPlaying() NowPlaying {
	np := NowPlaying{
		Live:      e.Live.Active(),
		Listeners: atomic.LoadInt64(&e.listeners),
	}
	if item, ok := e.Playlist.Current(); ok {
		np.Title = item.Title
		np.Artist = item.Artist
		np.TrackID = item.ID
	}
	return np
}

// AddListener / RemoveListener track the live audience count (called when HLS
// manifests are requested / a session times out).
func (e *Engine) AddListener()    { atomic.AddInt64(&e.listeners, 1) }
func (e *Engine) RemoveListener() { atomic.AddInt64(&e.listeners, -1) }

func clamp(g float64) float64 {
	if g < 0 {
		return 0
	}
	if g > 1 {
		return 1
	}
	return g
}

// mix sums two equal-length PCM frames with saturation.
func mix(a, b []byte) []byte {
	out := make([]byte, len(a))
	for i := 0; i+1 < len(a); i += 2 {
		sa := int32(int16(uint16(a[i]) | uint16(a[i+1])<<8))
		sb := int32(int16(uint16(b[i]) | uint16(b[i+1])<<8))
		s := sa + sb
		if s > 32767 {
			s = 32767
		} else if s < -32768 {
			s = -32768
		}
		out[i] = byte(int16(s))
		out[i+1] = byte(int16(s) >> 8)
	}
	return out
}
