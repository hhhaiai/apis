package toolcatalog

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

type Status string

const (
	StatusSupported    Status = "supported"
	StatusExperimental Status = "experimental"
	StatusUnsupported  Status = "unsupported"
)

type ToolSpec struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Notes  string `json:"notes,omitempty"`
}

type Catalog struct {
	mu    sync.RWMutex
	tools map[string]ToolSpec
}

func NewCatalog(tools []ToolSpec) *Catalog {
	c := &Catalog{tools: map[string]ToolSpec{}}
	c.Replace(tools)
	return c
}

func NewFromEnv() (*Catalog, error) {
	raw := strings.TrimSpace(os.Getenv("TOOL_CATALOG_JSON"))
	if raw == "" {
		return NewCatalog(nil), nil
	}
	var tools []ToolSpec
	if err := json.Unmarshal([]byte(raw), &tools); err != nil {
		return nil, fmt.Errorf("invalid TOOL_CATALOG_JSON: %w", err)
	}
	return NewCatalog(tools), nil
}

func (c *Catalog) Replace(tools []ToolSpec) {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := map[string]ToolSpec{}
	for _, t := range tools {
		name := normalizeToolName(t.Name)
		if name == "" {
			continue
		}
		st := normalizeStatus(t.Status)
		next[name] = ToolSpec{
			Name:   name,
			Status: st,
			Notes:  strings.TrimSpace(t.Notes),
		}
	}
	c.tools = next
}

func (c *Catalog) Snapshot() []ToolSpec {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]ToolSpec, 0, len(c.tools))
	for _, spec := range c.tools {
		out = append(out, spec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func (c *Catalog) Get(name string) (ToolSpec, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	spec, ok := c.tools[normalizeToolName(name)]
	return spec, ok
}

func (c *Catalog) CheckAllowed(name string, allowExperimental, allowUnknown bool) error {
	spec, ok := c.Get(name)
	if !ok {
		if allowUnknown {
			return nil
		}
		return fmt.Errorf("tool %q is not registered", name)
	}
	switch spec.Status {
	case StatusSupported:
		return nil
	case StatusExperimental:
		if allowExperimental {
			return nil
		}
		return fmt.Errorf("tool %q is experimental and disabled", spec.Name)
	case StatusUnsupported:
		return fmt.Errorf("tool %q is marked unsupported", spec.Name)
	default:
		return fmt.Errorf("tool %q has invalid status %q", spec.Name, spec.Status)
	}
}

func normalizeStatus(s Status) Status {
	switch Status(strings.ToLower(strings.TrimSpace(string(s)))) {
	case StatusSupported:
		return StatusSupported
	case StatusExperimental:
		return StatusExperimental
	case StatusUnsupported:
		return StatusUnsupported
	default:
		return StatusSupported
	}
}

func normalizeToolName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
