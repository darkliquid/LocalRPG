package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// TTSJob is one offline batch synthesis job. The clip cache is the source of
// truth for what is done; a job row tracks only what is in flight and what
// failed, so a restart resumes cleanly.
type TTSJob struct {
	ID           string
	GameID       string
	Provider     string
	Model        string
	Status       string
	InputURI     string
	RequestCount int
	Completed    int
	FailedKeys   []string
	CostMicros   int64
}

// rowScanner is the shared shape of *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

// UpsertTTSJob inserts or replaces a job record.
func (s *Store) UpsertTTSJob(job TTSJob) error {
	const query = `
	INSERT INTO tts_jobs (id, game_id, provider, model, status, input_uri, request_count, completed, failed_keys, cost_micros, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(id) DO UPDATE SET
		status = excluded.status,
		input_uri = excluded.input_uri,
		request_count = excluded.request_count,
		completed = excluded.completed,
		failed_keys = excluded.failed_keys,
		cost_micros = excluded.cost_micros,
		updated_at = CURRENT_TIMESTAMP`
	if _, err := s.db.Exec(query, job.ID, job.GameID, job.Provider, job.Model, job.Status,
		job.InputURI, job.RequestCount, job.Completed, strings.Join(job.FailedKeys, "\n"), job.CostMicros); err != nil {
		return fmt.Errorf("upsert tts job %q: %w", job.ID, err)
	}
	return nil
}

// GetTTSJob reads one job by id, returning nil when it does not exist.
func (s *Store) GetTTSJob(id string) (*TTSJob, error) {
	const query = `
	SELECT id, game_id, provider, model, status, input_uri, request_count, completed, failed_keys, cost_micros
	FROM tts_jobs WHERE id = ?`
	job, err := scanTTSJob(s.db.QueryRow(query, id))
	if err != nil {
		return nil, fmt.Errorf("get tts job %q: %w", id, err)
	}
	return job, nil
}

// ListTTSJobs lists a campaign's jobs, newest first.
func (s *Store) ListTTSJobs(gameID string) ([]TTSJob, error) {
	const query = `
	SELECT id, game_id, provider, model, status, input_uri, request_count, completed, failed_keys, cost_micros
	FROM tts_jobs WHERE game_id = ? ORDER BY created_at DESC, id DESC`
	rows, err := s.db.Query(query, gameID)
	if err != nil {
		return nil, fmt.Errorf("list tts jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]TTSJob, 0)
	for rows.Next() {
		job, err := scanTTSJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tts job: %w", err)
		}
		if job != nil {
			jobs = append(jobs, *job)
		}
	}
	return jobs, rows.Err()
}

// UpdateTTSJobStatus updates a job's status, progress, and failed keys.
func (s *Store) UpdateTTSJobStatus(id, status string, completed int, failedKeys []string) error {
	const query = `UPDATE tts_jobs SET status = ?, completed = ?, failed_keys = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	if _, err := s.db.Exec(query, status, completed, strings.Join(failedKeys, "\n"), id); err != nil {
		return fmt.Errorf("update tts job %q: %w", id, err)
	}
	return nil
}

// scanTTSJob reads one job row, returning nil when there is no row.
func scanTTSJob(scanner rowScanner) (*TTSJob, error) {
	var job TTSJob
	var failed string
	if err := scanner.Scan(&job.ID, &job.GameID, &job.Provider, &job.Model, &job.Status,
		&job.InputURI, &job.RequestCount, &job.Completed, &failed, &job.CostMicros); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	job.FailedKeys = splitKeys(failed)
	return &job, nil
}

// splitKeys splits a newline-separated key list, dropping empty entries.
func splitKeys(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
