package audio

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// HLSEncoder consumes mixed pivot PCM and produces an HLS playlist (live.m3u8 +
// rolling .ts segments) in a directory served statically to listeners.
type HLSEncoder struct {
	ffmpeg   string
	dir      string
	segmentS int
	listSize int

	cmd *exec.Cmd
	in  io.WriteCloser
}

func NewHLSEncoder(ffmpegBin, dir string, segmentS, listSize int) *HLSEncoder {
	return &HLSEncoder{ffmpeg: ffmpegBin, dir: dir, segmentS: segmentS, listSize: listSize}
}

// Playlist is the path of the generated HLS manifest.
func (h *HLSEncoder) Playlist() string {
	return filepath.Join(h.dir, "live.m3u8")
}

// Start launches the encoder process. PCM written via Write is encoded to AAC
// and segmented in real time (the engine paces writes to a 20 ms clock).
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
	return nil
}

// Write pushes one PCM frame to the encoder.
func (h *HLSEncoder) Write(frame []byte) error {
	if h.in == nil {
		return fmt.Errorf("hls: encoder not started")
	}
	_, err := h.in.Write(frame)
	return err
}

// Stop tears down the encoder process.
func (h *HLSEncoder) Stop() {
	if h.in != nil {
		_ = h.in.Close()
	}
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
		_ = h.cmd.Wait()
	}
	h.cmd, h.in = nil, nil
}
