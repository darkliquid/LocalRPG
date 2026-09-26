package media

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/hajimehoshi/go-mp3"
)

// DecodeProviderAudio turns whatever a provider returned into interleaved s16
// PCM, so the pipeline can normalise it to Opus. It recognises headerless PCM
// (Gemini), WAV (Kokoro and friends), and MP3 (ElevenLabs and HTTP TTS).
func DecodeProviderAudio(data []byte, mimeType string) ([]int16, int, int, error) {
	if len(data) == 0 {
		return nil, 0, 0, fmt.Errorf("decode audio: empty payload")
	}

	switch {
	case bytes.HasPrefix(data, []byte("RIFF")):
		return decodeWAV(data)
	case IsPCMAudio(mimeType) && !looksLikeMP3(data):
		return decodePCMS16(data, PCMSampleRate(mimeType), 1)
	case looksLikeMP3(data) || strings.Contains(strings.ToLower(mimeType), "mpeg") || strings.Contains(strings.ToLower(mimeType), "mp3"):
		return decodeMP3(data)
	default:
		return nil, 0, 0, fmt.Errorf("decode audio: unsupported format %q", mimeType)
	}
}

func looksLikeMP3(data []byte) bool {
	if bytes.HasPrefix(data, []byte("ID3")) {
		return true
	}
	return len(data) > 1 && data[0] == 0xFF && data[1]&0xE0 == 0xE0
}

func decodePCMS16(data []byte, sampleRate, channels int) ([]int16, int, int, error) {
	if len(data)%2 != 0 {
		return nil, 0, 0, fmt.Errorf("decode pcm: odd byte count %d", len(data))
	}
	pcm := make([]int16, len(data)/2)
	for i := range pcm {
		pcm[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return pcm, sampleRate, channels, nil
}

func decodeWAV(data []byte) ([]int16, int, int, error) {
	if len(data) < 12 || string(data[8:12]) != "WAVE" {
		return nil, 0, 0, fmt.Errorf("decode wav: not a RIFF/WAVE file")
	}
	var (
		channels, bits, format int
		rate                   int
		payload                []byte
	)
	pos := 12
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		if pos+8+size > len(data) {
			break
		}
		body := data[pos+8 : pos+8+size]
		switch id {
		case "fmt ":
			if size < 16 {
				return nil, 0, 0, fmt.Errorf("decode wav: short fmt chunk")
			}
			format = int(binary.LittleEndian.Uint16(body[0:2]))
			channels = int(binary.LittleEndian.Uint16(body[2:4]))
			rate = int(binary.LittleEndian.Uint32(body[4:8]))
			bits = int(binary.LittleEndian.Uint16(body[14:16]))
		case "data":
			payload = body
		}
		pos += 8 + size
		if size%2 == 1 {
			pos++
		}
	}
	if format != 1 || payload == nil {
		return nil, 0, 0, fmt.Errorf("decode wav: only uncompressed PCM is supported")
	}
	if channels < 1 {
		channels = 1
	}
	switch bits {
	case 8:
		pcm := make([]int16, len(payload))
		for i, b := range payload {
			pcm[i] = int16(int(b)-128) << 8
		}
		return pcm, rate, channels, nil
	case 16:
		return decodePCMS16(payload, rate, channels)
	default:
		return nil, 0, 0, fmt.Errorf("decode wav: unsupported bit depth %d", bits)
	}
}

func decodeMP3(data []byte) ([]int16, int, int, error) {
	decoder, err := mp3.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("decode mp3: %w", err)
	}
	raw, err := io.ReadAll(decoder)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("decode mp3: %w", err)
	}
	// go-mp3 always emits 16-bit stereo.
	const channels = 2
	pcm := make([]int16, len(raw)/2)
	for i := range pcm {
		pcm[i] = int16(binary.LittleEndian.Uint16(raw[i*2:]))
	}
	return pcm, decoder.SampleRate(), channels, nil
}
