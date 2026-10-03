package turnstream

import "testing"

func TestRecordPayloadsDecode(t *testing.T) {
	roll := Record{Type: RecordRoll, Payload: []byte(`{"actor":"player","check_kind":"skill","stakes":"the bridge","outcomes":{"pass":"cross","fail":"fall"}}`)}
	req, err := roll.DecodeRoll()
	if err != nil || req.CheckKind != "skill" || req.Outcomes["pass"] != "cross" {
		t.Fatalf("DecodeRoll = %#v, %v", req, err)
	}

	move := Record{Type: RecordMove, Payload: []byte(`{"location":"[[aldon-harbour]]"}`)}
	loc, err := move.DecodeMove()
	if err != nil || loc != "[[aldon-harbour]]" {
		t.Fatalf("DecodeMove = %q, %v", loc, err)
	}

	persona := Record{Type: RecordPersona, Payload: []byte(`{"name":"Kae","type":"character","new":true}`)}
	decl, err := persona.DecodePersona()
	if err != nil || decl.Name != "Kae" || !decl.New {
		t.Fatalf("DecodePersona = %#v, %v", decl, err)
	}

	memory := Record{Type: RecordMemory, Payload: []byte(`{"kind":"event","entity_refs":["kae"],"text":"Kae arrived.","importance":2}`)}
	mem, err := memory.DecodeMemory()
	if err != nil || mem.Kind != "event" || mem.Importance != 2 {
		t.Fatalf("DecodeMemory = %#v, %v", mem, err)
	}

	state := Record{Type: RecordState, Payload: []byte(`{"entity":"kae","path":"hp","op":"sub","value":2}`)}
	change, err := state.DecodeState()
	if err != nil || change.Path != "hp" || change.Op != "sub" {
		t.Fatalf("DecodeState = %#v, %v", change, err)
	}
}
