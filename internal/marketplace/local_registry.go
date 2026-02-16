package marketplace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// LocalRegistry implements Registry using local file system storage.
type LocalRegistry struct {
	mu        sync.RWMutex
	dir       string
	manifests map[string]PluginManifest   // key: plugin name (latest version)
	versions  map[string][]PluginManifest // key: plugin name, value: all versions
}

// NewLocalRegistry creates a new local file-based registry.
func NewLocalRegistry(dir string) *LocalRegistry {
	return &LocalRegistry{
		dir:       dir,
		manifests: make(map[string]PluginManifest),
		versions:  make(map[string][]PluginManifest),
	}
}

// List returns all available plugin manifests.
func (r *LocalRegistry) List() ([]PluginManifest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]PluginManifest, 0, len(r.manifests))
	for _, m := range r.manifests {
		result = append(result, m)
	}
	return result, nil
}

// Get retrieves a specific plugin manifest by name.
// If version is empty, returns the latest version.
// If version is specified, returns that specific version if available.
func (r *LocalRegistry) Get(name string, version string) (PluginManifest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)

	// If no version specified, return latest
	if version == "" {
		m, ok := r.manifests[name]
		if !ok {
			return PluginManifest{}, fmt.Errorf("plugin %q not found in registry", name)
		}
		return m, nil
	}

	// Search for specific version
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
func (r *LocalRegistry) Search(query string, tags []string) ([]PluginManifest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]PluginManifest, 0)

	for _, m := range r.manifests {
		// Text search: match name or description
		if query != "" {
			nameMatch := strings.Contains(strings.ToLower(m.Name), query)
			descMatch := strings.Contains(strings.ToLower(m.Description), query)
			if !nameMatch && !descMatch {
				continue
			}
		}

		// Tag filter: must have all specified tags (AND logic)
		if len(tags) > 0 {
			if !hasAllTags(m.Tags, tags) {
				continue
			}
		}

		result = append(result, m)
	}

	return result, nil
}

// Add registers a new plugin manifest.
// If a plugin with the same name already exists, adds this as a new version.
func (r *LocalRegistry) Add(manifest PluginManifest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := strings.TrimSpace(manifest.Name)
	if name == "" {
		return fmt.Errorf("plugin name is required")
	}

	// Add to version history
	r.versions[name] = append(r.versions[name], manifest)

	// Update latest version (simple: last added is latest)
	// In production, should compare semantic versions
	r.manifests[name] = manifest

	// Persist to file (versioned filename)
	filename := filepath.Join(r.dir, fmt.Sprintf("%s-%s.json", name, manifest.Version))
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	if err := os.MkdirAll(r.dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write manifest file: %w", err)
	}

	return nil
}

// Remove deletes a plugin manifest by name (all versions).
func (r *LocalRegistry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name = strings.TrimSpace(name)
	if _, ok := r.manifests[name]; !ok {
		return fmt.Errorf("plugin %q not found in registry", name)
	}

	// Get all versions to delete files
	versions := r.versions[name]

	// Remove from memory
	delete(r.manifests, name)
	delete(r.versions, name)

	// Remove all version files
	for _, v := range versions {
		filename := filepath.Join(r.dir, fmt.Sprintf("%s-%s.json", name, v.Version))
		if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove manifest file: %w", err)
		}
	}

	return nil
}

// Refresh reloads manifests from the file system.
func (r *LocalRegistry) Refresh() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing manifests
	r.manifests = make(map[string]PluginManifest)
	r.versions = make(map[string][]PluginManifest)

	// Check if directory exists
	if _, err := os.Stat(r.dir); os.IsNotExist(err) {
		// Directory doesn't exist yet, that's okay
		return nil
	}

	// Read all JSON files from directory
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filename := filepath.Join(r.dir, entry.Name())
		data, err := os.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filename, err)
		}

		var manifest PluginManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return fmt.Errorf("failed to parse manifest %s: %w", filename, err)
		}

		if manifest.Name != "" {
			// Add to version history
			r.versions[manifest.Name] = append(r.versions[manifest.Name], manifest)
		}
	}

	// Determine latest version for each plugin using semantic versioning
	for name, versions := range r.versions {
		if latest, ok := FindLatestVersion(versions); ok {
			r.manifests[name] = latest
		}
	}

	return nil
}

// hasAllTags checks if pluginTags contains all required tags.
func hasAllTags(pluginTags []string, requiredTags []string) bool {
	tagSet := make(map[string]bool)
	for _, t := range pluginTags {
		tagSet[strings.ToLower(strings.TrimSpace(t))] = true
	}

	for _, required := range requiredTags {
		required = strings.ToLower(strings.TrimSpace(required))
		if !tagSet[required] {
			return false
		}
	}

	return true
}
