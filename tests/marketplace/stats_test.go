package marketplace_test

import (
	"testing"
	"time"

	"ccgateway/internal/marketplace"
)

func TestStatsTracker(t *testing.T) {
	tracker := marketplace.NewStatsTracker()

	// Test recording install
	tracker.RecordInstall("test-plugin")
	stats, ok := tracker.GetStats("test-plugin")
	if !ok {
		t.Fatal("expected stats to exist")
	}
	if stats.InstallCount != 1 {
		t.Errorf("expected install count 1, got %d", stats.InstallCount)
	}
	if stats.ActiveInstalls != 1 {
		t.Errorf("expected active installs 1, got %d", stats.ActiveInstalls)
	}

	// Test recording multiple installs
	tracker.RecordInstall("test-plugin")
	stats, _ = tracker.GetStats("test-plugin")
	if stats.InstallCount != 2 {
		t.Errorf("expected install count 2, got %d", stats.InstallCount)
	}
	if stats.ActiveInstalls != 2 {
		t.Errorf("expected active installs 2, got %d", stats.ActiveInstalls)
	}

	// Test recording uninstall
	tracker.RecordUninstall("test-plugin")
	stats, _ = tracker.GetStats("test-plugin")
	if stats.UninstallCount != 1 {
		t.Errorf("expected uninstall count 1, got %d", stats.UninstallCount)
	}
	if stats.ActiveInstalls != 1 {
		t.Errorf("expected active installs 1, got %d", stats.ActiveInstalls)
	}

	// Test last installed/uninstalled timestamps
	if stats.LastInstalled.IsZero() {
		t.Error("expected last_installed to be set")
	}
	if stats.LastUninstalled.IsZero() {
		t.Error("expected last_uninstalled to be set")
	}
}

func TestGetPopularPlugins(t *testing.T) {
	tracker := marketplace.NewStatsTracker()

	// Create plugins with different popularity
	tracker.RecordInstall("plugin-a")
	tracker.RecordInstall("plugin-a")
	tracker.RecordInstall("plugin-a")

	tracker.RecordInstall("plugin-b")
	tracker.RecordInstall("plugin-b")

	tracker.RecordInstall("plugin-c")

	// Get popular plugins
	popular := tracker.GetPopularPlugins(0)
	if len(popular) != 3 {
		t.Fatalf("expected 3 plugins, got %d", len(popular))
	}

	// Verify sorting by active installs
	if popular[0].PluginName != "plugin-a" {
		t.Errorf("expected plugin-a to be most popular, got %s", popular[0].PluginName)
	}
	if popular[0].ActiveInstalls != 3 {
		t.Errorf("expected plugin-a to have 3 active installs, got %d", popular[0].ActiveInstalls)
	}

	if popular[1].PluginName != "plugin-b" {
		t.Errorf("expected plugin-b to be second, got %s", popular[1].PluginName)
	}
	if popular[1].ActiveInstalls != 2 {
		t.Errorf("expected plugin-b to have 2 active installs, got %d", popular[1].ActiveInstalls)
	}

	// Test limit
	limited := tracker.GetPopularPlugins(2)
	if len(limited) != 2 {
		t.Errorf("expected 2 plugins with limit, got %d", len(limited))
	}
}

func TestGetAllStats(t *testing.T) {
	tracker := marketplace.NewStatsTracker()

	tracker.RecordInstall("plugin-1")
	tracker.RecordInstall("plugin-2")
	tracker.RecordInstall("plugin-3")

	allStats := tracker.GetAllStats()
	if len(allStats) != 3 {
		t.Errorf("expected 3 stats entries, got %d", len(allStats))
	}
}

func TestStatsNotFound(t *testing.T) {
	tracker := marketplace.NewStatsTracker()

	_, ok := tracker.GetStats("nonexistent")
	if ok {
		t.Error("expected stats not found for nonexistent plugin")
	}
}

func TestUninstallWithoutInstall(t *testing.T) {
	tracker := marketplace.NewStatsTracker()

	// Uninstall a plugin that was never installed
	tracker.RecordUninstall("test-plugin")

	stats, ok := tracker.GetStats("test-plugin")
	if !ok {
		t.Fatal("expected stats to be created")
	}

	if stats.UninstallCount != 1 {
		t.Errorf("expected uninstall count 1, got %d", stats.UninstallCount)
	}
	if stats.ActiveInstalls != 0 {
		t.Errorf("expected active installs 0, got %d", stats.ActiveInstalls)
	}
}

func TestTimestamps(t *testing.T) {
	tracker := marketplace.NewStatsTracker()

	before := time.Now()
	tracker.RecordInstall("test-plugin")
	after := time.Now()

	stats, _ := tracker.GetStats("test-plugin")

	if stats.LastInstalled.Before(before) || stats.LastInstalled.After(after) {
		t.Error("last_installed timestamp is out of expected range")
	}

	before = time.Now()
	tracker.RecordUninstall("test-plugin")
	after = time.Now()

	stats, _ = tracker.GetStats("test-plugin")

	if stats.LastUninstalled.Before(before) || stats.LastUninstalled.After(after) {
		t.Error("last_uninstalled timestamp is out of expected range")
	}
}
