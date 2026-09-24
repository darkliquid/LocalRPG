package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// TestWrappedDriverRecordsQuerySpan proves the game database is opened through
// the instrumented driver, so every statement becomes a span.
func TestWrappedDriverRecordsQuerySpan(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	db, err := OpenDB(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(context.Background(), "CREATE TABLE probe (id INTEGER)"); err != nil {
		t.Fatalf("exec: %v", err)
	}

	found := false
	for _, span := range recorder.Spans() {
		if strings.HasPrefix(span.Name(), "sql.") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an sql span, got %d spans", len(recorder.Spans()))
	}
}
