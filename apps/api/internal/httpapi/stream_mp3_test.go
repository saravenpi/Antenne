package httpapi

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

// TestICYWriterInterleave checks the ICY framing is byte-exact: exactly
// defaultICYMetaInt audio bytes, then a length-prefixed padded metadata block, then the
// audio resumes — and an unchanged title collapses to a single zero byte.
func TestICYWriterInterleave(t *testing.T) {
	var out bytes.Buffer
	bw := bufio.NewWriter(&out)
	iw := &icyWriter{w: bw, wantMeta: true, metaInt: defaultICYMetaInt, meta: func() string { return "Daft Punk - Aerodynamic" }}

	// Feed 2*defaultICYMetaInt+5 audio bytes in awkward chunk sizes to exercise splitting.
	audio := bytes.Repeat([]byte{0xAB}, 2*defaultICYMetaInt+5)
	for off := 0; off < len(audio); off += 7000 {
		end := off + 7000
		if end > len(audio) {
			end = len(audio)
		}
		if _, err := iw.Write(audio[off:end]); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	got := out.Bytes()

	// First defaultICYMetaInt bytes must be pure audio.
	if !bytes.Equal(got[:defaultICYMetaInt], bytes.Repeat([]byte{0xAB}, defaultICYMetaInt)) {
		t.Fatal("first block is not pure audio")
	}

	// Then a metadata block: length byte, then that many 16-byte units.
	lenByte := int(got[defaultICYMetaInt])
	if lenByte == 0 {
		t.Fatal("expected a non-empty metadata block on first boundary")
	}
	metaStart := defaultICYMetaInt + 1
	metaEnd := metaStart + lenByte*16
	meta := string(bytes.TrimRight(got[metaStart:metaEnd], "\x00"))
	if want := "StreamTitle='Daft Punk - Aerodynamic';"; meta != want {
		t.Fatalf("metadata = %q, want %q", meta, want)
	}

	// Audio resumes immediately after the block.
	if got[metaEnd] != 0xAB {
		t.Fatal("audio did not resume after metadata block")
	}

	// Second boundary: title unchanged -> single zero byte.
	secondBoundary := metaEnd + defaultICYMetaInt
	if got[secondBoundary] != 0 {
		t.Fatalf("expected zero-length metadata on unchanged title, got %d", got[secondBoundary])
	}
}

// TestICYWriterNoMeta verifies a client that did not ask for metadata gets raw
// bytes with nothing injected.
func TestICYWriterNoMeta(t *testing.T) {
	var out bytes.Buffer
	bw := bufio.NewWriter(&out)
	iw := &icyWriter{w: bw, wantMeta: false, meta: func() string { return "x" }}
	audio := bytes.Repeat([]byte{0x01}, defaultICYMetaInt*2)
	if _, err := iw.Write(audio); err != nil {
		t.Fatal(err)
	}
	bw.Flush()
	if !bytes.Equal(out.Bytes(), audio) {
		t.Fatal("raw stream must be untouched when metadata not requested")
	}
}

// TestICYEscape ensures a hostile title cannot break the StreamTitle framing.
func TestICYEscape(t *testing.T) {
	got := icyEscape("Rock 'n' Roll; DROP TABLE\r\n")
	if strings.ContainsAny(got, "';\r\n") {
		t.Fatalf("icyEscape left framing chars in %q", got)
	}
}
