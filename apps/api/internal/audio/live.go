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

	mu     sync.Mutex
	active bool
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    io.ReadCloser
	buf    []byte
}

func NewLiveSource(ffmpegBin string) *LiveSource {
	return &LiveSource{ffmpeg: ffmpegBin, buf: make([]byte, FrameBytes)}
}

// Active reports whether the admin currently holds the mic.
func (l *LiveSource) Active() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.active
}

// Start opens an ffmpeg process that decodes the incoming container stream to
// pivot PCM. Call Write to feed encoded chunks and Stop to end the broadcast.
func (l *LiveSource) Start() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active {
		return nil
	}
	cmd := exec.Command(l.ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-i", "pipe:0",
		"-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1",
	)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	l.cmd, l.in, l.out, l.active = cmd, in, out, true
	log.Print("live: broadcast started")
	return nil
}

// Write feeds an encoded chunk from the browser into the decoder.
func (l *LiveSource) Write(chunk []byte) error {
	l.mu.Lock()
	in := l.in
	active := l.active
	l.mu.Unlock()
	if !active || in == nil {
		return nil
	}
	_, err := in.Write(chunk)
	return err
}

// ReadFrame returns the next decoded live frame, or a silence frame if the
// decoder underruns (e.g. brief network gap).
func (l *LiveSource) ReadFrame() []byte {
	l.mu.Lock()
	out := l.out
	active := l.active
	l.mu.Unlock()
	if !active || out == nil {
		return silence()
	}
	if _, err := io.ReadFull(out, l.buf); err != nil {
		return silence()
	}
	frame := make([]byte, FrameBytes)
	copy(frame, l.buf)
	return frame
}

// Stop ends the broadcast and tears down the decoder.
func (l *LiveSource) Stop() {
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
	l.cmd, l.in, l.out, l.active = nil, nil, nil, false
	log.Print("live: broadcast stopped")
}
