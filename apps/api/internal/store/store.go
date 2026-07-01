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

// ExtractCover pulls an embedded cover image out of an audio file and writes it
// next to the source as "<name>.jpg", returning that filename. Returns ok=false
// when the file has no embedded art. Uses the ffmpeg binary derived alongside
// ffprobe.
func (s *Store) ExtractCover(filename string) (string, bool) {
	ffmpeg := strings.TrimSuffix(s.ffprobe, "ffprobe") + "ffmpeg"
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	cover := base + ".jpg"
	dst := s.Path(cover)
	// -frames:v 1 grabs the attached picture (audio files expose it as a video
	// stream) and re-encodes it to JPEG; -update 1 allows a single-image output.
	err := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", s.Path(filename),
		"-an", "-frames:v", "1", "-update", "1",
		dst,
	).Run()
	if err != nil {
		_ = os.Remove(dst)
		return "", false
	}
	if info, statErr := os.Stat(dst); statErr != nil || info.Size() == 0 {
		_ = os.Remove(dst)
		return "", false
	}
	return cover, true
}

// Metadata probes an audio file's embedded title/artist tags via ffprobe.
// Missing tags come back as empty strings. Container tag keys are
// case-insensitive across formats, so ffprobe normalises them for us.
func (s *Store) Metadata(filename string) (title, artist string) {
	out, err := exec.Command(s.ffprobe,
		"-v", "error",
		"-show_entries", "format_tags=title,artist",
		"-of", "default=noprint_wrappers=1",
		s.Path(filename),
	).Output()
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimPrefix(key, "TAG:")) {
		case "title":
			title = strings.TrimSpace(val)
		case "artist":
			artist = strings.TrimSpace(val)
		}
	}
	return title, artist
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
