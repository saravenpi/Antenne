package httpapi

import (
	"sync"
	"time"
)

// listenerTracker estimates the live audience by counting distinct client IPs
// that fetched the HLS manifest within a recent window. HLS is stateless, so
// this is the practical way to count listeners.
type listenerTracker struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
}

func newListenerTracker() *listenerTracker {
	return &listenerTracker{seen: make(map[string]time.Time), ttl: 20 * time.Second}
}

// hit records that ip requested the manifest just now.
func (t *listenerTracker) hit(ip string) {
	t.mu.Lock()
	t.seen[ip] = time.Now()
	t.mu.Unlock()
}

// count returns the number of distinct IPs seen within the TTL, pruning stale
// entries as it goes.
func (t *listenerTracker) count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	cut := time.Now().Add(-t.ttl)
	n := 0
	for ip, ts := range t.seen {
		if ts.After(cut) {
			n++
		} else {
			delete(t.seen, ip)
		}
	}
	return n
}
