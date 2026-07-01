package audio

import "sync"

// ring is a bounded FIFO of decoded audio frames shared between a single
// producer goroutine (an ffmpeg reader) and the real-time mixer.
//
// The two sides have deliberately asymmetric blocking behaviour:
//
//   - push blocks the producer while the buffer is full. Because the mixer
//     drains at exactly real time, this gives natural back-pressure all the way
//     down to ffmpeg (its output pipe fills, it stops reading its input), so a
//     decoder never races ahead of the broadcast and memory stays bounded.
//   - pop NEVER blocks the mixer. On underrun it returns ok=false and the clock
//     keeps ticking (emitting silence for that source). A transient starvation
//     therefore causes a few milliseconds of silence, not a stalled stream.
//
// This is the jitter buffer that decouples network/decoder hiccups from the
// broadcast clock.
type ring[T any] struct {
	mu      sync.Mutex
	notFull sync.Cond
	buf     []T
	head    int // index of the oldest element
	count   int
	closed  bool
}

func newRing[T any](capacity int) *ring[T] {
	if capacity < 1 {
		capacity = 1
	}
	r := &ring[T]{buf: make([]T, capacity)}
	r.notFull.L = &r.mu
	return r
}

// push appends a frame, blocking while the buffer is full. It returns false once
// the ring is closed (the producer should then stop).
func (r *ring[T]) push(v T) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for r.count == len(r.buf) && !r.closed {
		r.notFull.Wait()
	}
	if r.closed {
		return false
	}
	r.buf[(r.head+r.count)%len(r.buf)] = v
	r.count++
	return true
}

// pop removes and returns the oldest frame without ever blocking. ok is false
// when the buffer is empty (underrun).
func (r *ring[T]) pop() (T, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var zero T
	if r.count == 0 {
		return zero, false
	}
	v := r.buf[r.head]
	r.buf[r.head] = zero // drop our reference so the frame can be GC'd
	r.head = (r.head + 1) % len(r.buf)
	r.count--
	r.notFull.Signal()
	return v, true
}

// len reports the number of buffered frames.
func (r *ring[T]) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

// flush drops every buffered frame. Used when a control action (a manual skip,
// a playlist change) makes the buffered-ahead audio stale.
func (r *ring[T]) flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	var zero T
	for r.count > 0 {
		r.buf[r.head] = zero
		r.head = (r.head + 1) % len(r.buf)
		r.count--
	}
	r.notFull.Signal()
}

// close wakes any blocked producer and prevents further pushes. Frames already
// buffered stay poppable so the mixer can drain the tail.
func (r *ring[T]) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.notFull.Broadcast()
}
