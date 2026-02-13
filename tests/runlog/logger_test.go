package runlog_test

import (
	. "ccgateway/internal/runlog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileLoggerWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.log")

	l, err := NewFileLogger(path)
	if err != nil {
		t.Fatalf("new file logger: %v", err)
	}
	err = l.Log(Entry{
		RunID:      "run_test",
		Path:       "/v1/messages",
		Mode:       "chat",
		Stream:     false,
		ToolCount:  0,
		Status:     200,
		DurationMS: 12,
	})
	if err != nil {
		t.Fatalf("log entry failed: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(raw), `"run_id":"run_test"`) {
		t.Fatalf("expected run id in log file, got: %s", string(raw))
	}
}
