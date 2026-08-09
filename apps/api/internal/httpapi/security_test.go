package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/saravenpi/poste/internal/config"
)

func TestClientIPIgnoresSpoofedHeadersFromUntrustedPeer(t *testing.T) {
	s := &Server{trustedProxies: parseTrustedProxies("")} // default private ranges

	// Direct hit from a public IP: forwarded headers must NOT be trusted, else a
	// client could forge an IP to evade bans / slow-mode / login limits.
	r := &http.Request{
		RemoteAddr: "203.0.113.7:54321",
		Header:     http.Header{},
	}
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.Header.Set("CF-Connecting-IP", "5.6.7.8")

	if got := s.clientIP(r); got != "203.0.113.7" {
		t.Fatalf("expected untrusted peer to use RemoteAddr, got %q", got)
	}
}

func TestClientIPHonorsHeadersFromTrustedProxy(t *testing.T) {
	s := &Server{trustedProxies: parseTrustedProxies("")}

	// Peer is a private-range proxy (Traefik): its CF-Connecting-IP is believed.
	r := &http.Request{
		RemoteAddr: "10.0.0.5:44444",
		Header:     http.Header{},
	}
	r.Header.Set("CF-Connecting-IP", "5.6.7.8")

	if got := s.clientIP(r); got != "5.6.7.8" {
		t.Fatalf("expected trusted proxy header to be honoured, got %q", got)
	}
}

func TestCheckOriginRejectsCrossSite(t *testing.T) {
	s := &Server{cfg: config.Config{ClientOrigin: "https://radio.example.com"}}
	up := s.newUpgrader()

	// Cross-site origin against a different host: reject (CSWSH guard).
	evil := &http.Request{Host: "radio.example.com", Header: http.Header{}}
	evil.Header.Set("Origin", "https://evil.example.com")
	if up.CheckOrigin(evil) {
		t.Fatal("expected cross-site origin to be rejected")
	}

	// Configured origin: allow.
	ok := &http.Request{Host: "radio.example.com", Header: http.Header{}}
	ok.Header.Set("Origin", "https://radio.example.com")
	if !up.CheckOrigin(ok) {
		t.Fatal("expected configured origin to be allowed")
	}

	// No Origin (native player / curl): allow — a victim browser always sends one.
	native := &http.Request{Host: "radio.example.com", Header: http.Header{}}
	if !up.CheckOrigin(native) {
		t.Fatal("expected missing-origin request to be allowed")
	}
}

func TestSanitizeURIRedactsToken(t *testing.T) {
	got := sanitizeURI("/api/chat/ws?token=supersecretjwt&foo=bar")
	if want := "/api/chat/ws?foo=bar&token=REDACTED"; got != want {
		t.Fatalf("token not redacted: got %q want %q", got, want)
	}
	// URIs without a token pass through untouched.
	if got := sanitizeURI("/api/now-playing"); got != "/api/now-playing" {
		t.Fatalf("unexpected rewrite: %q", got)
	}
}

func TestValidBackground(t *testing.T) {
	ok := []string{
		"",
		"#0a0a0a",
		"linear-gradient(#000, #fff)",
		`url("data:image/jpeg;base64,AAAA") center/cover no-repeat fixed`,
	}
	for _, s := range ok {
		if !validBackground(s) {
			t.Errorf("expected %q to be a valid background", s)
		}
	}
	bad := []string{
		`url("https://evil.example/x.png")`,
		`url(http://evil.example/x.png)`,
		`<img src=x onerror=alert(1)>`,
		"expression(alert(1))",
		"@import url(x)",
		`url("javascript:alert(1)")`,
	}
	for _, s := range bad {
		if validBackground(s) {
			t.Errorf("expected %q to be rejected", s)
		}
	}
}

func TestValidLinkURL(t *testing.T) {
	if !validLinkURL("https://x.com") || !validLinkURL("mailto:a@b.c") {
		t.Fatal("expected https/mailto to be allowed")
	}
	for _, s := range []string{"javascript:alert(1)", "data:text/html,x", "  javascript:alert(1)"} {
		if validLinkURL(s) {
			t.Errorf("expected %q to be rejected", s)
		}
	}
}

func TestAllowedAudioExt(t *testing.T) {
	if !allowedAudioExt("song.MP3") || !allowedAudioExt("a.flac") {
		t.Fatal("expected common audio extensions to be allowed")
	}
	for _, s := range []string{"payload.html", "x.exe", "noext", "y.svg"} {
		if allowedAudioExt(s) {
			t.Errorf("expected %q to be rejected", s)
		}
	}
}

func TestLoginLimiterBlocksAfterMax(t *testing.T) {
	l := newLoginLimiter(3, time.Minute) // 3 per minute
	now := time.Unix(1_700_000_000, 0)
	for i := 0; i < 3; i++ {
		if !l.allow("1.1.1.1", now) {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if l.allow("1.1.1.1", now) {
		t.Fatal("4th attempt in window should be blocked")
	}
	// A different IP is unaffected.
	if !l.allow("2.2.2.2", now) {
		t.Fatal("distinct IP should not be rate-limited")
	}
}
