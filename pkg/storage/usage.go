package storage

import (
	"database/sql"
	"time"
)

// UsageRecord is one provider call's consumption and cost.
type UsageRecord struct {
	TurnNumber   int       `json:"turn_number"`
	Role         string    `json:"role"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model,omitempty"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	Characters   int       `json:"characters,omitempty"`
	Requests     int       `json:"requests,omitempty"`
	Estimated    bool      `json:"estimated,omitempty"`
	CostMicros   int64     `json:"cost_micros,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// UsageSummary is what a spend view needs without shipping every row.
type UsageSummary struct {
	TotalCostMicros int64            `json:"total_cost_micros"`
	ByProvider      map[string]int64 `json:"by_provider"`
	ByRole          map[string]int64 `json:"by_role"`
	Rows            int              `json:"rows"`
}

// SaveUsage records one provider call's consumption and cost.
func (s *Store) SaveUsage(rec UsageRecord) error {
	const query = `
	INSERT INTO usage_records (turn_number, role, provider, model, input_tokens, output_tokens, characters, requests, estimated, cost_micros)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	estimated := 0
	if rec.Estimated {
		estimated = 1
	}
	_, err := s.db.Exec(query, rec.TurnNumber, rec.Role, rec.Provider, rec.Model,
		rec.InputTokens, rec.OutputTokens, rec.Characters, rec.Requests, estimated, rec.CostMicros)
	return err
}

// UsageByTurn returns every call recorded for one turn, in insertion order.
func (s *Store) UsageByTurn(turn int) ([]UsageRecord, error) {
	const query = `
	SELECT turn_number, role, provider, model, input_tokens, output_tokens, characters, requests, estimated, cost_micros
	FROM usage_records WHERE turn_number = ? ORDER BY id`
	rows, err := s.db.Query(query, turn)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsage(rows)
}

// UsageSummary totals the campaign's spend and breaks it down by provider and role.
func (s *Store) UsageSummary() (UsageSummary, error) {
	summary := UsageSummary{ByProvider: map[string]int64{}, ByRole: map[string]int64{}}
	rows, err := s.db.Query(`SELECT provider, role, cost_micros FROM usage_records`)
	if err != nil {
		return summary, err
	}
	defer rows.Close()
	for rows.Next() {
		var provider, role string
		var cost int64
		if err := rows.Scan(&provider, &role, &cost); err != nil {
			return summary, err
		}
		summary.Rows++
		summary.TotalCostMicros += cost
		summary.ByProvider[provider] += cost
		summary.ByRole[role] += cost
	}
	return summary, rows.Err()
}

// UsageTotal is convenience for the global view.
func (s *Store) UsageTotal() (int64, error) {
	var total int64
	err := s.db.QueryRow(`SELECT COALESCE(SUM(cost_micros), 0) FROM usage_records`).Scan(&total)
	return total, err
}

func scanUsage(rows *sql.Rows) ([]UsageRecord, error) {
	out := make([]UsageRecord, 0)
	for rows.Next() {
		var rec UsageRecord
		var estimated int
		if err := rows.Scan(&rec.TurnNumber, &rec.Role, &rec.Provider, &rec.Model,
			&rec.InputTokens, &rec.OutputTokens, &rec.Characters, &rec.Requests, &estimated, &rec.CostMicros); err != nil {
			return nil, err
		}
		rec.Estimated = estimated != 0
		out = append(out, rec)
	}
	return out, rows.Err()
}
