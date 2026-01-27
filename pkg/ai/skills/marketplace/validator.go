package marketplace

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-version"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Validator validates skill packages against various rules.
type Validator struct {
	atmosVersion *version.Version
}

// NewValidator creates a new validator for the given Atmos version.
func NewValidator(atmosVersion string) *Validator {
	v, _ := version.NewVersion(atmosVersion)
	return &Validator{
		atmosVersion: v,
	}
}

// Validate performs comprehensive validation of a skill package.
func (v *Validator) Validate(skillPath string, metadata *SkillMetadata) error {
	defer perf.Track(nil, "marketplace.Validator.Validate")()

	// 1. Validate SKILL.md file exists.
	skillMDPath := filepath.Join(skillPath, "SKILL.md")
	if _, err := os.Stat(skillMDPath); os.IsNotExist(err) {
		return fmt.Errorf("%w: SKILL.md not found", ErrInvalidMetadata)
	}

	// 2. Validate SKILL.md is not empty.
	stat, err := os.Stat(skillMDPath)
	if err != nil {
		return fmt.Errorf("%w: failed to stat SKILL.md: %w", ErrMissingPromptFile, err)
	}
	if stat.Size() == 0 {
		return fmt.Errorf("%w: SKILL.md is empty", ErrMissingPromptFile)
	}

	// 3. Validate Atmos version compatibility.
	if err := v.validateVersionCompatibility(metadata); err != nil {
		return err
	}

	// 4. Validate tool configuration.
	if err := v.validateToolConfig(metadata); err != nil {
		return err
	}

	// 5. Validate prompt structure (basic checks).
	if err := v.validatePromptStructure(skillMDPath); err != nil {
		return err
	}

	return nil
}

// validateVersionCompatibility checks if skill is compatible with current Atmos version.
func (v *Validator) validateVersionCompatibility(metadata *SkillMetadata) error {
	minVerStr := metadata.GetMinAtmosVersion()
	if minVerStr == "" {
		return nil // No version requirement.
	}

	// Parse minimum version.
	minVer, err := version.NewVersion(minVerStr)
	if err != nil {
		return fmt.Errorf("%w: invalid compatibility.atmos %q: %w", ErrIncompatibleVersion, minVerStr, err)
	}

	// Check minimum version.
	if v.atmosVersion != nil && v.atmosVersion.LessThan(minVer) {
		return fmt.Errorf(
			"%w: skill requires Atmos >= %s, but current version is %s",
			ErrIncompatibleVersion,
			minVerStr,
			v.atmosVersion.String(),
		)
	}

	return nil
}

// validateToolConfig validates tool access configuration.
func (v *Validator) validateToolConfig(metadata *SkillMetadata) error {
	// Check for conflicts (tool in both allowed and restricted).
	allowedMap := make(map[string]bool)
	for _, tool := range metadata.AllowedTools {
		allowedMap[tool] = true
	}

	for _, tool := range metadata.RestrictedTools {
		if allowedMap[tool] {
			return fmt.Errorf(
				"%w: tool %q cannot be both allowed and restricted",
				ErrInvalidToolConfig,
				tool,
			)
		}
	}

	return nil
}

// validatePromptStructure performs basic validation on SKILL.md structure.
func (v *Validator) validatePromptStructure(skillMDPath string) error {
	file, err := os.Open(skillMDPath)
	if err != nil {
		return fmt.Errorf("failed to open SKILL.md: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	inFrontmatter := false
	frontmatterEnded := false
	hasHeading := false

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// Check for frontmatter delimiter.
		if strings.TrimSpace(line) == "---" {
			if !inFrontmatter && lineNum == 1 {
				inFrontmatter = true
				continue
			} else if inFrontmatter {
				inFrontmatter = false
				frontmatterEnded = true
				continue
			}
		}

		// After frontmatter, look for heading.
		if frontmatterEnded && !hasHeading {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue // Skip empty lines.
			}
			if strings.HasPrefix(trimmed, "# ") {
				hasHeading = true
			} else {
				return &ValidationError{
					Field:   "prompt",
					Message: "SKILL.md content should start with a level-1 heading (# Skill: Name) after frontmatter",
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading SKILL.md: %w", err)
	}

	if !frontmatterEnded {
		return &ValidationError{
			Field:   "frontmatter",
			Message: "SKILL.md must have YAML frontmatter (content between --- delimiters)",
		}
	}

	return nil
}
