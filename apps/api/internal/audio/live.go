package audio

import (
	"io"
	"log"
	"os/exec"
	"sync"
)

// LiveSource receives the admin's microphone stream (WebM/Opus chunks pushed
// from the browser) and decodes it to pivot PCM via ffmpeg. The engine pulls
// decoded frames while a broadcast is active.
type LiveSource struct {
	ffmpeg string

	mu       sync.Mutex
	active   bool // engine should keep pulling live frames
	draining bool // stdin closed; flushing the decoder's tail before ending
	cmd      *exec.Cmd
	in       io.WriteCloser
	out      io.ReadCloser
	buf      []byte
}

func NewLiveSource(ffmpegBin string) *LiveSource {
	return &LiveSource{ffmpeg: ffmpegBin, buf: make([]byte, FrameBytes)}
}

// Active reports whether the admin currently holds the mic. It stays true while
// the decoder is draining its tail, so the engine keeps broadcasting the final
// words instead of cutting them off.
func (l *LiveSource) Active() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.active
}

// Start opens an ffmpeg process that decodes the incoming container stream to
// pivot PCM. It returns started=false (without error) when a broadcast is
// already running, so callers can reject a second concurrent ingest connection
// rather than corrupting the single decoder with two interleaved streams.
func (l *LiveSource) Start() (started bool, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active {
		return false, nil
	}
	cmd := exec.Command(l.ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-i", "pipe:0",
		"-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1",
	)
	in, err := cmd.StdinPipe()
	if err != nil {
		return false, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return false, err
	}
	if err := cmd.Start(); err != nil {
		return false, err
	}
	l.cmd, l.in, l.out, l.active, l.draining = cmd, in, out, true, false
	log.Print("live: broadcast started")
	return true, nil
}

// Write feeds an encoded chunk from the browser into the decoder. Chunks that
// arrive after Stop (during the drain) are dropped: stdin is already closed.
func (l *LiveSource) Write(chunk []byte) error {
	l.mu.Lock()
	in := l.in
	active := l.active
	draining := l.draining
	l.mu.Unlock()
	if !active || draining || in == nil {
		return nil
	}
	_, err := in.Write(chunk)
	return err
}

// ReadFrame returns the next decoded live frame. When the decoder underruns or
// finishes flushing its tail (EOF after Stop), it finalises the broadcast and
// returns silence so the engine can fade back to the playlist.
func (l *LiveSource) ReadFrame() []byte {
	l.mu.Lock()
	out := l.out
	active := l.active
	l.mu.Unlock()
	if !active || out == nil {
		return silence()
	}
	if _, err := io.ReadFull(out, l.buf); err != nil {
		l.finish()
		return silence()
	}
	frame := make([]byte, FrameBytes)
	copy(frame, l.buf)
	return frame
}

// Stop begins a graceful shutdown: it closes ffmpeg's stdin so the decoder
// flushes any buffered audio (the tail of the admin's speech), while leaving
// the broadcast active. ReadFrame keeps emitting those final frames until the
// decoder hits EOF, then finish() tears everything down. This is what makes the
// end of a mic take reach listeners instead of being cut on click.
func (l *LiveSource) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.active || l.draining {
		return
	}
	l.draining = true
	if l.in != nil {
		_ = l.in.Close() // EOF -> ffmpeg flushes remaining decoded PCM
		l.in = nil
	}
	log.Print("live: draining broadcast tail")
}

// finish tears down the decoder once its output is fully drained.
func (l *LiveSource) finish() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.active {
		return
	}
	if l.in != nil {
		_ = l.in.Close()
	}
	if l.out != nil {
		_ = l.out.Close()
	}
	if l.cmd != nil && l.cmd.Process != nil {
		_ = l.cmd.Process.Kill()
		_ = l.cmd.Wait()
	}
	l.cmd, l.in, l.out, l.active, l.draining = nil, nil, nil, false, false
	log.Print("live: broadcast stopped")
}
