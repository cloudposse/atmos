package marketplace

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

// SkillMetadata represents the YAML frontmatter in a SKILL.md file.
// This follows the Agent Skills open standard (https://agentskills.io).
type SkillMetadata struct {
	// Required fields (per Agent Skills spec).
	Name        string `yaml:"name"`
	Description string `yaml:"description"`

	// Optional: License information.
	License string `yaml:"license,omitempty"`

	// Optional: Compatibility requirements.
	Compatibility *CompatibilityConfig `yaml:"compatibility,omitempty"`

	// Optional: Extended metadata.
	Metadata *ExtendedMetadata `yaml:"metadata,omitempty"`

	// Optional: Tool access (Atmos extension).
	AllowedTools []string `yaml:"allowed-tools,omitempty"`

	// Atmos-specific extension: Restricted tools (not in standard).
	RestrictedTools []string `yaml:"restricted-tools,omitempty"`
}

// CompatibilityConfig defines version requirements.
type CompatibilityConfig struct {
	Atmos string `yaml:"atmos,omitempty"` // e.g., ">=1.0.0".
}

// ExtendedMetadata contains optional extended information.
type ExtendedMetadata struct {
	DisplayName string `yaml:"display_name,omitempty"`
	Version     string `yaml:"version,omitempty"`
	Author      string `yaml:"author,omitempty"`
	Category    string `yaml:"category,omitempty"`
	Repository  string `yaml:"repository,omitempty"`
}

// ParseSkillMetadata reads and parses a SKILL.md file's frontmatter.
func ParseSkillMetadata(path string) (*SkillMetadata, error) {
	defer perf.Track(nil, "marketplace.ParseSkillMetadata")()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read SKILL.md file: %w", err)
	}

	// Extract YAML frontmatter.
	frontmatter, err := extractFrontmatter(string(data))
	if err != nil {
		return nil, fmt.Errorf("failed to extract frontmatter: %w", err)
	}

	var metadata SkillMetadata
	if err := yaml.Unmarshal([]byte(frontmatter), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse YAML frontmatter: %w", err)
	}

	// Basic validation.
	if err := validateSkillMetadata(&metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

// extractFrontmatter extracts the YAML frontmatter from a SKILL.md file.
func extractFrontmatter(content string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var lines []string
	inFrontmatter := false
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// Check for frontmatter delimiter.
		if strings.TrimSpace(line) == "---" {
			if !inFrontmatter && lineNum == 1 {
				// Start of frontmatter.
				inFrontmatter = true
				continue
			} else if inFrontmatter {
				// End of frontmatter.
				break
			}
		}

		// Collect lines in frontmatter.
		if inFrontmatter {
			lines = append(lines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error scanning content: %w", err)
	}

	if len(lines) == 0 {
		return "", fmt.Errorf("no YAML frontmatter found (file must start with ---)")
	}

	return strings.Join(lines, "\n"), nil
}

// validateSkillMetadata performs basic validation on skill metadata.
func validateSkillMetadata(m *SkillMetadata) error {
	// Required by Agent Skills spec.
	if m.Name == "" {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	if m.Description == "" {
		return &ValidationError{Field: "description", Message: "description is required"}
	}

	// Optional: Validate category if provided.
	if m.Metadata != nil && m.Metadata.Category != "" {
		validCategories := map[string]bool{
			"general":      true,
			"analysis":     true,
			"refactor":     true,
			"security":     true,
			"validation":   true,
			"optimization": true,
		}
		if !validCategories[m.Metadata.Category] {
			return &ValidationError{
				Field:   "metadata.category",
				Message: fmt.Sprintf("invalid category %q (must be one of: general, analysis, refactor, security, validation, optimization)", m.Metadata.Category),
			}
		}
	}

	return nil
}

// GetDisplayName returns the display name, falling back to name if not set.
func (m *SkillMetadata) GetDisplayName() string {
	if m.Metadata != nil && m.Metadata.DisplayName != "" {
		return m.Metadata.DisplayName
	}
	return m.Name
}

// GetVersion returns the version, falling back to "0.0.0" if not set.
func (m *SkillMetadata) GetVersion() string {
	if m.Metadata != nil && m.Metadata.Version != "" {
		return m.Metadata.Version
	}
	return "0.0.0"
}

// GetCategory returns the category, falling back to "general" if not set.
func (m *SkillMetadata) GetCategory() string {
	if m.Metadata != nil && m.Metadata.Category != "" {
		return m.Metadata.Category
	}
	return "general"
}

// GetAuthor returns the author, or empty string if not set.
func (m *SkillMetadata) GetAuthor() string {
	if m.Metadata != nil {
		return m.Metadata.Author
	}
	return ""
}

// GetRepository returns the repository URL, or empty string if not set.
func (m *SkillMetadata) GetRepository() string {
	if m.Metadata != nil {
		return m.Metadata.Repository
	}
	return ""
}

// GetMinAtmosVersion extracts the minimum version from compatibility string.
// Compatibility format: ">=1.0.0" returns "1.0.0".
func (m *SkillMetadata) GetMinAtmosVersion() string {
	if m.Compatibility == nil || m.Compatibility.Atmos == "" {
		return ""
	}

	// Parse common formats: ">=1.0.0", ">1.0.0", "1.0.0".
	ver := m.Compatibility.Atmos
	ver = strings.TrimPrefix(ver, ">=")
	ver = strings.TrimPrefix(ver, ">")
	ver = strings.TrimSpace(ver)

	return ver
}
