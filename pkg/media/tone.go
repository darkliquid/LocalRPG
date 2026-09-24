package media

import (
	"bytes"
	"encoding/binary"
	"math"
)

// GenerateToneWAV generates a valid PCM WAV audio tone of the given frequency
// and duration. It is the portable fallback and the probe signal for STT tests.
func GenerateToneWAV(freq float64, durationSec float64) []byte {
	sampleRate := 44100
	numSamples := int(float64(sampleRate) * durationSec)
	dataSize := numSamples * 2 // 16-bit = 2 bytes per sample

	var buf bytes.Buffer

	// RIFF header
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16)) // subchunk size
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // Mono
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*2)) // ByteRate
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))            // BlockAlign
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))           // BitsPerSample

	// data chunk
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dataSize))

	for i := 0; i < numSamples; i++ {
		t := float64(i) / float64(sampleRate)
		val := math.Sin(2.0 * math.Pi * freq * t)
		// Envelope decay
		env := 1.0 - (float64(i) / float64(numSamples))
		sample := int16(val * env * 16000.0)
		_ = binary.Write(&buf, binary.LittleEndian, sample)
	}

	return buf.Bytes()
}
