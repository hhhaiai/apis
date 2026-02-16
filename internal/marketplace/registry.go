package marketplace

// Registry manages plugin manifests and provides query capabilities.
type Registry interface {
	// List returns all available plugin manifests.
	List() ([]PluginManifest, error)

	// Get retrieves a specific plugin manifest by name.
	// If version is empty, returns the latest version.
	Get(name string, version string) (PluginManifest, error)

	// Search finds plugins matching query and tag filters.
	Search(query string, tags []string) ([]PluginManifest, error)

	// Add registers a new plugin manifest.
	Add(manifest PluginManifest) error

	// Remove deletes a plugin manifest by name.
	Remove(name string) error

	// Refresh reloads manifests from the source.
	Refresh() error
}
