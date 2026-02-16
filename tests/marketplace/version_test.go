package marketplace_test

import (
	"testing"

	"ccgateway/internal/marketplace"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.9.9", "2.0.0", -1},
		{"1.2.3", "1.2.3", 0},
		{"1.10.0", "1.9.0", 1},
		{"1.0.10", "1.0.9", 1},
	}

	for _, tt := range tests {
		result := marketplace.CompareVersions(tt.v1, tt.v2)
		if result != tt.expected {
			t.Errorf("CompareVersions(%q, %q) = %d, expected %d", tt.v1, tt.v2, result, tt.expected)
		}
	}
}

func TestFindLatestVersion(t *testing.T) {
	manifests := []marketplace.PluginManifest{
		{Name: "test", Version: "1.0.0"},
		{Name: "test", Version: "1.2.0"},
		{Name: "test", Version: "1.1.0"},
		{Name: "test", Version: "2.0.0"},
		{Name: "test", Version: "1.5.0"},
	}

	latest, ok := marketplace.FindLatestVersion(manifests)
	if !ok {
		t.Fatal("expected to find latest version")
	}

	if latest.Version != "2.0.0" {
		t.Errorf("expected latest version 2.0.0, got %s", latest.Version)
	}
}

func TestFindLatestVersionEmpty(t *testing.T) {
	_, ok := marketplace.FindLatestVersion([]marketplace.PluginManifest{})
	if ok {
		t.Error("expected no latest version for empty list")
	}
}

func TestCheckVersionConstraint(t *testing.T) {
	tests := []struct {
		version    string
		constraint string
		expected   bool
		shouldErr  bool
	}{
		// Exact match
		{"1.0.0", "1.0.0", true, false},
		{"1.0.0", "1.0.1", false, false},

		// >= constraint
		{"1.5.0", ">=1.0.0", true, false},
		{"1.0.0", ">=1.0.0", true, false},
		{"0.9.0", ">=1.0.0", false, false},

		// <= constraint
		{"1.0.0", "<=1.5.0", true, false},
		{"1.5.0", "<=1.5.0", true, false},
		{"2.0.0", "<=1.5.0", false, false},

		// > constraint
		{"1.5.0", ">1.0.0", true, false},
		{"1.0.0", ">1.0.0", false, false},

		// < constraint
		{"1.0.0", "<1.5.0", true, false},
		{"1.5.0", "<1.5.0", false, false},

		// ^ constraint (compatible with)
		{"1.5.0", "^1.0.0", true, false},
		{"1.0.0", "^1.0.0", true, false},
		{"2.0.0", "^1.0.0", false, false},
		{"0.9.0", "^1.0.0", false, false},

		// ~ constraint (approximately equivalent)
		{"1.2.5", "~1.2.0", true, false},
		{"1.2.0", "~1.2.0", true, false},
		{"1.3.0", "~1.2.0", false, false},
		{"1.1.9", "~1.2.0", false, false},
	}

	for _, tt := range tests {
		result, err := marketplace.CheckVersionConstraint(tt.version, tt.constraint)
		if tt.shouldErr && err == nil {
			t.Errorf("CheckVersionConstraint(%q, %q) expected error", tt.version, tt.constraint)
		}
		if !tt.shouldErr && err != nil {
			t.Errorf("CheckVersionConstraint(%q, %q) unexpected error: %v", tt.version, tt.constraint, err)
		}
		if result != tt.expected {
			t.Errorf("CheckVersionConstraint(%q, %q) = %v, expected %v", tt.version, tt.constraint, result, tt.expected)
		}
	}
}

func TestParseVersionParts(t *testing.T) {
	major, minor, patch, err := marketplace.ParseVersionParts("1.2.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if major != 1 || minor != 2 || patch != 3 {
		t.Errorf("expected 1.2.3, got %d.%d.%d", major, minor, patch)
	}

	// Test invalid versions
	invalidVersions := []string{
		"1.2",
		"1",
		"1.2.3.4",
		"a.b.c",
		"1.x.3",
	}

	for _, v := range invalidVersions {
		_, _, _, err := marketplace.ParseVersionParts(v)
		if err == nil {
			t.Errorf("expected error for invalid version %q", v)
		}
	}
}
