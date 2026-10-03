package turnstream

import (
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// Record types the GM may emit as an "@type {json}" line.
const (
	RecordPersona = "persona"
	RecordRoll    = "roll"
	RecordState   = "state"
	RecordMemory  = "memory"
	RecordMove    = "move"
)

// Record is one control line: its type, its raw JSON payload, its ordinal, and
// the error that made it unusable, if any. A record with an error is retained so
// a trace can explain why a declaration was dropped.
type Record struct {
	Type    string
	Payload []byte
	Line    int
	Err     error
}

// validRecordType reports whether a record type is recognised.
func validRecordType(typ string) bool {
	switch typ {
	case RecordPersona, RecordRoll, RecordState, RecordMemory, RecordMove:
		return true
	default:
		return false
	}
}

// DecodePersona decodes a persona record's payload.
func (r Record) DecodePersona() (harness.PersonaDecl, error) {
	var decl harness.PersonaDecl
	err := json.Unmarshal(r.Payload, &decl)
	return decl, err
}

// DecodeRoll decodes a roll record's payload into a check request.
func (r Record) DecodeRoll() (harness.CheckRequest, error) {
	var req harness.CheckRequest
	err := json.Unmarshal(r.Payload, &req)
	return req, err
}

// DecodeState decodes a state-change record's payload.
func (r Record) DecodeState() (harness.StateChangeDecl, error) {
	var change harness.StateChangeDecl
	err := json.Unmarshal(r.Payload, &change)
	return change, err
}

// DecodeMemory decodes a memory record's payload.
func (r Record) DecodeMemory() (harness.MemoryDecl, error) {
	var memory harness.MemoryDecl
	err := json.Unmarshal(r.Payload, &memory)
	return memory, err
}

// DecodeMove decodes a move record's payload into a location reference.
func (r Record) DecodeMove() (string, error) {
	var move struct {
		Location string `json:"location"`
	}
	if err := json.Unmarshal(r.Payload, &move); err != nil {
		return "", err
	}
	return move.Location, nil
}
