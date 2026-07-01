package clips

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/google/uuid"
)

// Service encodes slices of the live broadcast to MP3 on local disk.
type Service struct {
	dir    string
	ffmpeg string
}

// New ensures the clips directory exists.
func New(dir, ffmpegBin string) (*Service, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Service{dir: dir, ffmpeg: ffmpegBin}, nil
}

// Dir returns the clips root.
func (s *Service) Dir() string { return s.dir }

// Path resolves a stored clip filename to an absolute path.
func (s *Service) Path(filename string) string {
	return filepath.Join(s.dir, filename)
}

// Encode pipes pivot PCM (s16le 48k stereo) to ffmpeg and writes an MP3 file.
// It returns the generated filename (<uuid>.mp3).
func (s *Service) Encode(pcm []byte) (string, error) {
	filename := uuid.NewString() + ".mp3"
	cmd := exec.Command(s.ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-f", "s16le", "-ar", "48000", "-ac", "2", "-i", "pipe:0",
		"-c:a", "libmp3lame", "-b:a", "192k",
		"-y", s.Path(filename),
	)
	cmd.Stdin = bytes.NewReader(pcm)
	if err := cmd.Run(); err != nil {
		_ = os.Remove(s.Path(filename))
		return "", err
	}
	return filename, nil
}

// Delete removes a stored clip file (missing files are ignored).
func (s *Service) Delete(filename string) error {
	err := os.Remove(s.Path(filename))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
