package engine

import (
	"encoding/json"
	"testing"

	"github.com/darkliquid/localrpg/pkg/turnstream"
)

func TestRecordReportNilWhenClean(t *testing.T) {
	o := &TurnOrchestrator{parser: turnstream.NewParser(streamTestRoster{})}
	o.parser.Feed("@roll {\"actor\":\"x\",\"check_kind\":\"do\"}\n")
	if got := o.recordReport(); got != nil {
		t.Fatalf("clean turn report = %+v, want nil", got)
	}
}

func TestRecordReportCountsAndCaps(t *testing.T) {
	o := &TurnOrchestrator{parser: turnstream.NewParser(streamTestRoster{})}
	o.parser.Feed("@roll {\"actor\":\"x\",\"check_kind\":\"do\",}\n") // repaired
	o.parser.Feed("@roll not json at all\n")                          // dropped
	for i := 0; i < 8; i++ {
		o.parser.Feed("@move not json\n") // more drops to exceed the cap
	}
	rep := o.recordReport()
	if rep == nil || rep.Repaired != 1 || rep.Failed != 9 {
		t.Fatalf("report = %+v, want repaired 1 failed 9", rep)
	}
	if len(rep.Issues) > maxRecordIssues {
		t.Fatalf("issues = %d, want <= %d", len(rep.Issues), maxRecordIssues)
	}
}

func TestRecordReportJSONRoundTrip(t *testing.T) {
	turn := Turn{Number: 1, RecordReport: &RecordReport{
		Total: 2, Repaired: 1, Failed: 1,
		Issues: []RecordIssue{{Type: "roll", Repair: "trailing_comma"}, {Type: "move", Error: "not JSON"}},
	}}
	data, err := json.Marshal(turn)
	if err != nil {
		t.Fatal(err)
	}
	var got Turn
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.RecordReport == nil || got.RecordReport.Repaired != 1 || got.RecordReport.Failed != 1 {
		t.Fatalf("reloaded report = %+v", got.RecordReport)
	}
}
