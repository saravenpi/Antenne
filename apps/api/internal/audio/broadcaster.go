package audio

import "sync"

// mp3SubBuffer is how many encoded chunks may queue for one listener before we
// declare them too slow and drop the connection. At ~128 kbit/s a chunk is a few
// KB and arrives every few ms, so this is several seconds of slack — enough to
// ride out a stalled TCP window without letting a dead client pin memory.
const mp3SubBuffer = 512

// mp3BurstBytes is the rolling tail of the stream replayed to every new listener
// the instant they connect. Handing a player ~1 s of already-encoded MP3 lets its
// decoder lock onto a frame boundary and start audio immediately instead of
// waiting for the next live bytes. This is the classic "burst-on-connect".
const mp3BurstBytes = 64 * 1024

// Broadcaster fans an encoded byte stream (the continuous MP3) out to many
// concurrent HTTP listeners. Each subscriber owns a buffered channel; a listener
// that cannot keep up is dropped rather than stalling the encoder or the other
// listeners (the ICY convention — a slow client reconnects and re-primes). All
// state is guarded by one mutex; publish is called from a single encoder
// goroutine, Subscribe/unsubscribe from HTTP handler goroutines.
type Broadcaster struct {
	mu    sync.Mutex
	subs  map[*subscriber]struct{}
	burst []byte
}

type subscriber struct {
	ch     chan []byte
	closed bool
}

// NewBroadcaster returns an empty broadcaster ready to accept subscribers.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subs: make(map[*subscriber]struct{})}
}

// publish delivers one freshly encoded chunk to every listener and appends it to
// the burst buffer. It never blocks: a listener whose buffer is full is closed
// and removed. The caller may reuse its buffer after publish returns — the chunk
// is copied once here and shared (read-only) by all subscribers.
func (b *Broadcaster) publish(chunk []byte) {
	if len(chunk) == 0 {
		return
	}
	c := make([]byte, len(chunk))
	copy(c, chunk)

	b.mu.Lock()
	defer b.mu.Unlock()

	b.burst = append(b.burst, c...)
	if len(b.burst) > mp3BurstBytes {
		trimmed := make([]byte, mp3BurstBytes)
		copy(trimmed, b.burst[len(b.burst)-mp3BurstBytes:])
		b.burst = trimmed
	}

	for s := range b.subs {
		select {
		case s.ch <- c:
		default:
			// Too slow: drop them. Closing the channel ends their handler loop.
			s.closed = true
			close(s.ch)
			delete(b.subs, s)
		}
	}
}

// Subscribe registers a new listener. It returns the listener's chunk channel, a
// snapshot of the current burst buffer to send first, and a cancel func the
// handler must defer to unregister on disconnect. The channel is closed either by
// cancel or by publish dropping a slow listener; ranging over it terminates the
// handler either way.
func (b *Broadcaster) Subscribe() (<-chan []byte, []byte, func()) {
	s := &subscriber{ch: make(chan []byte, mp3SubBuffer)}

	b.mu.Lock()
	burst := make([]byte, len(b.burst))
	copy(burst, b.burst)
	b.subs[s] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		if !s.closed {
			s.closed = true
			close(s.ch)
			delete(b.subs, s)
		}
		b.mu.Unlock()
	}
	return s.ch, burst, cancel
}

// Close disconnects every current listener by closing their channels, which ends
// each listener's handler loop. Called when the encoder shuts down for good so
// no goroutine is left blocked waiting for bytes that will never come. Safe to
// call more than once.
func (b *Broadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		if !s.closed {
			s.closed = true
			close(s.ch)
		}
		delete(b.subs, s)
	}
}
