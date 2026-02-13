package toolruntime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// --- Real Tool Implementations ---

// handleWebSearchReal performs an HTTP-based web search using a configurable search API.
func handleWebSearchReal(_ context.Context, call Call) (Result, error) {
	query := firstString(call.Input, "query", "q", "keyword")
	if query == "" {
		return Result{}, fmt.Errorf("web_search requires query")
	}

	apiURL := firstString(call.Input, "api_url")
	if apiURL == "" {
		apiURL = os.Getenv("SEARCH_API_URL")
	}
	if apiURL == "" {
		// Fallback to DuckDuckGo instant answer API (no API key required)
		apiURL = "https://api.duckduckgo.com/?format=json&q=" + query
	} else {
		apiURL = strings.ReplaceAll(apiURL, "{query}", query)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return Result{IsError: true, Content: fmt.Sprintf("search request failed: %v", err)}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32768))
	if err != nil {
		return Result{IsError: true, Content: fmt.Sprintf("failed to read search response: %v", err)}, nil
	}

	return Result{
		Content: map[string]any{
			"tool":        call.Name,
			"query":       query,
			"status_code": resp.StatusCode,
			"body":        string(body),
		},
	}, nil
}

// handleFileRead reads a file from the filesystem (restricted to allowed paths).
func handleFileRead(_ context.Context, call Call) (Result, error) {
	path := firstString(call.Input, "path", "file", "filename")
	if path == "" {
		return Result{}, fmt.Errorf("file_read requires path")
	}

	path = filepath.Clean(path)
	if err := validatePath(path); err != nil {
		return Result{IsError: true, Content: err.Error()}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Result{IsError: true, Content: fmt.Sprintf("failed to read file: %v", err)}, nil
	}

	// Limit output to 64KB
	content := string(data)
	if len(content) > 65536 {
		content = content[:65536] + "\n...[truncated]"
	}

	return Result{
		Content: map[string]any{
			"tool":    call.Name,
			"path":    path,
			"content": content,
			"size":    len(data),
		},
	}, nil
}

// handleFileWrite writes content to a file (restricted to allowed paths).
func handleFileWrite(_ context.Context, call Call) (Result, error) {
	path := firstString(call.Input, "path", "file", "filename")
	content := firstString(call.Input, "content", "data", "text")
	if path == "" {
		return Result{}, fmt.Errorf("file_write requires path")
	}
	if content == "" {
		return Result{}, fmt.Errorf("file_write requires content")
	}

	path = filepath.Clean(path)
	if err := validatePath(path); err != nil {
		return Result{IsError: true, Content: err.Error()}, nil
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return Result{IsError: true, Content: fmt.Sprintf("failed to create directory: %v", err)}, nil
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return Result{IsError: true, Content: fmt.Sprintf("failed to write file: %v", err)}, nil
	}

	return Result{
		Content: map[string]any{
			"tool":  call.Name,
			"path":  path,
			"bytes": len(content),
		},
	}, nil
}

// handleFileList lists files in a directory.
func handleFileList(_ context.Context, call Call) (Result, error) {
	path := firstString(call.Input, "path", "directory", "dir")
	if path == "" {
		return Result{}, fmt.Errorf("file_list requires path")
	}

	path = filepath.Clean(path)
	if err := validatePath(path); err != nil {
		return Result{IsError: true, Content: err.Error()}, nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return Result{IsError: true, Content: fmt.Sprintf("failed to list directory: %v", err)}, nil
	}

	files := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		info, _ := e.Info()
		entry := map[string]any{
			"name":   e.Name(),
			"is_dir": e.IsDir(),
		}
		if info != nil {
			entry["size"] = info.Size()
			entry["modified"] = info.ModTime().Format(time.RFC3339)
		}
		files = append(files, entry)
	}

	return Result{
		Content: map[string]any{
			"tool":  call.Name,
			"path":  path,
			"count": len(files),
			"files": files,
		},
	}, nil
}

// validatePath ensures path is safe (not a system directory).
func validatePath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path")
	}
	// Block system-critical paths
	blocked := []string{"/etc", "/usr", "/bin", "/sbin", "/boot", "/dev", "/proc", "/sys", "/var/run"}
	for _, b := range blocked {
		if strings.HasPrefix(abs, b) {
			return fmt.Errorf("access to %q is restricted", b)
		}
	}
	return nil
}
