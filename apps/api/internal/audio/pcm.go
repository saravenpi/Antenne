package audio

// Pivot PCM format used everywhere inside the engine: signed 16-bit
// little-endian, 48 kHz, stereo. Every source is decoded to this format, mixed,
// then encoded once for HLS.
const (
	SampleRate   = 48000
	Channels     = 2
	BytesPerSmp  = 2 // int16
	FrameMs      = 20
	FrameSamples = SampleRate * FrameMs / 1000           // per channel: 960
	FrameBytes   = FrameSamples * Channels * BytesPerSmp // 3840
)

// silence returns a fresh zeroed frame (one 20 ms slice of stereo PCM).
func silence() []byte {
	return make([]byte, FrameBytes)
}

// scaleFrame multiplies every sample in a frame by gain in-place (0.0–1.0),
// used for fades and ducking. It mutates and returns buf.
func scaleFrame(buf []byte, gain float64) []byte {
	if gain >= 1.0 {
		return buf
	}
	for i := 0; i+1 < len(buf); i += 2 {
		s := int16(uint16(buf[i]) | uint16(buf[i+1])<<8)
		s = int16(float64(s) * gain)
		buf[i] = byte(s)
		buf[i+1] = byte(s >> 8)
	}
	return buf
}
