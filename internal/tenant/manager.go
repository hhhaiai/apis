package tenant

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Tenant represents a tenant with API access.
type Tenant struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	APIKey    string         `json:"api_key"`
	QuotaRPM  int            `json:"quota_rpm"` // requests per minute, 0 = unlimited
	QuotaTPD  int            `json:"quota_tpd"` // tokens per day, 0 = unlimited
	IsActive  bool           `json:"is_active"`
	Usage     Usage          `json:"usage"`
	CreatedAt time.Time      `json:"created_at"`
	Meta      map[string]any `json:"metadata,omitempty"`
}

// Usage tracks tenant resource consumption.
type Usage struct {
	Requests    int64     `json:"requests"`
	Tokens      int64     `json:"tokens"`
	CostUSD     float64   `json:"cost_usd"`
	LastRequest time.Time `json:"last_request"`
	WindowStart time.Time `json:"window_start"`
	WindowReqs  int       `json:"window_requests"` // requests in current minute window
}

// Manager handles multi-tenant operations.
type Manager struct {
	mu      sync.RWMutex
	tenants map[string]*Tenant // id -> tenant
	keys    map[string]string  // api_key -> tenant id
}

// NewManager creates a tenant manager.
func NewManager() *Manager {
	return &Manager{
		tenants: make(map[string]*Tenant),
		keys:    make(map[string]string),
	}
}

// Create creates a new tenant.
func (m *Manager) Create(id, name, apiKey string, quotaRPM, quotaTPD int) (Tenant, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	apiKey = strings.TrimSpace(apiKey)
	if id == "" || name == "" || apiKey == "" {
		return Tenant{}, fmt.Errorf("id, name, and api_key are required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tenants[id]; exists {
		return Tenant{}, fmt.Errorf("tenant %q already exists", id)
	}
	if _, exists := m.keys[apiKey]; exists {
		return Tenant{}, fmt.Errorf("api_key is already in use")
	}

	t := &Tenant{
		ID:        id,
		Name:      name,
		APIKey:    apiKey,
		QuotaRPM:  quotaRPM,
		QuotaTPD:  quotaTPD,
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	}
	m.tenants[id] = t
	m.keys[apiKey] = id
	return *t, nil
}

// Get returns a tenant by ID.
func (m *Manager) Get(id string) (Tenant, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tenants[id]
	if !ok {
		return Tenant{}, false
	}
	return *t, true
}

// List returns all tenants.
func (m *Manager) List() []Tenant {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Tenant, 0, len(m.tenants))
	for _, t := range m.tenants {
		out = append(out, *t)
	}
	return out
}

// Delete removes a tenant.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tenants[id]
	if !ok {
		return fmt.Errorf("tenant %q not found", id)
	}
	delete(m.keys, t.APIKey)
	delete(m.tenants, id)
	return nil
}

// Deactivate disables a tenant.
func (m *Manager) Deactivate(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tenants[id]
	if !ok {
		return fmt.Errorf("tenant %q not found", id)
	}
	t.IsActive = false
	return nil
}

// Activate enables a tenant.
func (m *Manager) Activate(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tenants[id]
	if !ok {
		return fmt.Errorf("tenant %q not found", id)
	}
	t.IsActive = true
	return nil
}

// Authenticate validates an API key and returns the tenant, enforcing quotas.
func (m *Manager) Authenticate(apiKey string) (Tenant, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return Tenant{}, fmt.Errorf("api_key is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	id, ok := m.keys[apiKey]
	if !ok {
		return Tenant{}, fmt.Errorf("invalid api_key")
	}
	t := m.tenants[id]
	if !t.IsActive {
		return Tenant{}, fmt.Errorf("tenant %q is deactivated", id)
	}

	now := time.Now().UTC()

	// Reset minute window if needed
	if now.Sub(t.Usage.WindowStart) > time.Minute {
		t.Usage.WindowStart = now
		t.Usage.WindowReqs = 0
	}

	// Check RPM quota
	if t.QuotaRPM > 0 && t.Usage.WindowReqs >= t.QuotaRPM {
		return Tenant{}, fmt.Errorf("rate limit exceeded: %d requests per minute", t.QuotaRPM)
	}

	t.Usage.Requests++
	t.Usage.WindowReqs++
	t.Usage.LastRequest = now

	return *t, nil
}

// RecordTokens records token usage for a tenant.
func (m *Manager) RecordTokens(id string, tokens int, costUSD float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tenants[id]
	if !ok {
		return
	}
	t.Usage.Tokens += int64(tokens)
	t.Usage.CostUSD += costUSD
}
