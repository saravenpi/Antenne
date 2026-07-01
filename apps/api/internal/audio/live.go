package audio

import (
	"io"
	"log"
	"os/exec"
	"sync"
	"sync/atomic"
)

const (
	// liveRingFrames caps how much decoded live audio we buffer ahead (~2 s). A
	// generous cap lets a post-stall catch-up burst be absorbed rather than
	// dropped.
	liveRingFrames = 100
	// livePrebufferFrames is the jitter-buffer depth we fill before the live
	// feed goes to air (~300 ms). It trades a little latency for immunity to
	// network jitter and the browser's chunked (timeslice) delivery — the DJ's
	// audio no longer cuts when a packet is late.
	livePrebufferFrames = 15
)

// LiveSource receives the admin's microphone stream (WebM/Opus chunks pushed
// from the browser) and decodes it to pivot PCM via ffmpeg. A dedicated reader
// goroutine drains ffmpeg into a jitter buffer, so network hiccups never stall
// the real-time mixer and — crucially — never end the broadcast: a transient
// underrun yields brief silence and recovers when audio resumes, instead of the
// old behaviour where a single short read tore the whole take down.
type LiveSource struct {
	ffmpeg string

	mu          sync.Mutex
	started     bool // ffmpeg running; the engine may pull from us
	prebuffered bool // jitter buffer has filled once; the feed is on air
	draining    bool // stdin closed; flushing the decoder's tail before ending
	finished    bool // decoder drained and torn down
	cmd         *exec.Cmd
	in          io.WriteCloser
	ring        *ring[[]byte]
	eof         atomic.Bool // reader saw ffmpeg reach EOF
}

func NewLiveSource(ffmpegBin string) *LiveSource {
	return &LiveSource{ffmpeg: ffmpegBin}
}

// Active reports whether the live feed should be mixed to air. It becomes true
// only once the jitter buffer has prebuffered (so switching to live never opens
// with a gap) and stays true while draining the tail after Stop.
func (l *LiveSource) Active() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.started || l.finished {
		return false
	}
	// ffmpeg exited and everything buffered has been played: end the take.
	if l.eof.Load() && (l.ring == nil || l.ring.len() == 0) {
		l.finishLocked()
		return false
	}
	if l.draining || l.prebuffered {
		return true
	}
	if l.ring != nil && l.ring.len() >= livePrebufferFrames {
		l.prebuffered = true
		return true
	}
	// Very short take: ffmpeg already hit EOF before the buffer filled. Go active
	// to drain the little that was captured.
	if l.eof.Load() {
		return true
	}
	return false
}

// Start opens an ffmpeg process that decodes the incoming container stream to
// pivot PCM and launches the reader goroutine. It returns started=false (without
// error) when a broadcast is already running, so callers can reject a second
// concurrent ingest connection rather than corrupting the single decoder with
// two interleaved streams.
func (l *LiveSource) Start() (started bool, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.started {
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
	l.cmd, l.in = cmd, in
	l.ring = newRing[[]byte](liveRingFrames)
	l.started, l.prebuffered, l.draining, l.finished = true, false, false, false
	l.eof.Store(false)
	go l.readLoop(out, l.ring)
	log.Print("live: broadcast started")
	return true, nil
}

// readLoop pulls decoded PCM from ffmpeg one frame at a time into the jitter
// buffer until EOF (stdin closed, decoder flushed, or process killed).
func (l *LiveSource) readLoop(out io.ReadCloser, rb *ring[[]byte]) {
	buf := make([]byte, FrameBytes)
	for {
		if _, err := io.ReadFull(out, buf); err != nil {
			l.eof.Store(true)
			rb.close() // wake the mixer's drain; buffered tail stays poppable
			_ = out.Close()
			return
		}
		frame := make([]byte, FrameBytes)
		copy(frame, buf)
		if !rb.push(frame) {
			return // ring closed (broadcast finished)
		}
	}
}

// Write feeds an encoded chunk from the browser into the decoder. Chunks that
// arrive after Stop (during the drain) are dropped: stdin is already closed.
func (l *LiveSource) Write(chunk []byte) error {
	l.mu.Lock()
	in := l.in
	ok := l.started && !l.draining
	l.mu.Unlock()
	if !ok || in == nil {
		return nil
	}
	_, err := in.Write(chunk)
	return err
}

// ReadFrame returns the next decoded live frame from the jitter buffer. On
// underrun it returns silence: if the decoder has finished and the buffer is
// drained the take ends, otherwise it is a transient hiccup and the broadcast
// carries on.
func (l *LiveSource) ReadFrame() []byte {
	l.mu.Lock()
	rb := l.ring
	dead := !l.started || l.finished || rb == nil
	l.mu.Unlock()
	if dead {
		return silence()
	}
	if frame, ok := rb.pop(); ok {
		return frame
	}
	if l.eof.Load() {
		l.mu.Lock()
		l.finishLocked()
		l.mu.Unlock()
		return silence()
	}
	return silence() // transient underrun — keep the broadcast alive
}

// Stop begins a graceful shutdown: it closes ffmpeg's stdin so the decoder
// flushes any buffered audio (the tail of the admin's speech), while leaving the
// broadcast active. ReadFrame keeps emitting those final frames until the
// decoder hits EOF, then finish tears everything down. This is what makes the
// end of a mic take reach listeners instead of being cut on click.
func (l *LiveSource) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.started || l.draining || l.finished {
		return
	}
	l.draining = true
	if l.in != nil {
		_ = l.in.Close() // EOF -> ffmpeg flushes remaining decoded PCM
		l.in = nil
	}
	log.Print("live: draining broadcast tail")
}

// finishLocked tears down the decoder once its output is fully drained. The
// caller must hold l.mu.
func (l *LiveSource) finishLocked() {
	if l.finished {
		return
	}
	if l.in != nil {
		_ = l.in.Close()
	}
	if l.ring != nil {
		l.ring.close()
	}
	if l.cmd != nil && l.cmd.Process != nil {
		_ = l.cmd.Process.Kill()
		_ = l.cmd.Wait()
	}
	l.cmd, l.in = nil, nil
	l.started, l.prebuffered, l.draining, l.finished = false, false, false, true
	log.Print("live: broadcast stopped")
}
