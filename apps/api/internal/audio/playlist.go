package audio

import (
	"io"
	"log"
	"os/exec"
	"sync"
)

// Item is a single playable entry in the 24/7 playlist.
type Item struct {
	ID     string
	Title  string
	Artist string
	Path   string // absolute path on disk
}

// Playlist decodes the current track to pivot PCM via ffmpeg and yields it frame
// by frame, advancing (and looping) automatically at end of track. It is safe
// for the engine to read frames from one goroutine while the API replaces the
// item list from another.
type Playlist struct {
	ffmpeg string

	mu      sync.Mutex
	items   []Item
	index   int
	current *decoder
}

// NewPlaylist creates an empty playlist bound to the given ffmpeg binary.
func NewPlaylist(ffmpegBin string) *Playlist {
	return &Playlist{ffmpeg: ffmpegBin}
}

// SetItems replaces the playlist. If the currently playing item is gone, the
// next frame starts the new list from the top.
func (p *Playlist) SetItems(items []Item) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.items = items
	if p.index >= len(items) {
		p.index = 0
		p.stopCurrent()
	}
}

// Current returns the item that is playing right now (or false if the playlist
// is empty).
func (p *Playlist) Current() (Item, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.items) == 0 {
		return Item{}, false
	}
	return p.items[p.index%len(p.items)], true
}

// ReadFrame returns the next 20 ms PCM frame, advancing tracks and looping as
// needed. It returns a silence frame when the playlist is empty.
func (p *Playlist) ReadFrame() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.items) == 0 {
		return silence()
	}
	if p.current == nil {
		p.startCurrent()
	}

	frame, err := p.current.read()
	if err != nil {
		// End of track (or decode error): advance and loop.
		p.stopCurrent()
		p.index = (p.index + 1) % len(p.items)
		p.startCurrent()
		if frame2, err2 := p.current.read(); err2 == nil {
			return frame2
		}
		return silence()
	}
	return frame
}

func (p *Playlist) startCurrent() {
	item := p.items[p.index%len(p.items)]
	d, err := newDecoder(p.ffmpeg, item.Path)
	if err != nil {
		log.Printf("playlist: cannot decode %q: %v", item.Path, err)
		return
	}
	p.current = d
}

func (p *Playlist) stopCurrent() {
	if p.current != nil {
		p.current.close()
		p.current = nil
	}
}

// decoder wraps an ffmpeg process that streams one file as pivot PCM.
type decoder struct {
	cmd *exec.Cmd
	out io.ReadCloser
	buf []byte
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

// read fills exactly one frame, returning io.EOF at end of stream.
func (d *decoder) read() ([]byte, error) {
	if _, err := io.ReadFull(d.out, d.buf); err != nil {
		return nil, err
	}
	frame := make([]byte, FrameBytes)
	copy(frame, d.buf)
	return frame, nil
}

func (d *decoder) close() {
	_ = d.out.Close()
	if d.cmd.Process != nil {
		_ = d.cmd.Process.Kill()
	}
	_ = d.cmd.Wait()
}
