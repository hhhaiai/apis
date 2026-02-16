package marketplace

import (
	"ccgateway/internal/plugin"
	"fmt"
	"strings"
)

// Service orchestrates plugin discovery, validation, and installation.
type Service struct {
	registry      Registry
	pluginManager *plugin.Manager
	validator     *Validator
	statsTracker  *StatsTracker
}

// NewService creates a new marketplace service.
func NewService(registry Registry, pluginManager *plugin.Manager) *Service {
	return &Service{
		registry:      registry,
		pluginManager: pluginManager,
		validator:     NewValidator(),
		statsTracker:  NewStatsTracker(),
	}
}

// NewServiceWithStats creates a new marketplace service with custom stats tracker.
func NewServiceWithStats(registry Registry, pluginManager *plugin.Manager, statsTracker *StatsTracker) *Service {
	return &Service{
		registry:      registry,
		pluginManager: pluginManager,
		validator:     NewValidator(),
		statsTracker:  statsTracker,
	}
}

// ListAvailable returns all plugins in the marketplace.
func (s *Service) ListAvailable() ([]PluginManifest, error) {
	return s.registry.List()
}

// GetManifest retrieves a specific plugin manifest.
func (s *Service) GetManifest(name string) (PluginManifest, error) {
	return s.registry.Get(name, "")
}

// Search finds plugins matching criteria with relevance scoring.
func (s *Service) Search(query string, tags []string) ([]SearchResult, error) {
	// Get matching manifests from registry
	manifests, err := s.registry.Search(query, tags)
	if err != nil {
		return nil, err
	}

	// Convert to search results with relevance scoring
	results := make([]SearchResult, 0, len(manifests))
	for _, m := range manifests {
		score := s.calculateRelevance(m, query)

		// Check if plugin is installed
		installed := false
		installedVersion := ""
		if p, ok := s.pluginManager.Get(m.Name); ok {
			installed = true
			installedVersion = p.Version
		}

		results = append(results, SearchResult{
			Manifest:         m,
			RelevanceScore:   score,
			Installed:        installed,
			InstalledVersion: installedVersion,
		})
	}

	// Sort by relevance score (descending)
	sortByRelevance(results)

	return results, nil
}

// calculateRelevance computes a relevance score for a plugin.
func (s *Service) calculateRelevance(manifest PluginManifest, query string) float64 {
	if query == "" {
		return 50.0 // Default score when no query
	}

	query = toLower(query)
	name := toLower(manifest.Name)
	desc := toLower(manifest.Description)

	// Exact name match: highest score
	if name == query {
		return 100.0
	}

	// Name contains query
	if contains(name, query) {
		queryLen := float64(len(query))
		nameLen := float64(len(name))
		return 50.0 + (queryLen/nameLen)*50.0
	}

	// Description contains query
	if contains(desc, query) {
		queryLen := float64(len(query))
		descLen := float64(len(desc))
		return 25.0 + (queryLen/descLen)*25.0
	}

	// Tag matches query
	for _, tag := range manifest.Tags {
		if toLower(tag) == query {
			return 75.0
		}
	}

	return 0.0
}

// sortByRelevance sorts search results by relevance score in descending order.
func sortByRelevance(results []SearchResult) {
	// Simple bubble sort (sufficient for small result sets)
	n := len(results)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if results[j].RelevanceScore < results[j+1].RelevanceScore {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}
}

// Helper functions
func toLower(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// Install installs a plugin from the marketplace.
func (s *Service) Install(name string, config map[string]string) error {
	// Get manifest from registry
	manifest, err := s.registry.Get(name, "")
	if err != nil {
		return fmt.Errorf("plugin %q not found in marketplace", name)
	}

	// Validate manifest
	if err := s.validator.ValidateManifest(manifest); err != nil {
		return fmt.Errorf("manifest validation failed: %w", err)
	}

	// Check if already installed
	if _, ok := s.pluginManager.Get(name); ok {
		return fmt.Errorf("plugin %q is already installed", name)
	}

	// Check dependencies
	if len(manifest.Dependencies) > 0 {
		missing := s.checkDependencies(manifest.Dependencies)
		if len(missing) > 0 {
			return fmt.Errorf("missing required dependencies: %v", missing)
		}
	}

	// Validate required configuration
	if err := s.validateRequiredConfig(manifest.ConfigSchema, config); err != nil {
		return err
	}

	// Convert manifest to plugin.Plugin
	p := plugin.Plugin{
		Name:        manifest.Name,
		Version:     manifest.Version,
		Description: manifest.Description,
		Skills:      manifest.Skills,
		Hooks:       manifest.Hooks,
		MCPServers:  manifest.MCPServers,
		Enabled:     true,
	}

	// Install via plugin manager
	if err := s.pluginManager.Install(p); err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	// Record installation statistics
	s.statsTracker.RecordInstall(name)

	return nil
}

// checkDependencies returns list of missing dependencies.
func (s *Service) checkDependencies(deps []Dependency) []string {
	missing := make([]string, 0)
	for _, dep := range deps {
		installedPlugin, ok := s.pluginManager.Get(dep.Name)
		if !ok {
			missing = append(missing, dep.Name)
			continue
		}

		// Check version constraint if specified
		if dep.VersionConstraint != "" {
			satisfied, err := CheckVersionConstraint(installedPlugin.Version, dep.VersionConstraint)
			if err != nil || !satisfied {
				missing = append(missing, fmt.Sprintf("%s (requires %s, found %s)",
					dep.Name, dep.VersionConstraint, installedPlugin.Version))
			}
		}
	}
	return missing
}

// validateRequiredConfig checks if all required config fields are provided.
func (s *Service) validateRequiredConfig(schema map[string]ConfigField, config map[string]string) error {
	for fieldName, field := range schema {
		if field.Required {
			value, ok := config[fieldName]
			if !ok || strings.TrimSpace(value) == "" {
				return fmt.Errorf("missing required configuration field: %s", fieldName)
			}
		}
	}
	return nil
}

// CheckUpdates compares installed versions with registry versions.
func (s *Service) CheckUpdates() ([]UpdateInfo, error) {
	installed := s.pluginManager.List()
	updates := make([]UpdateInfo, 0)

	for _, p := range installed {
		// Get latest version from registry
		manifest, err := s.registry.Get(p.Name, "")
		if err != nil {
			// Plugin not in registry, skip
			continue
		}

		// Compare versions
		updateAvailable := CompareVersions(manifest.Version, p.Version) > 0

		updates = append(updates, UpdateInfo{
			PluginName:       p.Name,
			CurrentVersion:   p.Version,
			AvailableVersion: manifest.Version,
			UpdateAvailable:  updateAvailable,
		})
	}

	return updates, nil
}

// Update updates an installed plugin to the latest version.
func (s *Service) Update(name string) error {
	// Get current installed plugin
	current, ok := s.pluginManager.Get(name)
	if !ok {
		return fmt.Errorf("plugin %q is not installed", name)
	}

	// Get latest version from registry
	manifest, err := s.registry.Get(name, "")
	if err != nil {
		return fmt.Errorf("plugin %q not found in marketplace", name)
	}

	// Check if update is available
	if CompareVersions(manifest.Version, current.Version) <= 0 {
		return fmt.Errorf("plugin %q is already at the latest version", name)
	}

	// Validate new manifest
	if err := s.validator.ValidateManifest(manifest); err != nil {
		return fmt.Errorf("manifest validation failed: %w", err)
	}

	// Backup current plugin for rollback
	backup := current

	// Uninstall current version
	if err := s.pluginManager.Uninstall(name); err != nil {
		return fmt.Errorf("failed to uninstall current version: %w", err)
	}

	// Install new version
	newPlugin := plugin.Plugin{
		Name:        manifest.Name,
		Version:     manifest.Version,
		Description: manifest.Description,
		Skills:      manifest.Skills,
		Hooks:       manifest.Hooks,
		MCPServers:  manifest.MCPServers,
		Enabled:     backup.Enabled, // Preserve enabled state
	}

	if err := s.pluginManager.Install(newPlugin); err != nil {
		// Rollback: restore previous version
		_ = s.pluginManager.Install(backup)
		return fmt.Errorf("update failed: %w. Previous version restored", err)
	}

	return nil
}

// GetPopularPlugins returns the most popular plugins.
func (s *Service) GetPopularPlugins(limit int) []PluginStats {
	return s.statsTracker.GetPopularPlugins(limit)
}

func (s *Service) GetRecommendations() ([]PluginManifest, error) {
	allPlugins, err := s.registry.List()
	if err != nil {
		return nil, err
	}

	installed := s.pluginManager.List()

	// If no plugins installed, recommend starter bundles + popular plugins
	if len(installed) == 0 {
		starters := s.getStarterBundles(allPlugins)
		if len(starters) >= 5 {
			return starters[:5], nil
		}

		// Fill remaining slots with popular plugins
		popular := s.getPopularUninstalledPlugins(allPlugins, installed, 5-len(starters))
		return append(starters, popular...), nil
	}

	// Otherwise, return popular uninstalled plugins
	recommendations := s.getPopularUninstalledPlugins(allPlugins, installed, 5)
	return recommendations, nil
}

// getPopularUninstalledPlugins returns popular plugins that are not installed.
func (s *Service) getPopularUninstalledPlugins(allPlugins []PluginManifest, installed []plugin.Plugin, limit int) []PluginManifest {
	installedNames := make(map[string]bool)
	for _, p := range installed {
		installedNames[p.Name] = true
	}

	// Get popular plugins from stats
	popularStats := s.statsTracker.GetPopularPlugins(0) // Get all
	recommendations := make([]PluginManifest, 0, limit)

	// First, add popular plugins that are not installed
	for _, stats := range popularStats {
		if installedNames[stats.PluginName] {
			continue
		}

		// Find manifest for this plugin
		for _, m := range allPlugins {
			if m.Name == stats.PluginName {
				recommendations = append(recommendations, m)
				break
			}
		}

		if len(recommendations) >= limit {
			return recommendations
		}
	}

	// If not enough popular plugins, fill with any uninstalled plugins
	for _, m := range allPlugins {
		if installedNames[m.Name] {
			continue
		}

		// Check if already in recommendations
		alreadyAdded := false
		for _, rec := range recommendations {
			if rec.Name == m.Name {
				alreadyAdded = true
				break
			}
		}

		if !alreadyAdded {
			recommendations = append(recommendations, m)
			if len(recommendations) >= limit {
				break
			}
		}
	}

	return recommendations
}

// getStarterBundles returns recommended starter bundles.
func (s *Service) getStarterBundles(allPlugins []PluginManifest) []PluginManifest {
	starterNames := []string{"glm-local", "openai-proxy", "anthropic-proxy", "search-tools", "file-tools"}
	starters := make([]PluginManifest, 0, 5)

	for _, name := range starterNames {
		for _, m := range allPlugins {
			if m.Name == name {
				starters = append(starters, m)
				break
			}
		}
		if len(starters) >= 5 {
			break
		}
	}

	return starters
}

// Uninstall uninstalls a plugin and records statistics.
func (s *Service) Uninstall(name string) error {
	if err := s.pluginManager.Uninstall(name); err != nil {
		return err
	}

	// Record uninstallation statistics
	s.statsTracker.RecordUninstall(name)

	return nil
}

// GetStats returns statistics for a specific plugin.
func (s *Service) GetStats(pluginName string) (PluginStats, bool) {
	return s.statsTracker.GetStats(pluginName)
}
