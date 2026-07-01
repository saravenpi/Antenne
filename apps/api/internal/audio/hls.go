package audio

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// hlsWriteBuffer is how many mixed frames may queue for the encoder (~4 s). A
// deep buffer absorbs a transient ffmpeg/disk stall without stalling the clock;
// only a sustained stall overflows it and starts dropping frames.
const hlsWriteBuffer = 200

// HLSEncoder consumes mixed pivot PCM and produces an HLS playlist (live.m3u8 +
// rolling .ts segments) in a directory served statically to listeners. Writes go
// through a buffered channel drained by a dedicated goroutine, so the real-time
// clock never blocks on ffmpeg or the filesystem.
type HLSEncoder struct {
	ffmpeg   string
	dir      string
	segmentS int
	listSize int

	cmd    *exec.Cmd
	in     io.WriteCloser
	frames chan []byte
	done   chan struct{}
}

func NewHLSEncoder(ffmpegBin, dir string, segmentS, listSize int) *HLSEncoder {
	return &HLSEncoder{ffmpeg: ffmpegBin, dir: dir, segmentS: segmentS, listSize: listSize}
}

// Playlist is the path of the generated HLS manifest.
func (h *HLSEncoder) Playlist() string {
	return filepath.Join(h.dir, "live.m3u8")
}

// Start launches the encoder process and its writer goroutine. PCM written via
// Write is encoded to AAC and segmented in real time (the engine paces writes to
// a 20 ms clock).
func (h *HLSEncoder) Start() error {
	if err := os.MkdirAll(h.dir, 0o755); err != nil {
		return err
	}
	cmd := exec.Command(h.ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-f", "s16le", "-ar", "48000", "-ac", "2", "-i", "pipe:0",
		"-c:a", "aac", "-b:a", "128k",
		"-f", "hls",
		"-hls_time", strconv.Itoa(h.segmentS),
		"-hls_list_size", strconv.Itoa(h.listSize),
		"-hls_flags", "delete_segments+append_list+omit_endlist+program_date_time",
		"-hls_segment_filename", filepath.Join(h.dir, "seg_%05d.ts"),
		h.Playlist(),
	)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	h.cmd, h.in = cmd, in
	h.frames = make(chan []byte, hlsWriteBuffer)
	h.done = make(chan struct{})
	go h.writeLoop()
	return nil
}

// writeLoop drains queued frames into ffmpeg's stdin.
func (h *HLSEncoder) writeLoop() {
	defer close(h.done)
	for frame := range h.frames {
		if _, err := h.in.Write(frame); err != nil {
			log.Printf("hls: encoder write failed: %v", err)
			// Keep draining the channel so producers never block; ffmpeg is gone.
			for range h.frames {
			}
			return
		}
	}
}

// Write queues one PCM frame for the encoder without blocking. If the encoder is
// not draining fast enough (a rare, sustained ffmpeg/disk stall) the frame is
// dropped rather than freezing the broadcast clock for every listener.
func (h *HLSEncoder) Write(frame []byte) error {
	if h.frames == nil {
		return fmt.Errorf("hls: encoder not started")
	}
	select {
	case h.frames <- frame:
	default:
		log.Print("hls: encoder backlogged, dropping a frame")
	}
	return nil
}

// Stop tears down the encoder process and its writer goroutine.
func (h *HLSEncoder) Stop() {
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill() // unblock a stuck stdin write
	}
	if h.frames != nil {
		close(h.frames)
		h.frames = nil
	}
	if h.done != nil {
		<-h.done
		h.done = nil
	}
	if h.in != nil {
		_ = h.in.Close()
	}
	if h.cmd != nil {
		_ = h.cmd.Wait()
	}
	h.cmd, h.in = nil, nil
}
