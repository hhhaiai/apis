package plugin

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Plugin represents an installable plugin bundle.
type Plugin struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description,omitempty"`
	Skills      []SkillConfig     `json:"skills,omitempty"`
	Hooks       []HookConfig      `json:"hooks,omitempty"`
	MCPServers  []MCPServerConfig `json:"mcp_servers,omitempty"`
	Enabled     bool              `json:"enabled"`
	InstalledAt time.Time         `json:"installed_at"`
}

// SkillConfig defines a skill provided by a plugin.
type SkillConfig struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Template    string `json:"template"`
}

// HookConfig defines a hook provided by a plugin.
type HookConfig struct {
	Name     string `json:"name"`
	Point    string `json:"point"`
	Priority int    `json:"priority"`
}

// MCPServerConfig defines an MCP server provided by a plugin.
type MCPServerConfig struct {
	Name      string `json:"name"`
	Transport string `json:"transport"` // http or stdio
	URL       string `json:"url,omitempty"`
	Command   string `json:"command,omitempty"`
}

// Manager manages plugin installation and lifecycle.
type Manager struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
}

// NewManager creates a new plugin manager.
func NewManager() *Manager {
	return &Manager{
		plugins: make(map[string]Plugin),
	}
}

// Install registers a new plugin.
func (m *Manager) Install(p Plugin) error {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return fmt.Errorf("plugin name is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.plugins[name]; exists {
		return fmt.Errorf("plugin %q is already installed", name)
	}

	p.Name = name
	if p.Version == "" {
		p.Version = "1.0"
	}
	p.Enabled = true
	p.InstalledAt = time.Now().UTC()
	m.plugins[name] = p
	return nil
}

// Uninstall removes a plugin.
func (m *Manager) Uninstall(name string) error {
	name = strings.TrimSpace(name)
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.plugins[name]; !ok {
		return fmt.Errorf("plugin %q not found", name)
	}
	delete(m.plugins, name)
	return nil
}

// Get retrieves a plugin by name.
func (m *Manager) Get(name string) (Plugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.plugins[strings.TrimSpace(name)]
	return p, ok
}

// List returns all installed plugins.
func (m *Manager) List() []Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		out = append(out, p)
	}
	return out
}

// Enable enables a plugin.
func (m *Manager) Enable(name string) error {
	return m.setEnabled(name, true)
}

// Disable disables a plugin.
func (m *Manager) Disable(name string) error {
	return m.setEnabled(name, false)
}

func (m *Manager) setEnabled(name string, enabled bool) error {
	name = strings.TrimSpace(name)
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.plugins[name]
	if !ok {
		return fmt.Errorf("plugin %q not found", name)
	}
	p.Enabled = enabled
	m.plugins[name] = p
	return nil
}
