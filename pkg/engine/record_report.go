package engine

// maxRecordIssues bounds the per-turn issue list so a pathological reply cannot
// bloat the history file.
const maxRecordIssues = 5

// maxRecordIssueChars bounds one issue's error text.
const maxRecordIssueChars = 120

// RecordIssue is one control record that needed repair or was dropped.
type RecordIssue struct {
	Type   string `json:"type"`
	Repair string `json:"repair,omitempty"`
	Error  string `json:"error,omitempty"`
}

// RecordReport is a turn's control-record health summary.
type RecordReport struct {
	Total    int           `json:"total"`
	Repaired int           `json:"repaired"`
	Failed   int           `json:"failed"`
	Issues   []RecordIssue `json:"issues,omitempty"`
}

// recordReport summarises the parser's record health. It returns nil when every
// record arrived valid, so a clean turn adds nothing to the history.
func (o *TurnOrchestrator) recordReport() *RecordReport {
	if o.parser == nil {
		return nil
	}
	summary := o.parser.RepairReport()
	if summary.Repaired == 0 && summary.Failed == 0 {
		return nil
	}
	rep := &RecordReport{
		Total:    summary.Total,
		Repaired: summary.Repaired,
		Failed:   summary.Failed,
	}
	for _, rec := range o.parser.Records() {
		if len(rep.Issues) >= maxRecordIssues {
			break
		}
		switch {
		case rec.Err != nil:
			rep.Issues = append(rep.Issues, RecordIssue{
				Type:  rec.Type,
				Error: trimRunes(rec.Err.Error(), maxRecordIssueChars),
			})
		case rec.Repaired != "":
			rep.Issues = append(rep.Issues, RecordIssue{
				Type:   rec.Type,
				Repair: string(rec.Repaired),
			})
		}
	}
	return rep
}

// trimRunes truncates s to at most n runes.
func trimRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
