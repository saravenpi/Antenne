package audio

import (
	"bufio"
	"io"
	"log"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// mp3WriteBuffer mirrors hlsWriteBuffer: how many mixed PCM frames may queue for
// the encoder (~4 s) before a sustained ffmpeg stall starts dropping frames.
const mp3WriteBuffer = 200

// mp3RestartBackoff is how long the supervisor waits before relaunching ffmpeg
// after it exits, so a persistent failure (bad binary) doesn't spin the CPU.
const mp3RestartBackoff = 500 * time.Millisecond

// MP3Encoder encodes the mixed pivot PCM to a single continuous MP3 stream and
// fans it out to HTTP listeners through its Broadcaster. This is the classic
// "internet radio" output: an endless audio/mpeg body any player can read
// (browser, VLC, hardware radio), with ICY metadata injected by the HTTP layer.
//
// It is a second, independent sink alongside the HLS encoder — same non-blocking
// write model, so the real-time broadcast clock is never held up by ffmpeg. PCM
// goes in via Write; encoded MP3 comes out of ffmpeg's stdout and is published to
// the Broadcaster. A supervisor goroutine restarts ffmpeg if it ever dies, so a
// transient encoder crash is a brief gap of audio, not a permanently dead stream
// with every listener wedged — the broadcaster and listeners survive restarts.
type MP3Encoder struct {
	ffmpeg   string
	bitrateK int
	Bcast    *Broadcaster

	frames   chan []byte
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// NewMP3Encoder builds an encoder at the given bitrate (kbit/s, defaults to 128).
func NewMP3Encoder(ffmpegBin string, bitrateK int) *MP3Encoder {
	if bitrateK <= 0 {
		bitrateK = 128
	}
	return &MP3Encoder{ffmpeg: ffmpegBin, bitrateK: bitrateK, Bcast: NewBroadcaster()}
}

// Start launches the supervisor. It returns immediately; the first ffmpeg spawn
// happens in the background (HLS shares the same binary and starts synchronously,
// so a misconfigured ffmpeg is already caught there — an MP3 hiccup must never
// take down the whole station).
func (m *MP3Encoder) Start() error {
	m.frames = make(chan []byte, mp3WriteBuffer)
	m.stop = make(chan struct{})
	m.done = make(chan struct{})
	go m.supervise()
	return nil
}

// supervise runs ffmpeg sessions back-to-back until Stop, restarting after a
// short backoff whenever a session ends.
func (m *MP3Encoder) supervise() {
	defer close(m.done)
	for {
		select {
		case <-m.stop:
			return
		default:
		}
		if err := m.runOnce(); err != nil {
			log.Printf("mp3: encoder session ended: %v", err)
		}
		select {
		case <-m.stop:
			return
		case <-time.After(mp3RestartBackoff):
		}
	}
}

// runOnce spawns one ffmpeg process, pumps PCM into it and MP3 out of it, and
// returns when the process dies or Stop is requested. It never lets cmd.Wait
// race an in-flight stdout read (the stdlib forbids that): the read goroutine is
// joined before Wait.
func (m *MP3Encoder) runOnce() error {
	cmd := exec.Command(m.ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-f", "s16le", "-ar", "48000", "-ac", "2", "-i", "pipe:0",
		"-c:a", "libmp3lame", "-b:a", strconv.Itoa(m.bitrateK)+"k",
		"-f", "mp3", "pipe:1",
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

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		m.readLoop(out)
	}()

	pumpErr := m.pumpFrames(in)

	_ = in.Close()
	if cmd.Process != nil {
		_ = cmd.Process.Kill() // unblock the stdout read so readLoop can exit
	}
	<-readDone     // no Read in flight before Wait (stdlib requirement)
	_ = cmd.Wait() // reap the process
	return pumpErr
}

// pumpFrames drains queued PCM frames into ffmpeg's stdin until Stop or a write
// error (ffmpeg died — the supervisor will relaunch).
func (m *MP3Encoder) pumpFrames(in io.Writer) error {
	for {
		select {
		case <-m.stop:
			return nil
		case frame := <-m.frames:
			if _, err := in.Write(frame); err != nil {
				return err
			}
		}
	}
}

// readLoop pumps encoded MP3 out of ffmpeg's stdout into the broadcaster. It
// returns on EOF/error (process exit); the supervisor decides whether to restart.
func (m *MP3Encoder) readLoop(out io.Reader) {
	r := bufio.NewReaderSize(out, 16*1024)
	buf := make([]byte, 8*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			m.Bcast.publish(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// Write queues one PCM frame without blocking. A frame is dropped rather than
// freezing the broadcast clock if the encoder falls behind or is mid-restart.
// The sole caller is the engine loop, which stops before Stop is called.
func (m *MP3Encoder) Write(frame []byte) error {
	select {
	case m.frames <- frame:
	default:
		log.Print("mp3: encoder backlogged, dropping a frame")
	}
	return nil
}

// Stop halts the supervisor (tearing down ffmpeg) and disconnects every listener
// so no handler goroutine is left blocked. Safe to call once.
func (m *MP3Encoder) Stop() {
	if m.stop == nil {
		return // never started
	}
	m.stopOnce.Do(func() { close(m.stop) })
	<-m.done
	m.Bcast.Close()
}
