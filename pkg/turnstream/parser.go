// Package turnstream parses the GM's reply as a line-framed stream of narration,
// speech, and control records, so segments can be attributed as they arrive
// rather than only once the whole reply is complete.
package turnstream

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
)

// Line classes.
const (
	KindNarration = "narration"
	KindSpeech    = "speech"
	KindRecord    = "record"
)

// Roster resolves a speaker name to an entity id. Declare adds a speaker that a
// persona record introduced mid-stream, so their first line can be attributed
// even though the declaration precedes the line that references it.
type Roster interface {
	Resolve(name string) (id string, ok bool)
	Declare(name, id string)
}

// Event is one parsed unit in stream order.
type Event struct {
	Kind      string
	Speaker   string
	SpeakerID string
	Text      string
	Record    *Record
}

// Parser turns text deltas into ordered events. It buffers a partial line until
// its newline arrives, which is what makes the event stream independent of how
// the provider chunked the text.
type Parser struct {
	roster  Roster
	buf     string
	pending []string
	records []Record
}

// NewParser builds a parser that attributes speech against roster.
func NewParser(roster Roster) *Parser {
	return &Parser{roster: roster}
}

// Feed adds streamed text and returns every event the new text completed.
func (p *Parser) Feed(text string) []Event {
	var events []Event
	p.buf += text
	for {
		i := strings.IndexByte(p.buf, '\n')
		if i < 0 {
			break
		}
		line := p.buf[:i]
		p.buf = p.buf[i+1:]
		events = append(events, p.consume(line)...)
	}
	return events
}

// Flush returns the event for any trailing partial line at end of stream.
func (p *Parser) Flush() []Event {
	var events []Event
	if p.buf != "" {
		events = append(events, p.consume(p.buf)...)
		p.buf = ""
	}
	events = append(events, p.flushNarration()...)
	return events
}

// consume classifies one complete line.
func (p *Parser) consume(line string) []Event {
	line = strings.TrimSuffix(line, "\r")
	trimmed := strings.TrimSpace(line)
	switch {
	case trimmed == "":
		return p.flushNarration()
	case strings.HasPrefix(trimmed, "@"):
		return append(p.flushNarration(), p.record(trimmed)...)
	case strings.HasPrefix(trimmed, ">"):
		return append(p.flushNarration(), p.speech(trimmed)...)
	default:
		p.pending = append(p.pending, trimmed)
		return nil
	}
}

// flushNarration emits the pending narration paragraph, if any. Narration lines
// accumulate until a blank line, a speech line, or a record flushes them, so a
// paragraph arrives as one segment rather than one per line.
func (p *Parser) flushNarration() []Event {
	if len(p.pending) == 0 {
		return nil
	}
	text := strings.Join(p.pending, "\n")
	p.pending = p.pending[:0]
	return []Event{{Kind: KindNarration, Text: text}}
}

// speech parses a blockquote line into a speech event when its speaker resolves,
// and into narration with the marker stripped otherwise, so an unattributed
// quote is never lost.
func (p *Parser) speech(line string) []Event {
	body := strings.TrimSpace(strings.TrimPrefix(line, ">"))
	name, text, ok := splitSpeaker(body)
	if !ok {
		return []Event{{Kind: KindNarration, Text: body}}
	}
	id, ok := p.roster.Resolve(name)
	if !ok {
		return []Event{{Kind: KindNarration, Text: body}}
	}
	return []Event{{Kind: KindSpeech, Speaker: name, SpeakerID: id, Text: text}}
}

// maxSpeakerLength bounds a candidate so a sentence cannot masquerade as a name.
const maxSpeakerLength = 64

// splitSpeaker splits "Name: utterance" at the first colon, stripping one
// wrapping pair of quotes from the utterance.
func splitSpeaker(body string) (name, text string, ok bool) {
	i := strings.IndexByte(body, ':')
	if i <= 0 {
		return "", "", false
	}
	name = strings.TrimSpace(body[:i])
	if name == "" || len(name) > maxSpeakerLength || strings.ContainsAny(name, "\n\"") {
		return "", "", false
	}
	text = trimQuotes(strings.TrimSpace(body[i+1:]))
	if text == "" {
		return "", "", false
	}
	return name, text, true
}

// trimQuotes removes one wrapping pair of straight or curly quotes.
func trimQuotes(s string) string {
	r := []rune(s)
	if len(r) < 2 {
		return s
	}
	pairs := map[rune]rune{'"': '"', '\u201c': '\u201d', '\u2018': '\u2019'}
	if pairs[r[0]] == r[len(r)-1] {
		return strings.TrimSpace(string(r[1 : len(r)-1]))
	}
	return s
}

// record parses an "@type payload" line. A malformed record is recorded with an
// error and produces no event, so the surrounding prose still parses.
func (p *Parser) record(line string) []Event {
	rest := strings.TrimPrefix(line, "@")
	typ, payload, _ := strings.Cut(rest, " ")
	rec := Record{
		Type:    strings.TrimSpace(typ),
		Payload: []byte(strings.TrimSpace(payload)),
		Line:    len(p.records) + 1,
	}
	switch {
	case !validRecordType(rec.Type):
		rec.Err = fmt.Errorf("unknown record type %q", rec.Type)
	case len(rec.Payload) == 0:
		rec.Err = fmt.Errorf("record %q has no payload", rec.Type)
	case !json.Valid(rec.Payload):
		rec.Err = fmt.Errorf("record %q payload is not JSON", rec.Type)
	}
	p.records = append(p.records, rec)
	if rec.Err != nil {
		return nil
	}
	if rec.Type == RecordPersona {
		p.declarePersona(rec)
	}
	out := p.records[len(p.records)-1]
	return []Event{{Kind: KindRecord, Record: &out}}
}

// declarePersona adds a declared character to the roster so their first line can
// be attributed even though the persona record precedes it.
func (p *Parser) declarePersona(rec Record) {
	var decl struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Payload, &decl); err != nil {
		return
	}
	if id := entity.Slugify(decl.Name); id != "" {
		p.roster.Declare(strings.TrimSpace(decl.Name), id)
	}
}

// Records returns every record seen so far, in order.
func (p *Parser) Records() []Record {
	out := make([]Record, len(p.records))
	copy(out, p.records)
	return out
}
