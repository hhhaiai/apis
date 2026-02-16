package marketplace_test

import (
	"testing"

	"ccgateway/internal/marketplace"
	"ccgateway/internal/plugin"
)

func TestMarketplaceBasicFlow(t *testing.T) {
	// Create a temporary registry
	registry := marketplace.NewLocalRegistry("../../configs/marketplace")
	if err := registry.Refresh(); err != nil {
		t.Fatalf("failed to refresh registry: %v", err)
	}

	// Create plugin manager and marketplace service
	pluginManager := plugin.NewManager()
	service := marketplace.NewService(registry, pluginManager)

	// Test 1: List available plugins
	manifests, err := service.ListAvailable()
	if err != nil {
		t.Fatalf("ListAvailable failed: %v", err)
	}
	if len(manifests) < 5 {
		t.Errorf("expected at least 5 manifests, got %d", len(manifests))
	}

	// Test 2: Get specific manifest
	manifest, err := service.GetManifest("glm-local")
	if err != nil {
		t.Fatalf("GetManifest failed: %v", err)
	}
	if manifest.Name != "glm-local" {
		t.Errorf("expected name 'glm-local', got %q", manifest.Name)
	}
	if manifest.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", manifest.Version)
	}

	// Test 3: Search by tag
	results, err := service.Search("", []string{"proxy"})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 proxy plugins, got %d", len(results))
	}

	// Test 4: Search by query
	results, err = service.Search("openai", []string{})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least 1 result for 'openai' query")
	}

	// Test 5: Get recommendations (no plugins installed)
	recommendations, err := service.GetRecommendations()
	if err != nil {
		t.Fatalf("GetRecommendations failed: %v", err)
	}
	if len(recommendations) == 0 {
		t.Error("expected at least 1 recommendation")
	}
	if len(recommendations) > 5 {
		t.Errorf("expected max 5 recommendations, got %d", len(recommendations))
	}

	// Test 6: Check updates (no plugins installed)
	updates, err := service.CheckUpdates()
	if err != nil {
		t.Fatalf("CheckUpdates failed: %v", err)
	}
	if len(updates) != 0 {
		t.Errorf("expected 0 updates with no plugins installed, got %d", len(updates))
	}
}

func TestManifestValidation(t *testing.T) {
	validator := marketplace.NewValidator()

	// Test valid manifest
	validManifest := marketplace.PluginManifest{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "Test plugin",
		Author:      "Test Author",
		Source:      "builtin.ccgateway",
		Verified:    true,
		License:     "AGPL-3.0-or-later",
		Homepage:    "https://github.com/hhhaiai/apis/blob/main/docs/MARKETPLACE_GUIDE.md#1-glm-local",
	}
	if err := validator.ValidateManifest(validManifest); err != nil {
		t.Errorf("valid manifest rejected: %v", err)
	}

	// Test missing name
	invalidManifest := marketplace.PluginManifest{
		Version:     "1.0.0",
		Description: "Test plugin",
		Author:      "Test Author",
	}
	if err := validator.ValidateManifest(invalidManifest); err == nil {
		t.Error("expected error for missing name")
	}

	// Test invalid version
	invalidManifest = marketplace.PluginManifest{
		Name:        "test-plugin",
		Version:     "1.0",
		Description: "Test plugin",
		Author:      "Test Author",
	}
	if err := validator.ValidateManifest(invalidManifest); err == nil {
		t.Error("expected error for invalid version format")
	}

	// Test invalid name format
	invalidManifest = marketplace.PluginManifest{
		Name:        "test plugin!",
		Version:     "1.0.0",
		Description: "Test plugin",
		Author:      "Test Author",
	}
	if err := validator.ValidateManifest(invalidManifest); err == nil {
		t.Error("expected error for invalid name format")
	}

	// Test invalid homepage URL
	invalidManifest = marketplace.PluginManifest{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "Test plugin",
		Author:      "Test Author",
		Homepage:    "not-a-valid-url",
	}
	if err := validator.ValidateManifest(invalidManifest); err == nil {
		t.Error("expected error for invalid homepage URL")
	}

	// Test placeholder homepage URL
	invalidManifest = marketplace.PluginManifest{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "Test plugin",
		Author:      "Test Author",
		Homepage:    "https://github.com/ccgateway/plugins/test-plugin",
	}
	if err := validator.ValidateManifest(invalidManifest); err == nil {
		t.Error("expected error for placeholder homepage URL")
	}

	// Test invalid source format
	invalidManifest = marketplace.PluginManifest{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "Test plugin",
		Author:      "Test Author",
		Source:      "bad source",
	}
	if err := validator.ValidateManifest(invalidManifest); err == nil {
		t.Error("expected error for invalid source format")
	}

	// Test verified manifest without source
	invalidManifest = marketplace.PluginManifest{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "Test plugin",
		Author:      "Test Author",
		Verified:    true,
	}
	if err := validator.ValidateManifest(invalidManifest); err == nil {
		t.Error("expected error for verified manifest without source")
	}

	// Test invalid license placeholder
	invalidManifest = marketplace.PluginManifest{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "Test plugin",
		Author:      "Test Author",
		License:     "UNKNOWN",
	}
	if err := validator.ValidateManifest(invalidManifest); err == nil {
		t.Error("expected error for invalid license placeholder")
	}
}

func TestVersionComparison(t *testing.T) {
	registry := marketplace.NewLocalRegistry("../../configs/marketplace")
	pluginManager := plugin.NewManager()
	service := marketplace.NewService(registry, pluginManager)

	// Add test manifest to registry
	testManifest := marketplace.PluginManifest{
		Name:        "test-plugin",
		Version:     "2.0.0",
		Description: "Test plugin",
		Author:      "Test Author",
	}
	if err := registry.Add(testManifest); err != nil {
		t.Fatalf("failed to add test manifest: %v", err)
	}

	// Install older version
	oldPlugin := plugin.Plugin{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "Test plugin",
	}
	if err := pluginManager.Install(oldPlugin); err != nil {
		t.Fatalf("failed to install plugin: %v", err)
	}

	// Check for updates
	updates, err := service.CheckUpdates()
	if err != nil {
		t.Fatalf("CheckUpdates failed: %v", err)
	}

	found := false
	for _, update := range updates {
		if update.PluginName == "test-plugin" {
			found = true
			if !update.UpdateAvailable {
				t.Error("expected update to be available")
			}
			if update.CurrentVersion != "1.0.0" {
				t.Errorf("expected current version '1.0.0', got %q", update.CurrentVersion)
			}
			if update.AvailableVersion != "2.0.0" {
				t.Errorf("expected available version '2.0.0', got %q", update.AvailableVersion)
			}
		}
	}

	if !found {
		t.Error("test-plugin not found in updates")
	}
}

func TestErrorHandling(t *testing.T) {
	registry := marketplace.NewLocalRegistry("../../configs/marketplace")
	if err := registry.Refresh(); err != nil {
		t.Fatalf("failed to refresh registry: %v", err)
	}
	pluginManager := plugin.NewManager()
	service := marketplace.NewService(registry, pluginManager)

	// Test 1: Install non-existent plugin
	err := service.Install("non-existent-plugin", nil)
	if err == nil {
		t.Error("expected error when installing non-existent plugin")
	}
	if err != nil && !contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}

	// Test 2: Get non-existent manifest
	_, err = service.GetManifest("non-existent-plugin")
	if err == nil {
		t.Error("expected error when getting non-existent manifest")
	}

	// Test 3: Install plugin with missing dependencies
	// First add a plugin with dependencies to registry
	depManifest := marketplace.PluginManifest{
		Name:        "dependent-plugin",
		Version:     "1.0.0",
		Description: "Plugin with dependencies",
		Author:      "Test",
		Dependencies: []marketplace.Dependency{
			{Name: "missing-dep", VersionConstraint: ">=1.0.0"},
		},
	}
	if err := registry.Add(depManifest); err != nil {
		t.Fatalf("failed to add dependent manifest: %v", err)
	}

	// Try to install without dependencies
	err = service.Install("dependent-plugin", nil)
	if err == nil {
		t.Error("expected error when installing plugin with missing dependencies")
	}
	if err != nil && !contains(err.Error(), "dependencies") {
		t.Errorf("expected 'dependencies' error, got: %v", err)
	}

	// Test 4: Install already installed plugin
	testPlugin := plugin.Plugin{
		Name:        "already-installed",
		Version:     "1.0.0",
		Description: "Test",
	}
	if err := pluginManager.Install(testPlugin); err != nil {
		t.Fatalf("failed to install test plugin: %v", err)
	}

	alreadyManifest := marketplace.PluginManifest{
		Name:        "already-installed",
		Version:     "1.0.0",
		Description: "Test",
		Author:      "Test",
	}
	if err := registry.Add(alreadyManifest); err != nil {
		t.Fatalf("failed to add manifest: %v", err)
	}

	err = service.Install("already-installed", nil)
	if err == nil {
		t.Error("expected error when installing already installed plugin")
	}
	if err != nil && !contains(err.Error(), "already installed") {
		t.Errorf("expected 'already installed' error, got: %v", err)
	}

	// Test 5: Invalid manifest handling
	invalidManifest := marketplace.PluginManifest{
		Name:        "invalid!@#",
		Version:     "not-a-version",
		Description: "Test",
		Author:      "Test",
	}
	if err := registry.Add(invalidManifest); err != nil {
		t.Fatalf("failed to add invalid manifest: %v", err)
	}

	err = service.Install("invalid!@#", nil)
	if err == nil {
		t.Error("expected error when installing plugin with invalid manifest")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(s == substr || len(s) >= len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
