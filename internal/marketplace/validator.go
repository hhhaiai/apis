package marketplace

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Validator ensures plugin manifests are safe and well-formed.
type Validator struct {
	namePattern    *regexp.Regexp
	versionPattern *regexp.Regexp
	sourcePattern  *regexp.Regexp
	licensePattern *regexp.Regexp
	dangerousChars *regexp.Regexp
}

// NewValidator creates a new manifest validator.
func NewValidator() *Validator {
	return &Validator{
		namePattern:    regexp.MustCompile(`^[a-zA-Z0-9_-]+$`),
		versionPattern: regexp.MustCompile(`^\d+\.\d+\.\d+$`),
		sourcePattern:  regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]{1,63}$`),
		licensePattern: regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+.\-]*$`),
		dangerousChars: regexp.MustCompile(`[;&|$<>` + "`" + `(){}[\]\\]`),
	}
}

// ValidateManifest checks manifest structure and content.
func (v *Validator) ValidateManifest(manifest PluginManifest) error {
	// Check required fields
	if strings.TrimSpace(manifest.Name) == "" {
		return fmt.Errorf("missing required field: name")
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return fmt.Errorf("missing required field: version")
	}
	if strings.TrimSpace(manifest.Description) == "" {
		return fmt.Errorf("missing required field: description")
	}
	if strings.TrimSpace(manifest.Author) == "" {
		return fmt.Errorf("missing required field: author")
	}

	// Validate name format
	if err := v.ValidateName(manifest.Name); err != nil {
		return err
	}

	// Validate version format
	if err := v.ValidateVersion(manifest.Version); err != nil {
		return err
	}

	// Validate homepage URL if present.
	if err := v.ValidateHomepage(manifest.Homepage); err != nil {
		return err
	}
	if err := v.ValidateSource(manifest.Source); err != nil {
		return err
	}
	if manifest.Verified && strings.TrimSpace(manifest.Source) == "" {
		return fmt.Errorf("verified manifest must include source")
	}
	if err := v.ValidateLicense(manifest.License); err != nil {
		return err
	}

	// Validate dependencies
	for _, dep := range manifest.Dependencies {
		if strings.TrimSpace(dep.Name) == "" {
			return fmt.Errorf("dependency missing name field")
		}
		if strings.TrimSpace(dep.VersionConstraint) == "" {
			return fmt.Errorf("dependency %q missing version_constraint field", dep.Name)
		}
	}

	// Validate MCP server commands
	for _, mcp := range manifest.MCPServers {
		if mcp.Command != "" {
			if err := v.ValidateCommand(mcp.Command); err != nil {
				return fmt.Errorf("MCP server %q: %w", mcp.Name, err)
			}
		}
	}

	return nil
}

// ValidateSource checks source format for marketplace trust metadata.
func (v *Validator) ValidateSource(source string) error {
	source = strings.TrimSpace(strings.ToLower(source))
	if source == "" {
		return nil
	}
	if !v.sourcePattern.MatchString(source) {
		return fmt.Errorf("source must match [a-z0-9][a-z0-9._:-]{1,63}")
	}
	if strings.Contains(source, "example") || strings.Contains(source, "todo") {
		return fmt.Errorf("source contains placeholder value")
	}
	return nil
}

// ValidateLicense checks license format and blocks placeholder values.
func (v *Validator) ValidateLicense(license string) error {
	license = strings.TrimSpace(license)
	if license == "" {
		return nil
	}
	if !v.licensePattern.MatchString(license) {
		return fmt.Errorf("license must be a valid SPDX-like identifier")
	}
	lower := strings.ToLower(license)
	if strings.Contains(lower, "unknown") || strings.Contains(lower, "todo") {
		return fmt.Errorf("license contains placeholder value")
	}
	return nil
}

// ValidateHomepage checks homepage URL format and blocks known placeholder values.
func (v *Validator) ValidateHomepage(homepage string) error {
	homepage = strings.TrimSpace(homepage)
	if homepage == "" {
		return nil
	}

	u, err := url.ParseRequestURI(homepage)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("homepage must be a valid absolute URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("homepage must use http or https scheme")
	}

	lower := strings.ToLower(homepage)
	placeholders := []string{
		"example.com",
		"github.com/ccgateway/plugins/",
	}
	for _, marker := range placeholders {
		if strings.Contains(lower, marker) {
			return fmt.Errorf("homepage points to a placeholder URL")
		}
	}

	return nil
}

// ValidateName checks plugin name format.
func (v *Validator) ValidateName(name string) error {
	name = strings.TrimSpace(name)
	if !v.namePattern.MatchString(name) {
		return fmt.Errorf("plugin name must contain only alphanumeric characters, hyphens, and underscores")
	}
	return nil
}

// ValidateVersion checks semantic versioning format.
func (v *Validator) ValidateVersion(version string) error {
	version = strings.TrimSpace(version)
	if !v.versionPattern.MatchString(version) {
		return fmt.Errorf("version must follow semantic versioning (major.minor.patch)")
	}

	// Additional check: parse version components
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return fmt.Errorf("version must follow semantic versioning (major.minor.patch)")
	}

	for i, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			componentName := []string{"major", "minor", "patch"}[i]
			return fmt.Errorf("version %s component must be a number", componentName)
		}
	}

	return nil
}

// ValidateCommand checks for shell injection risks.
func (v *Validator) ValidateCommand(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}

	// Check for dangerous shell metacharacters
	if v.dangerousChars.MatchString(command) {
		return fmt.Errorf("command contains potentially unsafe patterns")
	}

	// Check for path traversal
	if strings.Contains(command, "..") {
		return fmt.Errorf("command contains potentially unsafe patterns")
	}

	// Check for command chaining attempts
	suspiciousPatterns := []string{"&&", "||", ";", "\n", "\r"}
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(command, pattern) {
			return fmt.Errorf("command contains potentially unsafe patterns")
		}
	}

	return nil
}
