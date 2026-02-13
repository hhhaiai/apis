package hooks

import (
	"context"
	"fmt"
	"sync"
)

// HookPoint identifies where in the request lifecycle a hook fires.
type HookPoint string

const (
	PreRequest   HookPoint = "pre_request"
	PostResponse HookPoint = "post_response"
	PreToolCall  HookPoint = "pre_tool_call"
	PostToolCall HookPoint = "post_tool_call"
	OnError      HookPoint = "on_error"
)

// HookHandler is a function that processes hook data.
// It receives context data and returns modified data.
type HookHandler func(ctx context.Context, data map[string]any) (map[string]any, error)

// Hook represents a registered hook.
type Hook struct {
	Name     string    `json:"name"`
	Point    HookPoint `json:"point"`
	Priority int       `json:"priority"` // higher = executed first
	handler  HookHandler
}

// Registry manages hook registration and execution.
type Registry struct {
	mu    sync.RWMutex
	hooks map[HookPoint][]Hook
}

// NewRegistry creates a new hook registry.
func NewRegistry() *Registry {
	return &Registry{
		hooks: make(map[HookPoint][]Hook),
	}
}

// Register adds a hook handler for a specific hook point.
func (r *Registry) Register(name string, point HookPoint, priority int, handler HookHandler) error {
	if name == "" {
		return fmt.Errorf("hook name is required")
	}
	if handler == nil {
		return fmt.Errorf("hook handler is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	h := Hook{
		Name:     name,
		Point:    point,
		Priority: priority,
		handler:  handler,
	}
	r.hooks[point] = append(r.hooks[point], h)

	// Keep sorted by priority (higher first)
	hooks := r.hooks[point]
	for i := len(hooks) - 1; i > 0; i-- {
		if hooks[i].Priority > hooks[i-1].Priority {
			hooks[i], hooks[i-1] = hooks[i-1], hooks[i]
		}
	}

	return nil
}

// Unregister removes a hook by name and point.
func (r *Registry) Unregister(name string, point HookPoint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	hooks := r.hooks[point]
	filtered := make([]Hook, 0, len(hooks))
	for _, h := range hooks {
		if h.Name != name {
			filtered = append(filtered, h)
		}
	}
	r.hooks[point] = filtered
}

// Fire executes all hooks registered at a given point.
// Hooks are executed in priority order. Each hook receives the output of the previous.
// If any hook returns an error, execution stops and the error is returned.
func (r *Registry) Fire(ctx context.Context, point HookPoint, data map[string]any) (map[string]any, error) {
	r.mu.RLock()
	hooks := make([]Hook, len(r.hooks[point]))
	copy(hooks, r.hooks[point])
	r.mu.RUnlock()

	if len(hooks) == 0 {
		return data, nil
	}

	current := data
	for _, h := range hooks {
		result, err := h.handler(ctx, current)
		if err != nil {
			return current, fmt.Errorf("hook %q failed: %w", h.Name, err)
		}
		if result != nil {
			current = result
		}
	}
	return current, nil
}

// List returns all registered hooks.
func (r *Registry) List() map[HookPoint][]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[HookPoint][]string)
	for point, hooks := range r.hooks {
		names := make([]string, 0, len(hooks))
		for _, h := range hooks {
			names = append(names, h.Name)
		}
		out[point] = names
	}
	return out
}

// Count returns total number of registered hooks.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	total := 0
	for _, hooks := range r.hooks {
		total += len(hooks)
	}
	return total
}
