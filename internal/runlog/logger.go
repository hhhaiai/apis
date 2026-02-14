package runlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Entry struct {
	Timestamp      string `json:"timestamp"`
	RunID          string `json:"run_id,omitempty"`
	Path           string `json:"path"`
	Mode           string `json:"mode,omitempty"`
	ClientModel    string `json:"client_model,omitempty"`
	RequestedModel string `json:"requested_model,omitempty"`
	UpstreamModel  string `json:"upstream_model,omitempty"`
	Stream         bool   `json:"stream"`
	ToolCount      int    `json:"tool_count"`
	Status         int    `json:"status"`
	Error          string `json:"error,omitempty"`
	RecordText     string `json:"record_text,omitempty"`
	DurationMS     int64  `json:"duration_ms"`
}

type Logger interface {
	Log(entry Entry) error
}

type FileLogger struct {
	mu   sync.Mutex
	path string
}

func NewFileLogger(path string) (*FileLogger, error) {
	path = filepath.Clean(path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	return &FileLogger{path: path}, nil
}

func (l *FileLogger) Log(entry Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(raw); err != nil {
		return err
	}
	if _, err := f.Write([]byte("\n")); err != nil {
		return err
	}
	return nil
}
