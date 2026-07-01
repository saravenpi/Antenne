package audio

import (
	"io"
	"log"
	"os/exec"
	"sync"
	"time"
)

// playlistRingFrames is how far ahead the decoder runs (~1 s). This lookahead is
// what makes track transitions gapless: the next decoder is spawned while the
// mixer keeps draining buffered audio, so process-spawn latency never reaches
// the broadcast clock.
const playlistRingFrames = 50

// Item is a single playable entry in the 24/7 playlist.
type Item struct {
	ID     string
	Title  string
	Artist string
	Path   string // absolute path on disk
}

// plFrame is one decoded frame tagged with the item it belongs to, so the mixer
// can report now-playing based on what is actually on air rather than what the
// look-ahead decoder has already moved on to.
type plFrame struct {
	data  []byte
	item  Item
	index int
}

// Playlist decodes the current track to pivot PCM via ffmpeg and yields it frame
// by frame, advancing (and looping) automatically at end of track. A dedicated
// reader goroutine owns the decoder and fills a jitter buffer; the mixer pops
// from that buffer without ever blocking on decode or process spawn. The API is
// safe to call from HTTP handlers while the engine reads frames.
type Playlist struct {
	ffmpeg string

	mu      sync.Mutex
	items   []Item
	index   int // index the reader is currently decoding
	paused  bool
	gen     int      // bumped on every control action (skip/prev/play/replace)
	current *decoder // decoder owned by the reader (nil = none)

	// on-air position: the item/index of the last frame the mixer popped.
	playItem  Item
	playIndex int
	hasPlay   bool

	ring *ring[plFrame]
	wake chan struct{}
	quit chan struct{}
	done chan struct{}
}

// NewPlaylist creates an empty playlist bound to the given ffmpeg binary and
// starts its decoder goroutine.
func NewPlaylist(ffmpegBin string) *Playlist {
	p := &Playlist{
		ffmpeg: ffmpegBin,
		ring:   newRing[plFrame](playlistRingFrames),
		wake:   make(chan struct{}, 1),
		quit:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	go p.run()
	return p
}

// signalLocked nudges the reader to re-read control state. Caller may hold p.mu.
func (p *Playlist) signalLocked() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// run is the decoder goroutine: it keeps the jitter buffer full from the current
// track, advancing and looping at end of track. It blocks on ring.push when the
// buffer is full, which paces it to real time.
func (p *Playlist) run() {
	defer close(p.done)
	for {
		select {
		case <-p.quit:
			return
		default:
		}

		p.mu.Lock()
		idle := len(p.items) == 0 || p.paused
		if !idle && p.current == nil {
			item := p.items[p.index%len(p.items)]
			d, err := newDecoder(p.ffmpeg, item.Path)
			if err != nil {
				log.Printf("playlist: cannot decode %q: %v", item.Path, err)
				p.index = (p.index + 1) % len(p.items)
				p.mu.Unlock()
				time.Sleep(50 * time.Millisecond) // avoid a hot loop on a broken list
				continue
			}
			p.current = d
		}
		if idle {
			p.mu.Unlock()
			select {
			case <-p.wake:
			case <-time.After(100 * time.Millisecond):
			case <-p.quit:
				return
			}
			continue
		}
		d := p.current
		gen := p.gen
		item := p.items[p.index%len(p.items)]
		idx := p.index
		p.mu.Unlock()

		frame, err := d.read()
		if err != nil {
			// End of track, or the decoder was killed by a control action.
			p.mu.Lock()
			if p.gen == gen {
				// Natural end of track: advance and loop.
				p.stopCurrentLocked()
				if len(p.items) > 0 {
					p.index = (p.index + 1) % len(p.items)
				}
			}
			// Otherwise a control action already repositioned index and killed the
			// decoder; just loop and start the new one.
			p.mu.Unlock()
			continue
		}

		if !p.ring.push(plFrame{data: frame, item: item, index: idx}) {
			return // ring closed (shutdown)
		}
	}
}

// SetItems replaces the playlist. To avoid restarting the current track on every
// library edit, it only resets playback when the current index falls out of the
// new list.
func (p *Playlist) SetItems(items []Item) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.items = items
	if p.index >= len(items) {
		p.index = 0
		p.gen++
		p.stopCurrentLocked()
		p.ring.flush()
		p.hasPlay = false
		p.signalLocked()
	}
}

// Current returns the item that is on air right now (or false if the playlist is
// empty).
func (p *Playlist) Current() (Item, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.items) == 0 {
		return Item{}, false
	}
	if p.hasPlay {
		return p.playItem, true
	}
	return p.items[p.index%len(p.items)], true
}

// Next returns the upcoming item (wrapping to the top), or false when the
// playlist is empty.
func (p *Playlist) Next() (Item, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(p.items)
	if n == 0 {
		return Item{}, false
	}
	base := p.index
	if p.hasPlay {
		base = p.playIndex
	}
	return p.items[(base+1)%n], true
}

// ReadFrame returns the next 20 ms PCM frame from the jitter buffer, or silence
// when the playlist is empty, paused, or momentarily underrunning (e.g. the
// instant after a skip while the new decoder spins up).
func (p *Playlist) ReadFrame() []byte {
	p.mu.Lock()
	blocked := len(p.items) == 0 || p.paused
	p.mu.Unlock()
	if blocked {
		return silence()
	}
	f, ok := p.ring.pop()
	if !ok {
		return silence()
	}
	p.mu.Lock()
	p.playItem, p.playIndex, p.hasPlay = f.item, f.index, true
	p.mu.Unlock()
	return f.data
}

// Skip jumps to the next track immediately (flushing buffered look-ahead audio
// so the change is heard now, not a second later).
func (p *Playlist) Skip() { p.jump(1) }

// Prev jumps to the previous track immediately.
func (p *Playlist) Prev() { p.jump(-1) }

func (p *Playlist) jump(dir int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(p.items)
	if n == 0 {
		return
	}
	p.gen++
	p.stopCurrentLocked()
	p.ring.flush()
	p.hasPlay = false
	p.index = ((p.index+dir)%n + n) % n
	p.paused = false
	p.signalLocked()
}

// PlayID jumps straight to the track with the given ID and starts it now.
// Returns false when no such track exists in the current list.
func (p *Playlist) PlayID(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, it := range p.items {
		if it.ID == id {
			p.gen++
			p.stopCurrentLocked()
			p.ring.flush()
			p.hasPlay = false
			p.index = i
			p.paused = false
			p.signalLocked()
			return true
		}
	}
	return false
}

// SetPaused pauses or resumes the playlist (dead air while paused). Buffered
// audio is preserved so playback resumes exactly where it left off.
func (p *Playlist) SetPaused(v bool) {
	p.mu.Lock()
	p.paused = v
	p.signalLocked()
	p.mu.Unlock()
}

// Paused reports whether playback is currently paused.
func (p *Playlist) Paused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.paused
}

// Stop halts the decoder goroutine and tears down any running decoder.
func (p *Playlist) Stop() {
	p.mu.Lock()
	select {
	case <-p.quit:
	default:
		close(p.quit)
	}
	p.stopCurrentLocked()
	p.mu.Unlock()
	p.ring.close()
	p.signalLocked()
	<-p.done
}

func (p *Playlist) stopCurrentLocked() {
	if p.current != nil {
		p.current.close()
		p.current = nil
	}
}

// decoder wraps an ffmpeg process that streams one file as pivot PCM.
type decoder struct {
	cmd       *exec.Cmd
	out       io.ReadCloser
	buf       []byte
	closeOnce sync.Once
}

func newDecoder(ffmpeg, path string) (*decoder, error) {
	cmd := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-i", path,
		"-f", "s16le", "-ar", "48000", "-ac", "2", "-",
	)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &decoder{cmd: cmd, out: out, buf: make([]byte, FrameBytes)}, nil
}

// read fills exactly one frame, returning an error at end of stream or when the
// process has been killed.
func (d *decoder) read() ([]byte, error) {
	if _, err := io.ReadFull(d.out, d.buf); err != nil {
		return nil, err
	}
	frame := make([]byte, FrameBytes)
	copy(frame, d.buf)
	return frame, nil
}

func (d *decoder) close() {
	d.closeOnce.Do(func() {
		_ = d.out.Close()
		if d.cmd.Process != nil {
			_ = d.cmd.Process.Kill()
		}
		_ = d.cmd.Wait()
	})
}
