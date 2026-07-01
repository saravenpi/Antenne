package store

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// Store persists uploaded audio files on local disk.
type Store struct {
	dir     string
	ffprobe string
}

// New ensures the storage directory exists. ffprobe is derived from the ffmpeg
// binary path (they ship together).
func New(dir, ffmpegBin string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	ffprobe := "ffprobe"
	if strings.HasSuffix(ffmpegBin, "ffmpeg") {
		ffprobe = strings.TrimSuffix(ffmpegBin, "ffmpeg") + "ffprobe"
	}
	return &Store{dir: dir, ffprobe: ffprobe}, nil
}

// Dir returns the storage root.
func (s *Store) Dir() string { return s.dir }

// Path resolves a stored filename to an absolute path.
func (s *Store) Path(filename string) string {
	return filepath.Join(s.dir, filename)
}

// Save copies an uploaded stream to disk, preserving the original extension, and
// returns the generated filename.
func (s *Store) Save(r io.Reader, originalName string) (string, error) {
	ext := filepath.Ext(originalName)
	filename := uuid.NewString() + ext
	dst, err := os.Create(s.Path(filename))
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, r); err != nil {
		return "", err
	}
	return filename, nil
}

// Delete removes a stored file (missing files are ignored).
func (s *Store) Delete(filename string) error {
	err := os.Remove(s.Path(filename))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Duration probes an audio file's length in seconds via ffprobe.
func (s *Store) Duration(filename string) float64 {
	out, err := exec.Command(s.ffprobe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		s.Path(filename),
	).Output()
	if err != nil {
		return 0
	}
	d, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	return d
}
