package toolruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

type Call struct {
	ID    string
	Name  string
	Input map[string]any
}

type Result struct {
	Content any
	IsError bool
}

var ErrToolNotImplemented = errors.New("tool is not implemented")

type Executor interface {
	Execute(ctx context.Context, call Call) (Result, error)
}

type Handler func(ctx context.Context, call Call) (Result, error)

type Registry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

func NewRegistry() *Registry {
	return &Registry{
		handlers: map[string]Handler{},
	}
}

func NewDefaultExecutor() *Registry {
	r := NewRegistry()
	r.Register("get_weather", handleGetWeather)
	r.Register("web_search", handleWebSearchReal)
	r.Register("image_recognition", handleImageRecognition)
	r.Register("image_analyze", handleImageRecognition)
	r.Register("image_search", handleImageRecognition)
	r.Register("file_read", handleFileRead)
	r.Register("file_write", handleFileWrite)
	r.Register("file_list", handleFileList)
	return r
}

func (r *Registry) Register(name string, handler Handler) {
	name = normalizeName(name)
	if name == "" || handler == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[name] = handler
}

func (r *Registry) Execute(ctx context.Context, call Call) (Result, error) {
	if r == nil {
		return Result{}, fmt.Errorf("tool executor is not configured")
	}
	name := normalizeName(call.Name)
	if name == "" {
		return Result{}, fmt.Errorf("tool name is required")
	}
	r.mu.RLock()
	handler := r.handlers[name]
	r.mu.RUnlock()
	if handler == nil {
		return Result{}, fmt.Errorf("%w: %q", ErrToolNotImplemented, call.Name)
	}
	if call.Input == nil {
		call.Input = map[string]any{}
	}
	call.Name = name
	return handler(ctx, call)
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func handleGetWeather(_ context.Context, call Call) (Result, error) {
	city := firstString(call.Input, "city", "location", "query", "q")
	if city == "" {
		city = "unknown"
	}
	return Result{
		Content: map[string]any{
			"tool":          call.Name,
			"city":          city,
			"condition":     "sunny",
			"temperature_c": 26,
		},
	}, nil
}

func handleWebSearch(_ context.Context, call Call) (Result, error) {
	query := firstString(call.Input, "query", "q", "keyword")
	if query == "" {
		return Result{}, fmt.Errorf("web_search requires query")
	}
	return Result{
		Content: map[string]any{
			"tool":  call.Name,
			"query": query,
			"results": []map[string]any{
				{
					"title": "Search result 1",
					"url":   "https://example.com/result-1",
					"snip":  "placeholder search result",
				},
				{
					"title": "Search result 2",
					"url":   "https://example.com/result-2",
					"snip":  "placeholder search result",
				},
			},
		},
	}, nil
}

func handleImageRecognition(_ context.Context, call Call) (Result, error) {
	imageURL := firstString(call.Input, "image_url", "url")
	if imageURL == "" {
		return Result{}, fmt.Errorf("image tool requires image_url")
	}
	return Result{
		Content: map[string]any{
			"tool":      call.Name,
			"image_url": imageURL,
			"summary":   "image analysis placeholder",
		},
	}, nil
}
