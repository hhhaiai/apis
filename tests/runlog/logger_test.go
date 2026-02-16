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
		RecordText: "generated output summary",
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
	if !strings.Contains(string(raw), `"record_text":"generated output summary"`) {
		t.Fatalf("expected record_text in log file, got: %s", string(raw))
	}
}

func TestFileLoggerWritesDecodeDiagnosticsFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.log")

	l, err := NewFileLogger(path)
	if err != nil {
		t.Fatalf("new file logger: %v", err)
	}
	err = l.Log(Entry{
		Path:        "/v1/cc/todos",
		Reason:      "unsupported_fields",
		Mode:        "request_decode",
		Stream:      false,
		ToolCount:   0,
		Status:      400,
		Error:       `json: unknown field "x_extra"`,
		RecordText:  "request.decode_failed",
		Unsupported: []string{"x_extra"},
		RequestBody: `{"title":"x","x_extra":1}`,
		CurlCommand: "curl -X 'POST' 'http://127.0.0.1:8080/v1/cc/todos'",
	})
	if err != nil {
		t.Fatalf("log entry failed: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"reason":"unsupported_fields"`) {
		t.Fatalf("expected reason in log file, got: %s", text)
	}
	if !strings.Contains(text, `"unsupported_fields":["x_extra"]`) {
		t.Fatalf("expected unsupported_fields in log file, got: %s", text)
	}
	if !strings.Contains(text, `"request_body":"{\"title\":\"x\",\"x_extra\":1}"`) {
		t.Fatalf("expected request_body in log file, got: %s", text)
	}
	if !strings.Contains(text, `"curl_command":"curl -X 'POST' 'http://127.0.0.1:8080/v1/cc/todos'"`) {
		t.Fatalf("expected curl_command in log file, got: %s", text)
	}
}
