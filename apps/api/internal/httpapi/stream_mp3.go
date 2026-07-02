package httpapi

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// defaultICYMetaInt is the fallback metadata interval. 16000 is Icecast's default
// and the value every player is proven to handle; overridable via ICY_METAINT.
const defaultICYMetaInt = 16000

// streamWriteTimeout bounds how long any single write to a listener may block. A
// client that stops reading (dead socket, full TCP window) is dropped instead of
// pinning a goroutine and a broadcaster slot forever.
const streamWriteTimeout = 30 * time.Second

// listenerPingEvery re-registers a connected MP3 listener with the tracker so a
// long-lived connection keeps counting past the tracker's TTL.
const listenerPingEvery = 10 * time.Second

// handleMP3Stream serves the continuous MP3 broadcast as an ICY/Icecast-style
// endless audio/mpeg body. If the client sent "Icy-MetaData: 1" we interleave a
// StreamTitle metadata block every icy-metaint bytes; otherwise it's raw MP3.
//
// We hijack the connection and write the response head by hand: an infinite
// stream must NOT be chunk-encoded (the #1 compatibility killer — chunk headers
// get decoded as audio), so we speak HTTP/1.0 with Connection: close and no
// Content-Length. The body is read until the socket closes, exactly as Icecast
// does.
func (s *Server) handleMP3Stream(w http.ResponseWriter, r *http.Request) {
	enc := s.engine.MP3()
	if enc == nil {
		writeErr(w, http.StatusNotFound, "mp3 stream disabled")
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	wantMeta := strings.TrimSpace(r.Header.Get("Icy-MetaData")) == "1"

	conn, brw, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()

	// Defeat Nagle so 20 ms audio frames aren't coalesced into laggy bursts.
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}

	writeDeadline := func() { _ = conn.SetWriteDeadline(time.Now().Add(streamWriteTimeout)) }

	writeDeadline()
	if _, err := brw.WriteString(s.mp3ResponseHead(wantMeta)); err != nil {
		return
	}
	if err := brw.Flush(); err != nil {
		return
	}

	ch, burst, cancel := enc.Bcast.Subscribe()
	defer cancel()

	ip := clientIP(r)
	s.listeners.hit(ip)
	lastPing := time.Now()

	iw := &icyWriter{w: brw.Writer, wantMeta: wantMeta, metaInt: s.icyMetaInt(), meta: s.currentStreamTitle}

	// Burst-on-connect: replay recent bytes so the player starts audio at once,
	// then pump live chunks until the client disconnects (a write error).
	send := func(p []byte) error {
		writeDeadline()
		if _, err := iw.Write(p); err != nil {
			return err
		}
		return brw.Flush()
	}

	if err := send(burst); err != nil {
		return
	}
	for chunk := range ch {
		if err := send(chunk); err != nil {
			return
		}
		if now := time.Now(); now.Sub(lastPing) >= listenerPingEvery {
			s.listeners.hit(ip)
			lastPing = now
		}
	}
}

// handleMP3StreamHead answers HEAD probes (players, directories, link
// validators) with the stream headers and no body.
func (s *Server) handleMP3StreamHead(w http.ResponseWriter, r *http.Request) {
	if s.engine.MP3() == nil {
		writeErr(w, http.StatusNotFound, "mp3 stream disabled")
		return
	}
	for _, kv := range s.icyHeaders(0) { // 0: no body, so no icy-metaint
		w.Header().Set(kv[0], kv[1])
	}
	w.WriteHeader(http.StatusOK)
}

// icyHeaders returns the response headers shared by the GET stream and HEAD
// probe, in wire order, so the two can never drift. metaInt > 0 appends
// icy-metaint (only the metadata-negotiated GET body carries it).
func (s *Server) icyHeaders(metaInt int) [][2]string {
	h := [][2]string{
		{"Content-Type", "audio/mpeg"},
		{"Cache-Control", "no-cache, no-store"},
		{"Access-Control-Allow-Origin", "*"},
		{"Server", "Antenne"},
		{"icy-name", headerSafe(s.cfg.StationName)},
		{"icy-genre", headerSafe(s.cfg.StationGenre)},
		{"icy-br", strconv.Itoa(s.cfg.MP3BitrateK)},
		{"icy-pub", s.icyPub()},
	}
	if s.cfg.StationURL != "" {
		h = append(h, [2]string{"icy-url", headerSafe(s.cfg.StationURL)})
	}
	if s.cfg.StationDesc != "" {
		h = append(h, [2]string{"icy-description", headerSafe(s.cfg.StationDesc)})
	}
	if metaInt > 0 {
		h = append(h, [2]string{"icy-metaint", strconv.Itoa(metaInt)})
	}
	return h
}

// mp3ResponseHead builds the raw ICY response header block for the hijacked
// connection. Icy-metaint is echoed only when the client opted into metadata.
func (s *Server) mp3ResponseHead(wantMeta bool) string {
	metaInt := 0
	if wantMeta {
		metaInt = s.icyMetaInt()
	}
	var b strings.Builder
	b.WriteString("HTTP/1.0 200 OK\r\n")
	for _, kv := range s.icyHeaders(metaInt) {
		b.WriteString(kv[0] + ": " + kv[1] + "\r\n")
	}
	b.WriteString("Connection: close\r\n")
	b.WriteString("\r\n")
	return b.String()
}

// handleStreamPLS returns a .pls playlist pointing at the MP3 stream — the file
// you hand to TuneIn, VLC "open network stream", or a hardware radio.
func (s *Server) handleStreamPLS(w http.ResponseWriter, r *http.Request) {
	url := absoluteURL(r, "/stream.mp3")
	w.Header().Set("Content-Type", "audio/x-scpls")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Disposition", `inline; filename="antenne.pls"`)
	fmt.Fprintf(w, "[playlist]\nNumberOfEntries=1\nFile1=%s\nTitle1=%s\nLength1=-1\nVersion=2\n",
		url, headerSafe(s.cfg.StationName))
}

// handleStreamM3U returns an extended .m3u playlist pointing at the MP3 stream.
// Some players/directories prefer m3u over pls.
func (s *Server) handleStreamM3U(w http.ResponseWriter, r *http.Request) {
	url := absoluteURL(r, "/stream.mp3")
	w.Header().Set("Content-Type", "audio/x-mpegurl")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Disposition", `inline; filename="antenne.m3u"`)
	fmt.Fprintf(w, "#EXTM3U\n#EXTINF:-1,%s\n%s\n", headerSafe(s.cfg.StationName), url)
}

// icyMetaInt returns the configured metadata interval, or the default.
func (s *Server) icyMetaInt() int {
	if s.cfg.ICYMetaInt > 0 {
		return s.cfg.ICYMetaInt
	}
	return defaultICYMetaInt
}

// icyPub maps the public-listing flag to the "1"/"0" the protocol expects.
func (s *Server) icyPub() string {
	if s.cfg.StationPublic {
		return "1"
	}
	return "0"
}

// currentStreamTitle formats the now-playing track for the ICY StreamTitle field.
func (s *Server) currentStreamTitle() string {
	np := s.engine.NowPlaying()
	switch {
	case np.Artist != "" && np.Title != "":
		return np.Artist + " - " + np.Title
	case np.Title != "":
		return np.Title
	default:
		return s.cfg.StationName
	}
}

// headerSafe removes CR/LF to prevent header injection via station/title config.
func headerSafe(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}

// absoluteURL builds an absolute URL for path from the request, honouring the
// reverse proxy's forwarded scheme and host.
func absoluteURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
		host = fh
	}
	return scheme + "://" + host + path
}
