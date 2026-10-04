package media

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"strings"
)

// DefaultPCMRate is the sample rate assumed for headerless speech when the
// provider's MIME type does not name one. Gemini TTS returns 24 kHz mono PCM.
const DefaultPCMRate = 24000

// IsPCMAudio reports whether a provider MIME type names headerless PCM audio,
// for example "audio/L16;codec=pcm;rate=24000". Such a clip needs a WAV
// container before a decoder or a browser can play it.
func IsPCMAudio(mimeType string) bool {
	m := strings.ToLower(strings.TrimSpace(mimeType))
	return strings.Contains(m, "l16") || strings.Contains(m, "pcm") || m == "audio/raw"
}

// PCMSampleRate extracts the "rate=NNNN" from a PCM MIME type, falling back to
// the Gemini default.
func PCMSampleRate(mimeType string) int {
	for _, part := range strings.Split(strings.ToLower(mimeType), ";") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "rate=") {
			continue
		}
		if rate, err := strconv.Atoi(strings.TrimPrefix(part, "rate=")); err == nil && rate > 0 && rate <= 192000 {
			return rate
		}
	}
	return DefaultPCMRate
}

// WrapPCMAsWAV prepends a 44-byte RIFF/WAVE header to 16-bit little-endian mono
// PCM so the clip behaves like any other WAV. A clip that already carries a
// container is returned unchanged.
func WrapPCMAsWAV(pcm []byte, sampleRate int) []byte {
	if bytes.HasPrefix(pcm, []byte("RIFF")) {
		return pcm
	}
	if sampleRate <= 0 || sampleRate > 192000 {
		sampleRate = DefaultPCMRate
	}
	const channels = 1
	const bitsPerSample = 16
	blockAlign := channels * bitsPerSample / 8
	byteRate := sampleRate * blockAlign
	dataSize := len(pcm)

	var buf bytes.Buffer
	buf.Grow(44 + dataSize)
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(bitsPerSample))
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dataSize))
	buf.Write(pcm)
	return buf.Bytes()
}
