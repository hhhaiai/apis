package marketplace

import (
	"fmt"
	"strconv"
	"strings"
)

// CompareVersions compares two semantic versions.
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal.
func CompareVersions(v1, v2 string) int {
	parts1 := parseVersion(v1)
	parts2 := parseVersion(v2)

	for i := 0; i < 3; i++ {
		if parts1[i] > parts2[i] {
			return 1
		}
		if parts1[i] < parts2[i] {
			return -1
		}
	}

	return 0
}

// parseVersion parses a semantic version string into [major, minor, patch].
func parseVersion(version string) [3]int {
	var result [3]int
	parts := strings.Split(strings.TrimSpace(version), ".")

	for i := 0; i < 3 && i < len(parts); i++ {
		var num int
		fmt.Sscanf(parts[i], "%d", &num)
		result[i] = num
	}

	return result
}

// FindLatestVersion returns the latest version from a list of manifests.
func FindLatestVersion(manifests []PluginManifest) (PluginManifest, bool) {
	if len(manifests) == 0 {
		return PluginManifest{}, false
	}

	latest := manifests[0]
	for i := 1; i < len(manifests); i++ {
		if CompareVersions(manifests[i].Version, latest.Version) > 0 {
			latest = manifests[i]
		}
	}

	return latest, true
}

// CheckVersionConstraint checks if a version satisfies a constraint.
// Supported constraints: ">=1.0.0", "<=2.0.0", "^1.0.0", "~1.2.0", "1.0.0"
func CheckVersionConstraint(version, constraint string) (bool, error) {
	constraint = strings.TrimSpace(constraint)
	version = strings.TrimSpace(version)

	// Exact match
	if !strings.ContainsAny(constraint, ">=<^~") {
		return version == constraint, nil
	}

	// >= constraint
	if strings.HasPrefix(constraint, ">=") {
		targetVersion := strings.TrimPrefix(constraint, ">=")
		return CompareVersions(version, targetVersion) >= 0, nil
	}

	// <= constraint
	if strings.HasPrefix(constraint, "<=") {
		targetVersion := strings.TrimPrefix(constraint, "<=")
		return CompareVersions(version, targetVersion) <= 0, nil
	}

	// > constraint
	if strings.HasPrefix(constraint, ">") {
		targetVersion := strings.TrimPrefix(constraint, ">")
		return CompareVersions(version, targetVersion) > 0, nil
	}

	// < constraint
	if strings.HasPrefix(constraint, "<") {
		targetVersion := strings.TrimPrefix(constraint, "<")
		return CompareVersions(version, targetVersion) < 0, nil
	}

	// ^ constraint (compatible with, same major version)
	if strings.HasPrefix(constraint, "^") {
		targetVersion := strings.TrimPrefix(constraint, "^")
		targetParts := parseVersion(targetVersion)
		versionParts := parseVersion(version)

		// Major version must match
		if versionParts[0] != targetParts[0] {
			return false, nil
		}

		// Version must be >= target
		return CompareVersions(version, targetVersion) >= 0, nil
	}

	// ~ constraint (approximately equivalent, same major.minor)
	if strings.HasPrefix(constraint, "~") {
		targetVersion := strings.TrimPrefix(constraint, "~")
		targetParts := parseVersion(targetVersion)
		versionParts := parseVersion(version)

		// Major and minor must match
		if versionParts[0] != targetParts[0] || versionParts[1] != targetParts[1] {
			return false, nil
		}

		// Patch must be >= target
		return versionParts[2] >= targetParts[2], nil
	}

	return false, fmt.Errorf("unsupported version constraint: %s", constraint)
}

// ParseVersionParts parses a version string into major, minor, patch components.
func ParseVersionParts(version string) (major, minor, patch int, err error) {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("invalid version format: %s", version)
	}

	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid major version: %s", parts[0])
	}

	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid minor version: %s", parts[1])
	}

	patch, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid patch version: %s", parts[2])
	}

	return major, minor, patch, nil
}
