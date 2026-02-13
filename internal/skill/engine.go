package skill

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Skill represents a registered skill with its template and parameters.
type Skill struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	Parameters  []Param   `json:"parameters,omitempty"`
	Template    string    `json:"template"`
	CreatedAt   time.Time `json:"created_at"`
}

// Param defines a skill parameter.
type Param struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
}

// Engine manages skill registration and execution.
type Engine struct {
	mu     sync.RWMutex
	skills map[string]Skill
}

// NewEngine creates a new skill engine.
func NewEngine() *Engine {
	return &Engine{
		skills: make(map[string]Skill),
	}
}

// Register adds a new skill or updates an existing one.
func (e *Engine) Register(s Skill) error {
	name := strings.TrimSpace(s.Name)
	if name == "" {
		return fmt.Errorf("skill name is required")
	}
	if strings.TrimSpace(s.Template) == "" {
		return fmt.Errorf("skill template is required")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	s.Name = name
	if s.Version == "" {
		s.Version = "1.0"
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	e.skills[name] = s
	return nil
}

// Get retrieves a skill by name.
func (e *Engine) Get(name string) (Skill, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.skills[strings.TrimSpace(name)]
	return s, ok
}

// List returns all registered skills.
func (e *Engine) List() []Skill {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]Skill, 0, len(e.skills))
	for _, s := range e.skills {
		out = append(out, s)
	}
	return out
}

// Delete removes a skill by name.
func (e *Engine) Delete(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("skill name is required")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.skills[name]; !ok {
		return fmt.Errorf("skill %q not found", name)
	}
	delete(e.skills, name)
	return nil
}

// Execute renders a skill template with the given parameters.
func (e *Engine) Execute(name string, params map[string]any) (string, error) {
	name = strings.TrimSpace(name)
	e.mu.RLock()
	s, ok := e.skills[name]
	e.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("skill %q not found", name)
	}

	// Validate required parameters
	for _, p := range s.Parameters {
		if p.Required {
			if _, ok := params[p.Name]; !ok {
				if p.Default != "" {
					params[p.Name] = p.Default
				} else {
					return "", fmt.Errorf("required parameter %q is missing", p.Name)
				}
			}
		}
	}

	// Apply defaults for optional missing params
	for _, p := range s.Parameters {
		if _, ok := params[p.Name]; !ok && p.Default != "" {
			params[p.Name] = p.Default
		}
	}

	// Simple template rendering: replace {{param_name}} with value
	result := s.Template
	for k, v := range params {
		placeholder := "{{" + k + "}}"
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", v))
	}

	return result, nil
}
