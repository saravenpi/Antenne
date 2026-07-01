package httpapi

import (
	"bufio"
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

	// Burst-on-connect: replay recent bytes so the player starts audio at once.
	writeDeadline()
	if _, err := iw.Write(burst); err != nil {
		return
	}
	if err := brw.Flush(); err != nil {
		return
	}

	for chunk := range ch {
		writeDeadline()
		if _, err := iw.Write(chunk); err != nil {
			return
		}
		if err := brw.Flush(); err != nil {
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
	h := w.Header()
	h.Set("Content-Type", "audio/mpeg")
	h.Set("Cache-Control", "no-cache, no-store")
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("icy-name", headerSafe(s.cfg.StationName))
	h.Set("icy-genre", headerSafe(s.cfg.StationGenre))
	h.Set("icy-br", strconv.Itoa(s.cfg.MP3BitrateK))
	h.Set("icy-pub", s.icyPub())
	if s.cfg.StationURL != "" {
		h.Set("icy-url", headerSafe(s.cfg.StationURL))
	}
	w.WriteHeader(http.StatusOK)
}

// mp3ResponseHead builds the raw ICY response header block for the hijacked
// connection. Icy-metaint is echoed only when the client opted into metadata.
func (s *Server) mp3ResponseHead(wantMeta bool) string {
	var b strings.Builder
	b.WriteString("HTTP/1.0 200 OK\r\n")
	b.WriteString("Content-Type: audio/mpeg\r\n")
	b.WriteString("Cache-Control: no-cache, no-store\r\n")
	b.WriteString("Access-Control-Allow-Origin: *\r\n")
	b.WriteString("Server: Antenne\r\n")
	b.WriteString("icy-name: " + headerSafe(s.cfg.StationName) + "\r\n")
	b.WriteString("icy-genre: " + headerSafe(s.cfg.StationGenre) + "\r\n")
	b.WriteString("icy-br: " + strconv.Itoa(s.cfg.MP3BitrateK) + "\r\n")
	b.WriteString("icy-pub: " + s.icyPub() + "\r\n")
	if s.cfg.StationURL != "" {
		b.WriteString("icy-url: " + headerSafe(s.cfg.StationURL) + "\r\n")
	}
	if s.cfg.StationDesc != "" {
		b.WriteString("icy-description: " + headerSafe(s.cfg.StationDesc) + "\r\n")
	}
	if wantMeta {
		b.WriteString("icy-metaint: " + strconv.Itoa(s.icyMetaInt()) + "\r\n")
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

// icyWriter wraps the raw socket and, when the client asked for metadata,
// interleaves an ICY metadata block after every metaInt bytes of audio.
type icyWriter struct {
	w        *bufio.Writer
	wantMeta bool
	metaInt  int
	meta     func() string

	sinceMeta int
	lastMeta  string
	started   bool // ensures the first boundary always transmits the title
}

// Write forwards audio bytes, splitting at metadata boundaries when metadata is
// enabled. It always reports len(p) written on success so callers see a normal
// io.Writer (the injected metadata bytes are invisible to them).
func (iw *icyWriter) Write(p []byte) (int, error) {
	if !iw.wantMeta {
		return iw.w.Write(p)
	}
	total := 0
	for len(p) > 0 {
		n := iw.metaInt - iw.sinceMeta
		if n > len(p) {
			n = len(p)
		}
		if _, err := iw.w.Write(p[:n]); err != nil {
			return total, err
		}
		total += n
		iw.sinceMeta += n
		p = p[n:]
		if iw.sinceMeta == iw.metaInt {
			if err := iw.writeMetaBlock(); err != nil {
				return total, err
			}
			iw.sinceMeta = 0
		}
	}
	return total, nil
}

// writeMetaBlock emits one ICY metadata segment: a length byte (in 16-byte
// units) followed by the padded payload. When the title is unchanged we emit a
// single zero byte, as the protocol prescribes, to avoid re-sending it.
func (iw *icyWriter) writeMetaBlock() error {
	title := iw.meta()
	if iw.started && title == iw.lastMeta {
		return iw.w.WriteByte(0)
	}
	iw.started = true
	iw.lastMeta = title

	payload := "StreamTitle='" + icyEscape(title) + "';"
	blocks := (len(payload) + 15) / 16
	if blocks > 255 { // length byte is a single byte; clamp defensively
		blocks = 255
		payload = payload[:255*16]
	}
	buf := make([]byte, 1+blocks*16)
	buf[0] = byte(blocks)
	copy(buf[1:], payload)
	_, err := iw.w.Write(buf)
	return err
}

// icyEscape strips the characters that would break the StreamTitle='...';
// framing. The ICY protocol defines no escaping, so removal is the only correct
// option — a raw quote or semicolon in a title corrupts the block for parsers.
func icyEscape(s string) string {
	return strings.NewReplacer("'", "", ";", " ", "\r", " ", "\n", " ").Replace(s)
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
