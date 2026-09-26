// Package opus encodes and decodes Ogg/Opus audio. It is the one on-disk format
// for generated speech: any provider output is normalised to Ogg/Opus so playback
// only ever decodes a single codec.
package opus

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// maxSegments is the number of lacing values one Ogg page may carry.
const maxSegments = 255

// Writer muxes Opus packets into a standard Ogg stream (RFC 7845): an OpusHead
// page, an OpusTags page, then audio pages.
type Writer struct {
	w       io.Writer
	serial  uint32
	seq     uint32
	lacing  []byte
	data    bytes.Buffer
	granule uint64
	closed  bool
}

// NewWriter writes the two Ogg/Opus header pages and returns a writer for the
// audio packets. serial identifies the logical stream.
func NewWriter(w io.Writer, channels uint8, preSkip uint16, inputSampleRate uint32, serial uint32) (*Writer, error) {
	writer := &Writer{w: w, serial: serial}

	head := &bytes.Buffer{}
	head.WriteString("OpusHead")
	head.WriteByte(1) // version
	head.WriteByte(channels)
	_ = binary.Write(head, binary.LittleEndian, preSkip)
	_ = binary.Write(head, binary.LittleEndian, inputSampleRate)
	_ = binary.Write(head, binary.LittleEndian, uint16(0)) // output gain
	head.WriteByte(0)                                      // channel mapping family

	tags := &bytes.Buffer{}
	tags.WriteString("OpusTags")
	vendor := "localrpg"
	_ = binary.Write(tags, binary.LittleEndian, uint32(len(vendor)))
	tags.WriteString(vendor)
	_ = binary.Write(tags, binary.LittleEndian, uint32(0)) // no user comments

	if err := writer.writeSoloPage(head.Bytes(), 0x02, 0); err != nil {
		return nil, err
	}
	if err := writer.writeSoloPage(tags.Bytes(), 0x00, 0); err != nil {
		return nil, err
	}
	return writer, nil
}

// writeSoloPage writes one packet alone in a page (used for the headers).
func (w *Writer) writeSoloPage(packet []byte, headerType byte, granule uint64) error {
	page := buildPage(headerType, granule, w.serial, w.seq, lace(packet), packet)
	if _, err := w.w.Write(page); err != nil {
		return fmt.Errorf("opus: write page: %w", err)
	}
	w.seq++
	return nil
}

// WritePacket adds one Opus packet. granule is the number of 48 kHz samples
// decodable through this packet, pre-skip included (RFC 7845 section 4). eos
// marks the last packet of the stream.
func (w *Writer) WritePacket(packet []byte, granule uint64, eos bool) error {
	if w.closed {
		return fmt.Errorf("opus: writer is closed")
	}
	segments := len(packet)/255 + 1
	if len(w.lacing)+segments > maxSegments {
		if err := w.flush(w.granule); err != nil {
			return err
		}
	}
	w.lacing = append(w.lacing, lace(packet)...)
	w.data.Write(packet)
	w.granule = granule
	if eos {
		return w.flush(granule)
	}
	return nil
}

// Close flushes any buffered audio as a final page.
func (w *Writer) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if len(w.lacing) == 0 {
		return nil
	}
	return w.flush(w.granule)
}

func (w *Writer) flush(granule uint64) error {
	if len(w.lacing) == 0 {
		return nil
	}
	page := buildPage(0, granule, w.serial, w.seq, w.lacing, w.data.Bytes())
	if _, err := w.w.Write(page); err != nil {
		return fmt.Errorf("opus: write page: %w", err)
	}
	w.seq++
	w.lacing = w.lacing[:0]
	w.data.Reset()
	w.granule = 0
	return nil
}

// lace returns the Ogg segment table for a packet.
func lace(packet []byte) []byte {
	table := make([]byte, 0, len(packet)/255+1)
	for n := len(packet); n >= 255; n -= 255 {
		table = append(table, 255)
	}
	return append(table, byte(len(packet)%255))
}

func buildPage(headerType byte, granule uint64, serial, seq uint32, lacing, data []byte) []byte {
	page := make([]byte, 27+len(lacing)+len(data))
	copy(page[0:4], "OggS")
	page[4] = 0
	page[5] = headerType
	binary.LittleEndian.PutUint64(page[6:14], granule)
	binary.LittleEndian.PutUint32(page[14:18], serial)
	binary.LittleEndian.PutUint32(page[18:22], seq)
	// CRC is computed with the field zeroed.
	page[26] = byte(len(lacing))
	copy(page[27:], lacing)
	copy(page[27+len(lacing):], data)
	binary.LittleEndian.PutUint32(page[22:26], oggCRC(page))
	return page
}

var crcTable = func() *[256]uint32 {
	table := &[256]uint32{}
	for i := 0; i < 256; i++ {
		crc := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ 0x04c11db7
			} else {
				crc <<= 1
			}
		}
		table[i] = crc
	}
	return table
}()

// oggCRC is the Ogg page checksum: CRC-32, polynomial 0x04c11db7, no reflection.
func oggCRC(data []byte) uint32 {
	var crc uint32
	for _, b := range data {
		crc = (crc << 8) ^ crcTable[byte(crc>>24)^b]
	}
	return crc
}
