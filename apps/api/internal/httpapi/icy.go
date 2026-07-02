package httpapi

import (
	"bufio"
	"strings"
)

// icyWriter wraps the raw socket and, when the client asked for metadata,
// interleaves an ICY metadata block after every metaInt bytes of audio. This is
// the SHOUTcast/Icecast in-band metadata convention: the audio stream carries a
// length-prefixed StreamTitle block at fixed byte intervals.
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
