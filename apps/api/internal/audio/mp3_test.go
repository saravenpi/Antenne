package audio

import (
	"os/exec"
	"testing"
	"time"
)

// TestMP3EncoderProducesStream drives the real ffmpeg pipeline: it feeds a second
// of silence frames and checks the broadcaster emits genuine MP3 (an 11-bit frame
// sync 0xFFEx). Skipped when ffmpeg is absent.
func TestMP3EncoderProducesStream(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}

	enc := NewMP3Encoder("ffmpeg", 128)
	if err := enc.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer enc.Stop()

	ch, _, cancel := enc.Bcast.Subscribe()
	defer cancel()

	// Feed ~1 s of silence (50 * 20 ms frames).
	go func() {
		for i := 0; i < 50; i++ {
			_ = enc.Write(silence())
		}
	}()

	var got []byte
	deadline := time.After(5 * time.Second)
	for len(got) < 512 {
		select {
		case chunk, ok := <-ch:
			if !ok {
				t.Fatal("broadcaster channel closed early")
			}
			got = append(got, chunk...)
		case <-deadline:
			t.Fatalf("timed out; only got %d bytes of MP3", len(got))
		}
	}

	if !hasMP3FrameSync(got) {
		t.Fatal("output does not contain an MP3 frame sync — not valid MP3")
	}
}

func hasMP3FrameSync(b []byte) bool {
	for i := 0; i+1 < len(b); i++ {
		if b[i] == 0xFF && b[i+1]&0xE0 == 0xE0 {
			return true
		}
	}
	return false
}
