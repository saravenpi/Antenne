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
	Paused    bool   `json:"paused"`
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
	Clips    *ClipBuffer
	encoder  *HLSEncoder

	fadeFrames int     // frames over which to fade between sources
	gain       float64 // current playlist gain (1 = full, 0 = fully ducked)

	timelineMu sync.Mutex
	timeline   []timelineEntry
	lastID     string // last observed playlist item ID (for change detection)

	listeners int64
	stop      chan struct{}
	loopDone  chan struct{}
	started   atomic.Bool
	once      sync.Once
}

// maxCatchupFrames caps how many frames the clock will emit in one tick to catch
// up after a scheduling stall. Beyond this we resync the baseline instead of
// spewing a burst that would desync the encoder's media timeline.
const maxCatchupFrames = 8

// timelineEntry records when a track began broadcasting (engine head time), so
// now-playing can be resolved by the listener's wall-clock accounting for HLS
// buffering delay.
type timelineEntry struct {
	At      time.Time
	TrackID string
	Title   string
	Artist  string
}

const maxTimelineEntries = 300

// NewEngine wires the sources and encoder together.
func NewEngine(pl *Playlist, live *LiveSource, enc *HLSEncoder, crossfadeMs int) *Engine {
	fade := crossfadeMs / FrameMs
	if fade < 1 {
		fade = 1
	}
	return &Engine{
		Playlist:   pl,
		Live:       live,
		Clips:      NewClipBuffer(),
		encoder:    enc,
		fadeFrames: fade,
		gain:       1.0,
		stop:       make(chan struct{}),
		loopDone:   make(chan struct{}),
	}
}

// Start launches the encoder and the real-time mixing loop.
func (e *Engine) Start() error {
	if err := e.encoder.Start(); err != nil {
		return err
	}
	e.started.Store(true)
	go e.loop()
	log.Print("engine: broadcasting")
	return nil
}

// Stop halts the loop (waiting for it to exit so nothing writes to the encoder
// concurrently), then the playlist decoder and the encoder.
func (e *Engine) Stop() {
	if !e.started.Load() {
		return
	}
	e.once.Do(func() { close(e.stop) })
	<-e.loopDone
	e.Playlist.Stop()
	e.encoder.Stop()
}

// loop is the broadcast clock. It ticks every 20 ms but paces emission against a
// monotonic frame counter, so a late tick (GC, scheduler) is caught up and the
// encoder is always fed at true real time — which keeps listeners' buffers
// healthy and prevents slow drift between media time and wall-clock.
func (e *Engine) loop() {
	defer close(e.loopDone)
	const frameDur = FrameMs * time.Millisecond
	ticker := time.NewTicker(frameDur)
	defer ticker.Stop()

	start := time.Now()
	var emitted int64
	for {
		select {
		case <-e.stop:
			return
		case <-ticker.C:
			target := int64(time.Since(start) / frameDur)
			if target-emitted > maxCatchupFrames {
				// Fell badly behind (long stall): resync rather than burst-feed.
				emitted = target - 1
			}
			for emitted < target {
				e.emitFrame()
				emitted++
			}
		}
	}
}

// emitFrame mixes one 20 ms frame from the live/playlist sources and hands it to
// the recorder and encoder. Every source read is non-blocking, so the clock is
// never held up by decode, network, or disk I/O.
func (e *Engine) emitFrame() {
	live := e.Live.Active()

	// Ramp the playlist gain down when live takes over, up when it ends.
	step := 1.0 / float64(e.fadeFrames)
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

	e.recordTimeline()
	e.Clips.Write(frame)
	if err := e.encoder.Write(frame); err != nil {
		log.Printf("engine: encoder write failed: %v", err)
	}
}

// NowPlaying returns the current broadcast state.
func (e *Engine) NowPlaying() NowPlaying {
	np := NowPlaying{
		Live:      e.Live.Active(),
		Paused:    e.Playlist.Paused(),
		Listeners: atomic.LoadInt64(&e.listeners),
	}
	if item, ok := e.Playlist.Current(); ok {
		np.Title = item.Title
		np.Artist = item.Artist
		np.TrackID = item.ID
	}
	return np
}

// recordTimeline appends a timeline entry whenever the playing item changes.
func (e *Engine) recordTimeline() {
	item, ok := e.Playlist.Current()
	if !ok || item.ID == "" {
		return
	}
	if item.ID == e.lastID {
		return
	}
	e.lastID = item.ID

	e.timelineMu.Lock()
	e.timeline = append(e.timeline, timelineEntry{
		At:      time.Now(),
		TrackID: item.ID,
		Title:   item.Title,
		Artist:  item.Artist,
	})
	if len(e.timeline) > maxTimelineEntries {
		e.timeline = e.timeline[len(e.timeline)-maxTimelineEntries:]
	}
	e.timelineMu.Unlock()
}

// NowPlayingAt resolves what was broadcasting at wall-clock time t: the entry
// with the greatest At <= t. Returns ok=false when there is no such entry.
func (e *Engine) NowPlayingAt(t time.Time) (title, artist, trackID string, ok bool) {
	e.timelineMu.Lock()
	defer e.timelineMu.Unlock()
	for i := len(e.timeline) - 1; i >= 0; i-- {
		if !e.timeline[i].At.After(t) {
			en := e.timeline[i]
			return en.Title, en.Artist, en.TrackID, true
		}
	}
	return "", "", "", false
}

// NextItem returns the upcoming playlist item.
func (e *Engine) NextItem() (title, artist, trackID string, ok bool) {
	item, ok := e.Playlist.Next()
	if !ok {
		return "", "", "", false
	}
	return item.Title, item.Artist, item.ID, true
}

// ExtractClipPCM returns the last N seconds of buffered mixed PCM.
func (e *Engine) ExtractClipPCM(seconds int) []byte {
	return e.Clips.Extract(seconds)
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
