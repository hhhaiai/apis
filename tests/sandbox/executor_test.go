package sandbox_test

import (
	. "ccgateway/internal/sandbox"
	"context"
	"strings"
	"testing"
)

func TestExecutor_BashEcho(t *testing.T) {
	e := NewExecutor(DefaultConfig())
	job, err := e.Execute(context.Background(), "bash", "echo hello world")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "completed" {
		t.Fatalf("expected completed, got %s: %s", job.Status, job.Error)
	}
	if !strings.Contains(job.Output, "hello world") {
		t.Fatalf("output should contain 'hello world', got: %s", job.Output)
	}
	if job.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", job.ExitCode)
	}
}

func TestExecutor_FailedScript(t *testing.T) {
	e := NewExecutor(DefaultConfig())
	job, err := e.Execute(context.Background(), "bash", "exit 42")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "failed" {
		t.Fatalf("expected failed, got %s", job.Status)
	}
	if job.ExitCode != 42 {
		t.Fatalf("expected exit code 42, got %d", job.ExitCode)
	}
}

func TestExecutor_EmptyCode(t *testing.T) {
	e := NewExecutor(DefaultConfig())
	_, err := e.Execute(context.Background(), "bash", "")
	if err == nil {
		t.Fatal("expected error for empty code")
	}
}

func TestExecutor_DisallowedLang(t *testing.T) {
	e := NewExecutor(DefaultConfig())
	_, err := e.Execute(context.Background(), "ruby", "puts 'hello'")
	if err == nil {
		t.Fatal("expected error for disallowed language")
	}
}

func TestExecutor_DangerousCode(t *testing.T) {
	e := NewExecutor(DefaultConfig())
	_, err := e.Execute(context.Background(), "bash", "rm -rf /")
	if err == nil {
		t.Fatal("expected error for dangerous code")
	}
}

func TestExecutor_GetAndAudit(t *testing.T) {
	e := NewExecutor(DefaultConfig())
	job, _ := e.Execute(context.Background(), "bash", "echo test")
	got, ok := e.Get(job.ID)
	if !ok {
		t.Fatal("job not found")
	}
	if got.ID != job.ID {
		t.Fatalf("expected %s, got %s", job.ID, got.ID)
	}
	audit := e.ListAudit()
	if len(audit) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(audit))
	}
}

func TestExecutor_Timeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DefaultTimeout = 1
	e := NewExecutor(cfg)
	job, err := e.Execute(context.Background(), "bash", "sleep 10")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "timeout" {
		t.Fatalf("expected timeout, got %s", job.Status)
	}
}
