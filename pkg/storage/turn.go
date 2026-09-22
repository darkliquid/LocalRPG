package storage

import (
	"fmt"
	"strings"
	"time"
)

// TurnEntityRef records how one entity was involved in a turn. Outcome is the
// turn's system-reported outcome, copied for single-table per-entity queries.
type TurnEntityRef struct {
	EntityID string
	Mention  string
	Outcome  string
}

// TurnRecord is the row shape of one timeline entry. Timestamps are stored as
// RFC3339 text so the value does not depend on driver time handling.
type TurnRecord struct {
	Number    int
	Timestamp time.Time
	Mode      string
	Input     string
	Narration string
	Location  string
	Outcome   string
	RollJSON  string
	Entities  []TurnEntityRef
}

// SaveTurn upserts one turn and replaces its entity links in a single transaction.
func (s *Store) SaveTurn(rec TurnRecord) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	const upsert = `
	INSERT INTO turns (number, timestamp, mode, input, narration, roll_json, location, outcome)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(number) DO UPDATE SET
		timestamp = excluded.timestamp,
		mode = excluded.mode,
		input = excluded.input,
		narration = excluded.narration,
		roll_json = excluded.roll_json,
		location = excluded.location,
		outcome = excluded.outcome
	`
	stamp := rec.Timestamp.UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(upsert, rec.Number, stamp, rec.Mode, rec.Input, rec.Narration,
		emptyToNull(rec.RollJSON), emptyToNull(rec.Location), emptyToNull(rec.Outcome)); err != nil {
		return fmt.Errorf("upsert turn %d: %w", rec.Number, err)
	}

	if _, err := tx.Exec(`DELETE FROM turn_entities WHERE turn_number = ?`, rec.Number); err != nil {
		return fmt.Errorf("clear turn %d links: %w", rec.Number, err)
	}
	const link = `INSERT OR IGNORE INTO turn_entities (turn_number, entity_id, mention, outcome) VALUES (?, ?, ?, ?)`
	for _, ref := range rec.Entities {
		if ref.EntityID == "" || ref.Mention == "" {
			continue
		}
		if _, err := tx.Exec(link, rec.Number, ref.EntityID, ref.Mention, emptyToNull(ref.Outcome)); err != nil {
			return fmt.Errorf("link entity %q to turn %d: %w", ref.EntityID, rec.Number, err)
		}
	}

	return tx.Commit()
}

// GetTurn returns one turn with its entity links.
func (s *Store) GetTurn(number int) (*TurnRecord, error) {
	const query = `
	SELECT number, timestamp, mode, input, narration,
	       COALESCE(roll_json, ''), COALESCE(location, ''), COALESCE(outcome, '')
	FROM turns WHERE number = ?`

	var rec TurnRecord
	var stamp string
	if err := s.db.QueryRow(query, number).Scan(&rec.Number, &stamp, &rec.Mode, &rec.Input,
		&rec.Narration, &rec.RollJSON, &rec.Location, &rec.Outcome); err != nil {
		return nil, fmt.Errorf("get turn %d: %w", number, err)
	}

	parsed, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return nil, fmt.Errorf("parse turn %d timestamp: %w", number, err)
	}
	rec.Timestamp = parsed

	refs, err := s.ListEntitiesForTurn(number)
	if err != nil {
		return nil, err
	}
	rec.Entities = refs
	return &rec, nil
}

// ListEntitiesForTurn returns the entity links recorded for a turn.
func (s *Store) ListEntitiesForTurn(number int) ([]TurnEntityRef, error) {
	const query = `SELECT entity_id, mention, COALESCE(outcome, '') FROM turn_entities WHERE turn_number = ? ORDER BY entity_id, mention`
	rows, err := s.db.Query(query, number)
	if err != nil {
		return nil, fmt.Errorf("list turn %d entities: %w", number, err)
	}
	defer rows.Close()

	refs := make([]TurnEntityRef, 0)
	for rows.Next() {
		var ref TurnEntityRef
		if err := rows.Scan(&ref.EntityID, &ref.Mention, &ref.Outcome); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return refs, nil
}

// ListTurnEntitiesByOutcome returns the turns where an entity's link carries a
// given system-reported outcome, which is what makes "every check this character
// failed" a single-table query.
func (s *Store) ListTurnEntitiesByOutcome(entityID, outcome string) ([]int, error) {
	const query = `
	SELECT DISTINCT turn_number FROM turn_entities
	WHERE entity_id = ? AND outcome = ?
	ORDER BY turn_number`

	rows, err := s.db.Query(query, entityID, outcome)
	if err != nil {
		return nil, fmt.Errorf("list turns for entity %q with outcome %q: %w", entityID, outcome, err)
	}
	defer rows.Close()

	numbers := make([]int, 0)
	for rows.Next() {
		var number int
		if err := rows.Scan(&number); err != nil {
			return nil, err
		}
		numbers = append(numbers, number)
	}
	return numbers, rows.Err()
}

func emptyToNull(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

// ListTurns returns timeline entries in ascending order. A limit of 0 returns all.
func (s *Store) ListTurns(limit, offset int) ([]TurnRecord, error) {
	query := `
	SELECT number, timestamp, mode, input, narration,
	       COALESCE(roll_json, ''), COALESCE(location, ''), COALESCE(outcome, '')
	FROM turns ORDER BY number`
	args := make([]interface{}, 0, 2)
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list turns: %w", err)
	}
	defer rows.Close()

	turns := make([]TurnRecord, 0)
	for rows.Next() {
		var rec TurnRecord
		var stamp string
		if err := rows.Scan(&rec.Number, &stamp, &rec.Mode, &rec.Input, &rec.Narration,
			&rec.RollJSON, &rec.Location, &rec.Outcome); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			return nil, fmt.Errorf("parse turn %d timestamp: %w", rec.Number, err)
		}
		rec.Timestamp = parsed
		turns = append(turns, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return turns, nil
}

// ListTurnsForEntity returns the ascending turn numbers an entity was involved in.
func (s *Store) ListTurnsForEntity(entityID string) ([]int, error) {
	const query = `SELECT DISTINCT turn_number FROM turn_entities WHERE entity_id = ? ORDER BY turn_number`
	rows, err := s.db.Query(query, entityID)
	if err != nil {
		return nil, fmt.Errorf("list turns for entity %q: %w", entityID, err)
	}
	defer rows.Close()

	numbers := make([]int, 0)
	for rows.Next() {
		var number int
		if err := rows.Scan(&number); err != nil {
			return nil, err
		}
		numbers = append(numbers, number)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return numbers, nil
}

// MaxTurnNumber returns the highest indexed turn number, or 0 when empty.
func (s *Store) MaxTurnNumber() (int, error) {
	var max int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(number), 0) FROM turns`).Scan(&max); err != nil {
		return 0, fmt.Errorf("read max turn number: %w", err)
	}
	return max, nil
}

// CountTurns returns the number of indexed turns.
func (s *Store) CountTurns() (int, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM turns`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count turns: %w", err)
	}
	return count, nil
}

// DeleteTurnsFrom discards a turn and everything after it, including their entity
// links. It backs /undo, where the JSONL log and the index must agree.
func (s *Store) DeleteTurnsFrom(number int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM turn_entities WHERE turn_number >= ?`, number); err != nil {
		return fmt.Errorf("delete turn links from %d: %w", number, err)
	}
	if _, err := tx.Exec(`DELETE FROM turns WHERE number >= ?`, number); err != nil {
		return fmt.Errorf("delete turns from %d: %w", number, err)
	}

	return tx.Commit()
}

// turnColumns is the projection every recall query shares, in the order scanTurn
// expects.
const turnColumns = "turns.number, turns.timestamp, turns.mode, turns.input, turns.narration, COALESCE(turns.roll_json, ''), COALESCE(turns.location, ''), COALESCE(turns.outcome, '')"

// scanTurn reads one projected turn. Entity links are loaded separately, because a
// recall excerpt never needs them.
func scanTurn(scanner interface{ Scan(...interface{}) error }) (TurnRecord, error) {
	var record TurnRecord
	var timestamp string
	if err := scanner.Scan(
		&record.Number,
		&timestamp,
		&record.Mode,
		&record.Input,
		&record.Narration,
		&record.RollJSON,
		&record.Location,
		&record.Outcome,
	); err != nil {
		return TurnRecord{}, err
	}
	if parsed, err := time.Parse(time.RFC3339, timestamp); err == nil {
		record.Timestamp = parsed
	}
	return record, nil
}

// TurnsAtLocation returns turns recorded at a location before the given turn,
// oldest first, so a scene can be reminded of what happened where it stands.
func (s *Store) TurnsAtLocation(locationID string, beforeTurn, limit int) ([]TurnRecord, error) {
	if locationID == "" || limit <= 0 {
		return nil, nil
	}

	query := `SELECT ` + turnColumns + ` FROM turns WHERE location = ? AND number < ? ORDER BY number DESC LIMIT ?`
	rows, err := s.db.Query(query, locationID, beforeTurn, limit)
	if err != nil {
		return nil, fmt.Errorf("turns at location %q: %w", locationID, err)
	}
	defer rows.Close()

	turnRecords := make([]TurnRecord, 0, limit)
	for rows.Next() {
		record, err := scanTurn(rows)
		if err != nil {
			return nil, fmt.Errorf("scan turn: %w", err)
		}
		turnRecords = append(turnRecords, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("turns at location %q: %w", locationID, err)
	}

	// The query walks backwards to take the newest, and the caller renders forwards.
	for i, j := 0, len(turnRecords)-1; i < j; i, j = i+1, j-1 {
		turnRecords[i], turnRecords[j] = turnRecords[j], turnRecords[i]
	}
	return turnRecords, nil
}

// TurnsMentioningEntities returns turns that mention any of the given entities,
// ranked by how many of them they mention and then by recency. This is what
// recovers the turn where a promise was made, which no fixed window can do.
func (s *Store) TurnsMentioningEntities(entityIDs []string, excludeFromTurn, limit int) ([]TurnRecord, error) {
	if len(entityIDs) == 0 || limit <= 0 {
		return nil, nil
	}

	placeholders := make([]string, 0, len(entityIDs))
	args := make([]interface{}, 0, len(entityIDs)+2)
	for _, id := range entityIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	args = append(args, excludeFromTurn, limit)

	query := `SELECT ` + turnColumns + `, COUNT(*) AS hits
		FROM turns JOIN turn_entities ON turn_entities.turn_number = turns.number
		WHERE turn_entities.entity_id IN (` + strings.Join(placeholders, ",") + `) AND turns.number < ?
		GROUP BY turns.number
		ORDER BY hits DESC, turns.number DESC
		LIMIT ?`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("turns mentioning entities: %w", err)
	}
	defer rows.Close()

	turnRecords := make([]TurnRecord, 0, limit)
	for rows.Next() {
		var record TurnRecord
		var timestamp string
		var hits int
		if err := rows.Scan(
			&record.Number, &timestamp, &record.Mode, &record.Input, &record.Narration,
			&record.RollJSON, &record.Location, &record.Outcome, &hits,
		); err != nil {
			return nil, fmt.Errorf("scan turn: %w", err)
		}
		if parsed, err := time.Parse(time.RFC3339, timestamp); err == nil {
			record.Timestamp = parsed
		}
		turnRecords = append(turnRecords, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("turns mentioning entities: %w", err)
	}
	return turnRecords, nil
}

// EntitiesInTurns returns the distinct entities mentioned by the given turns,
// which is how "who is in play" is known without a second index.
func (s *Store) EntitiesInTurns(turnNumbers []int) ([]string, error) {
	if len(turnNumbers) == 0 {
		return nil, nil
	}

	placeholders := make([]string, 0, len(turnNumbers))
	args := make([]interface{}, 0, len(turnNumbers))
	for _, number := range turnNumbers {
		placeholders = append(placeholders, "?")
		args = append(args, number)
	}

	query := `SELECT DISTINCT entity_id FROM turn_entities WHERE turn_number IN (` +
		strings.Join(placeholders, ",") + `) ORDER BY entity_id`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("entities in turns: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan entity id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("entities in turns: %w", err)
	}
	return ids, nil
}
