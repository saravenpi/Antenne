package httpapi

import (
	"net/http"
	"sync"
	"time"
)

// loginLimiter is a small per-IP fixed-window rate limiter guarding the login
// endpoint against brute-force attacks. bcrypt slows each guess, but does not by
// itself stop a patient attacker — especially against weak passwords.
type loginLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string]*windowCounter
}

type windowCounter struct {
	start time.Time
	count int
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{max: max, window: window, hits: make(map[string]*windowCounter)}
}

// allow records an attempt for ip and reports whether it is still within the
// limit for the current window.
func (l *loginLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Opportunistic pruning to keep the map bounded under a flood of distinct IPs.
	if len(l.hits) > 10000 {
		for k, c := range l.hits {
			if now.Sub(c.start) > l.window {
				delete(l.hits, k)
			}
		}
	}

	c, ok := l.hits[ip]
	if !ok || now.Sub(c.start) > l.window {
		l.hits[ip] = &windowCounter{start: now, count: 1}
		return true
	}
	c.count++
	return c.count <= l.max
}

// rateLimitLogin is middleware that throttles login attempts per client IP.
func (s *Server) rateLimitLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.loginLimiter.allow(s.clientIP(r), time.Now()) {
			w.Header().Set("Retry-After", "60")
			writeErr(w, http.StatusTooManyRequests, "too many attempts, try again later")
			return
		}
		next.ServeHTTP(w, r)
	})
}
