package marketplace

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// PluginStats tracks plugin usage statistics.
type PluginStats struct {
	PluginName      string    `json:"plugin_name"`
	InstallCount    int64     `json:"install_count"`
	UninstallCount  int64     `json:"uninstall_count"`
	LastInstalled   time.Time `json:"last_installed"`
	LastUninstalled time.Time `json:"last_uninstalled"`
	ActiveInstalls  int64     `json:"active_installs"`
}

// StatsTracker tracks marketplace statistics.
type StatsTracker struct {
	mu       sync.RWMutex
	stats    map[string]*PluginStats
	filePath string
}

// NewStatsTracker creates a new stats tracker.
func NewStatsTracker() *StatsTracker {
	return &StatsTracker{
		stats: make(map[string]*PluginStats),
	}
}

// NewStatsTrackerWithPersistence creates a stats tracker with file persistence.
func NewStatsTrackerWithPersistence(filePath string) *StatsTracker {
	tracker := &StatsTracker{
		stats:    make(map[string]*PluginStats),
		filePath: filePath,
	}
	// Try to load existing stats
	_ = tracker.Load()
	return tracker
}

// Load loads statistics from file.
func (t *StatsTracker) Load() error {
	if t.filePath == "" {
		return nil
	}

	data, err := os.ReadFile(t.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's okay
		}
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	var statsSlice []PluginStats
	if err := json.Unmarshal(data, &statsSlice); err != nil {
		return err
	}

	// Convert slice to map
	t.stats = make(map[string]*PluginStats)
	for i := range statsSlice {
		t.stats[statsSlice[i].PluginName] = &statsSlice[i]
	}

	return nil
}

// Save saves statistics to file.
func (t *StatsTracker) Save() error {
	if t.filePath == "" {
		return nil
	}

	t.mu.RLock()
	statsSlice := make([]PluginStats, 0, len(t.stats))
	for _, stats := range t.stats {
		statsSlice = append(statsSlice, *stats)
	}
	t.mu.RUnlock()

	data, err := json.MarshalIndent(statsSlice, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(t.filePath, data, 0644)
}

// RecordInstall records a plugin installation.
func (t *StatsTracker) RecordInstall(pluginName string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.stats[pluginName]; !ok {
		t.stats[pluginName] = &PluginStats{
			PluginName: pluginName,
		}
	}

	t.stats[pluginName].InstallCount++
	t.stats[pluginName].ActiveInstalls++
	t.stats[pluginName].LastInstalled = time.Now()

	// Auto-save if persistence is enabled
	if t.filePath != "" {
		t.mu.Unlock()
		_ = t.Save()
		t.mu.Lock()
	}
}

// RecordUninstall records a plugin uninstallation.
func (t *StatsTracker) RecordUninstall(pluginName string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.stats[pluginName]; !ok {
		t.stats[pluginName] = &PluginStats{
			PluginName: pluginName,
		}
	}

	t.stats[pluginName].UninstallCount++
	if t.stats[pluginName].ActiveInstalls > 0 {
		t.stats[pluginName].ActiveInstalls--
	}
	t.stats[pluginName].LastUninstalled = time.Now()

	// Auto-save if persistence is enabled
	if t.filePath != "" {
		t.mu.Unlock()
		_ = t.Save()
		t.mu.Lock()
	}
}

// GetStats returns statistics for a specific plugin.
func (t *StatsTracker) GetStats(pluginName string) (PluginStats, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats, ok := t.stats[pluginName]
	if !ok {
		return PluginStats{}, false
	}
	return *stats, true
}

// GetAllStats returns all plugin statistics.
func (t *StatsTracker) GetAllStats() []PluginStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]PluginStats, 0, len(t.stats))
	for _, stats := range t.stats {
		result = append(result, *stats)
	}
	return result
}

// GetPopularPlugins returns the most popular plugins by active installs.
func (t *StatsTracker) GetPopularPlugins(limit int) []PluginStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	all := make([]PluginStats, 0, len(t.stats))
	for _, stats := range t.stats {
		all = append(all, *stats)
	}

	// Simple bubble sort by active installs
	n := len(all)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if all[j].ActiveInstalls < all[j+1].ActiveInstalls {
				all[j], all[j+1] = all[j+1], all[j]
			}
		}
	}

	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}

	return all
}
