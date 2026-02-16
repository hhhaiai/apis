package marketplace

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RemoteRegistry implements Registry by fetching manifests from a remote HTTP endpoint.
type RemoteRegistry struct {
	mu            sync.RWMutex
	url           string
	client        *http.Client
	manifests     map[string]PluginManifest
	versions      map[string][]PluginManifest
	lastRefresh   time.Time
	cacheDuration time.Duration
}

// NewRemoteRegistry creates a new remote HTTP-based registry.
func NewRemoteRegistry(url string, cacheDuration time.Duration) *RemoteRegistry {
	if cacheDuration == 0 {
		cacheDuration = 5 * time.Minute
	}
	return &RemoteRegistry{
		url:           url,
		client:        &http.Client{Timeout: 30 * time.Second},
		manifests:     make(map[string]PluginManifest),
		versions:      make(map[string][]PluginManifest),
		cacheDuration: cacheDuration,
	}
}

// List returns all available plugin manifests.
func (r *RemoteRegistry) List() ([]PluginManifest, error) {
	if err := r.refreshIfNeeded(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]PluginManifest, 0, len(r.manifests))
	for _, m := range r.manifests {
		result = append(result, m)
	}
	return result, nil
}

// Get retrieves a specific plugin manifest by name.
func (r *RemoteRegistry) Get(name string, version string) (PluginManifest, error) {
	if err := r.refreshIfNeeded(); err != nil {
		return PluginManifest{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)

	if version == "" {
		m, ok := r.manifests[name]
		if !ok {
			return PluginManifest{}, fmt.Errorf("plugin %q not found in registry", name)
		}
		return m, nil
	}

	versions, ok := r.versions[name]
	if !ok {
		return PluginManifest{}, fmt.Errorf("plugin %q not found in registry", name)
	}

	for _, m := range versions {
		if m.Version == version {
			return m, nil
		}
	}

	return PluginManifest{}, fmt.Errorf("plugin %q version %q not found", name, version)
}

// Search finds plugins matching query and tag filters.
func (r *RemoteRegistry) Search(query string, tags []string) ([]PluginManifest, error) {
	if err := r.refreshIfNeeded(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]PluginManifest, 0)

	for _, m := range r.manifests {
		if query != "" {
			nameMatch := strings.Contains(strings.ToLower(m.Name), query)
			descMatch := strings.Contains(strings.ToLower(m.Description), query)
			if !nameMatch && !descMatch {
				continue
			}
		}

		if len(tags) > 0 {
			if !hasAllTags(m.Tags, tags) {
				continue
			}
		}

		result = append(result, m)
	}

	return result, nil
}

// Add is not supported for remote registries.
func (r *RemoteRegistry) Add(manifest PluginManifest) error {
	return fmt.Errorf("add operation not supported for remote registry")
}

// Remove is not supported for remote registries.
func (r *RemoteRegistry) Remove(name string) error {
	return fmt.Errorf("remove operation not supported for remote registry")
}

// Refresh reloads manifests from the remote endpoint.
func (r *RemoteRegistry) Refresh() error {
	resp, err := r.client.Get(r.url)
	if err != nil {
		return fmt.Errorf("failed to fetch remote registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("remote registry returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var manifests []PluginManifest
	if err := json.Unmarshal(body, &manifests); err != nil {
		return fmt.Errorf("failed to parse manifests: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.manifests = make(map[string]PluginManifest)
	r.versions = make(map[string][]PluginManifest)

	for _, m := range manifests {
		if m.Name != "" {
			r.versions[m.Name] = append(r.versions[m.Name], m)
			r.manifests[m.Name] = m
		}
	}

	r.lastRefresh = time.Now()
	return nil
}

// refreshIfNeeded refreshes the cache if it has expired.
func (r *RemoteRegistry) refreshIfNeeded() error {
	r.mu.RLock()
	needsRefresh := time.Since(r.lastRefresh) > r.cacheDuration
	r.mu.RUnlock()

	if needsRefresh {
		return r.Refresh()
	}
	return nil
}
