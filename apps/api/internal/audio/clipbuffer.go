package audio

import "sync"

// clipBufferFrames is the ring capacity: ~120 seconds of mixed PCM at 50
// frames/sec (20 ms frames).
const clipBufferFrames = 6000

// ClipBuffer is a mutex-protected ring buffer holding the most recent mixed PCM
// frames, so the admin can retroactively record a slice of the live broadcast.
type ClipBuffer struct {
	mu     sync.Mutex
	frames [][]byte // ring of copied frames
	next   int      // index to write next
	count  int      // number of valid frames (<= cap)
}

// NewClipBuffer allocates an empty ring buffer.
func NewClipBuffer() *ClipBuffer {
	return &ClipBuffer{frames: make([][]byte, clipBufferFrames)}
}

// Write copies one mixed PCM frame into the ring, evicting the oldest.
func (b *ClipBuffer) Write(frame []byte) {
	cp := make([]byte, len(frame))
	copy(cp, frame)

	b.mu.Lock()
	defer b.mu.Unlock()
	b.frames[b.next] = cp
	b.next = (b.next + 1) % len(b.frames)
	if b.count < len(b.frames) {
		b.count++
	}
}

// Extract returns the last N seconds of buffered PCM, concatenated in
// chronological order. The request is clamped to the available data.
func (b *ClipBuffer) Extract(seconds int) []byte {
	if seconds < 0 {
		seconds = 0
	}
	want := seconds * (1000 / FrameMs) // frames per second = 50

	b.mu.Lock()
	defer b.mu.Unlock()

	if want > b.count {
		want = b.count
	}
	if want == 0 {
		return nil
	}

	out := make([]byte, 0, want*FrameBytes)
	// The oldest of the wanted frames sits `want` slots behind the write head.
	start := (b.next - want + len(b.frames)) % len(b.frames)
	for i := 0; i < want; i++ {
		idx := (start + i) % len(b.frames)
		out = append(out, b.frames[idx]...)
	}
	return out
}
