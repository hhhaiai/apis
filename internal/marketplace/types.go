package marketplace

import "ccgateway/internal/plugin"

// PluginManifest describes a plugin available in the marketplace.
type PluginManifest struct {
	Name         string                   `json:"name"`
	Version      string                   `json:"version"`
	Description  string                   `json:"description"`
	Author       string                   `json:"author"`
	Tags         []string                 `json:"tags,omitempty"`
	Source       string                   `json:"source,omitempty"`
	Verified     bool                     `json:"verified,omitempty"`
	Dependencies []Dependency             `json:"dependencies,omitempty"`
	Homepage     string                   `json:"homepage,omitempty"`
	License      string                   `json:"license,omitempty"`
	IconURL      string                   `json:"icon_url,omitempty"`
	Skills       []plugin.SkillConfig     `json:"skills,omitempty"`
	Hooks        []plugin.HookConfig      `json:"hooks,omitempty"`
	MCPServers   []plugin.MCPServerConfig `json:"mcp_servers,omitempty"`
	ConfigSchema map[string]ConfigField   `json:"config_schema,omitempty"`
}

// Dependency represents a plugin dependency with version constraint.
type Dependency struct {
	Name              string `json:"name"`
	VersionConstraint string `json:"version_constraint"` // e.g., ">=1.0.0", "^2.0.0"
}

// ConfigField describes a configuration field for a plugin.
type ConfigField struct {
	Type        string `json:"type"` // "string", "int", "bool", "url"
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
}

// UpdateInfo contains information about available plugin updates.
type UpdateInfo struct {
	PluginName       string `json:"plugin_name"`
	CurrentVersion   string `json:"current_version"`
	AvailableVersion string `json:"available_version"`
	UpdateAvailable  bool   `json:"update_available"`
}

// SearchResult represents a plugin search result with relevance scoring.
type SearchResult struct {
	Manifest         PluginManifest `json:"manifest"`
	RelevanceScore   float64        `json:"relevance_score"`
	Installed        bool           `json:"installed"`
	InstalledVersion string         `json:"installed_version,omitempty"`
}
