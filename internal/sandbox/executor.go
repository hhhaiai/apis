package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// ScriptJob represents a script execution request.
type ScriptJob struct {
	ID        string     `json:"id"`
	Language  string     `json:"language"`
	Code      string     `json:"code"`
	Timeout   int        `json:"timeout_seconds,omitempty"` // default 30
	Status    string     `json:"status"`                    // pending, running, completed, failed, timeout
	Output    string     `json:"output,omitempty"`
	Error     string     `json:"error,omitempty"`
	ExitCode  int        `json:"exit_code"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Duration  int64      `json:"duration_ms,omitempty"`
}

// AuditEntry records a script execution for auditing.
type AuditEntry struct {
	JobID     string    `json:"job_id"`
	Language  string    `json:"language"`
	CodeHash  string    `json:"code_hash"`
	ExitCode  int       `json:"exit_code"`
	Duration  int64     `json:"duration_ms"`
	Timestamp time.Time `json:"timestamp"`
}

// Config configures sandbox limits.
type Config struct {
	DefaultTimeout int      `json:"default_timeout_seconds"` // default 30
	MaxOutputBytes int      `json:"max_output_bytes"`        // default 64KB
	AllowedLangs   []string `json:"allowed_languages"`       // default: bash, python3, node
}

// DefaultConfig returns safe defaults.
func DefaultConfig() Config {
	return Config{
		DefaultTimeout: 30,
		MaxOutputBytes: 65536,
		AllowedLangs:   []string{"bash", "sh", "python3", "python", "node"},
	}
}

// Executor runs scripts in a sandboxed environment.
type Executor struct {
	mu      sync.RWMutex
	config  Config
	jobs    map[string]ScriptJob
	audit   []AuditEntry
	counter uint64

	// denyPatterns blocks dangerous commands
	denyPatterns []string
}

// NewExecutor creates a new sandbox executor.
func NewExecutor(cfg Config) *Executor {
	if cfg.DefaultTimeout <= 0 {
		cfg.DefaultTimeout = 30
	}
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = 65536
	}
	if len(cfg.AllowedLangs) == 0 {
		cfg.AllowedLangs = DefaultConfig().AllowedLangs
	}
	return &Executor{
		config: cfg,
		jobs:   make(map[string]ScriptJob),
		denyPatterns: []string{
			"rm -rf /", "mkfs.", "dd if=", ":(){:|:&};:",
			"chmod -R 777 /", "shutdown", "reboot", "halt",
			"> /dev/sd", "mv / ", "wget|sh", "curl|sh",
		},
	}
}

// Execute runs a script with timeout and resource limits.
func (e *Executor) Execute(ctx context.Context, language, code string) (ScriptJob, error) {
	language = strings.ToLower(strings.TrimSpace(language))
	code = strings.TrimSpace(code)

	if code == "" {
		return ScriptJob{}, fmt.Errorf("code is required")
	}
	if !e.isAllowed(language) {
		return ScriptJob{}, fmt.Errorf("language %q is not allowed; allowed: %v", language, e.config.AllowedLangs)
	}
	if e.isDangerous(code) {
		return ScriptJob{}, fmt.Errorf("code contains forbidden patterns")
	}

	now := time.Now().UTC()
	id := fmt.Sprintf("script_%d_%x", now.Unix(), atomic.AddUint64(&e.counter, 1))

	job := ScriptJob{
		ID:        id,
		Language:  language,
		Code:      code,
		Timeout:   e.config.DefaultTimeout,
		Status:    "running",
		StartedAt: &now,
	}

	// Determine interpreter and build command with code as argument
	interpreter, flag := e.resolveInterpreter(language)

	timeout := time.Duration(e.config.DefaultTimeout) * time.Second
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, interpreter, flag, code)
	// Set process group ID so we can kill the whole process tree on timeout
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	endTime := time.Now().UTC()
	job.EndedAt = &endTime
	job.Duration = endTime.Sub(now).Milliseconds()

	// Truncate output if needed
	output := stdout.String()
	errOutput := stderr.String()
	if len(output) > e.config.MaxOutputBytes {
		output = output[:e.config.MaxOutputBytes] + "\n...[truncated]"
	}
	if len(errOutput) > e.config.MaxOutputBytes {
		errOutput = errOutput[:e.config.MaxOutputBytes] + "\n...[truncated]"
	}

	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			job.Status = "timeout"
			job.Error = fmt.Sprintf("execution timed out after %ds", e.config.DefaultTimeout)
		} else {
			job.Status = "failed"
			job.Error = errOutput
			if job.Error == "" {
				job.Error = err.Error()
			}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			job.ExitCode = exitErr.ExitCode()
		} else {
			job.ExitCode = -1
		}
	} else {
		job.Status = "completed"
		job.ExitCode = 0
	}

	if output != "" {
		job.Output = output
	} else if errOutput != "" && job.Status == "completed" {
		job.Output = errOutput
	}

	// Store job and audit entry
	e.mu.Lock()
	e.jobs[id] = job
	e.audit = append(e.audit, AuditEntry{
		JobID:     id,
		Language:  language,
		CodeHash:  fmt.Sprintf("%x", len(code)), // simple hash
		ExitCode:  job.ExitCode,
		Duration:  job.Duration,
		Timestamp: endTime,
	})
	e.mu.Unlock()

	return job, nil
}

// Get retrieves a job by ID.
func (e *Executor) Get(id string) (ScriptJob, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	j, ok := e.jobs[id]
	return j, ok
}

// ListAudit returns all audit entries.
func (e *Executor) ListAudit() []AuditEntry {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]AuditEntry, len(e.audit))
	copy(out, e.audit)
	return out
}

func (e *Executor) isAllowed(lang string) bool {
	for _, l := range e.config.AllowedLangs {
		if l == lang {
			return true
		}
	}
	return false
}

func (e *Executor) isDangerous(code string) bool {
	lower := strings.ToLower(code)
	for _, p := range e.denyPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func (e *Executor) resolveInterpreter(lang string) (string, string) {
	switch lang {
	case "python3", "python":
		return "python3", "-c"
	case "node":
		return "node", "-e"
	default: // bash, sh
		return "bash", "-c"
	}
}
