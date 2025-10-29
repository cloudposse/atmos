package marketplace

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/go-version"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Validator validates agent packages against various rules.
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

// Validate performs comprehensive validation of an agent package.
func (v *Validator) Validate(agentPath string, metadata *AgentMetadata) error {
	defer perf.Track(nil, "marketplace.Validator.Validate")()

	// 1. Validate metadata file exists.
	metadataPath := filepath.Join(agentPath, ".agent.yaml")
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		return fmt.Errorf("%w: .agent.yaml not found", ErrInvalidMetadata)
	}

	// 2. Validate prompt file exists.
	promptPath := filepath.Join(agentPath, metadata.Prompt.File)
	if _, err := os.Stat(promptPath); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s not found", ErrMissingPromptFile, metadata.Prompt.File)
	}

	// 3. Validate prompt file is not empty.
	stat, err := os.Stat(promptPath)
	if err != nil {
		return fmt.Errorf("%w: failed to stat prompt file: %w", ErrMissingPromptFile, err)
	}
	if stat.Size() == 0 {
		return fmt.Errorf("%w: prompt file is empty", ErrMissingPromptFile)
	}

	// 4. Validate Atmos version compatibility.
	if err := v.validateVersionCompatibility(metadata); err != nil {
		return err
	}

	// 5. Validate tool configuration.
	if err := v.validateToolConfig(metadata); err != nil {
		return err
	}

	// 6. Validate prompt structure (basic checks).
	if err := v.validatePromptStructure(promptPath); err != nil {
		return err
	}

	return nil
}

// validateVersionCompatibility checks if agent is compatible with current Atmos version.
func (v *Validator) validateVersionCompatibility(metadata *AgentMetadata) error {
	// Parse minimum version.
	minVer, err := version.NewVersion(metadata.Atmos.MinVersion)
	if err != nil {
		return fmt.Errorf("%w: invalid min_version %q: %w", ErrIncompatibleVersion, metadata.Atmos.MinVersion, err)
	}

	// Check minimum version.
	if v.atmosVersion != nil && v.atmosVersion.LessThan(minVer) {
		return fmt.Errorf(
			"%w: agent requires Atmos >= %s, but current version is %s",
			ErrIncompatibleVersion,
			metadata.Atmos.MinVersion,
			v.atmosVersion.String(),
		)
	}

	// Check maximum version if specified.
	if metadata.Atmos.MaxVersion != "" {
		maxVer, err := version.NewVersion(metadata.Atmos.MaxVersion)
		if err != nil {
			return fmt.Errorf("%w: invalid max_version %q: %w", ErrIncompatibleVersion, metadata.Atmos.MaxVersion, err)
		}

		if v.atmosVersion != nil && v.atmosVersion.GreaterThan(maxVer) {
			return fmt.Errorf(
				"%w: agent requires Atmos <= %s, but current version is %s",
				ErrIncompatibleVersion,
				metadata.Atmos.MaxVersion,
				v.atmosVersion.String(),
			)
		}
	}

	return nil
}

// validateToolConfig validates tool access configuration.
func (v *Validator) validateToolConfig(metadata *AgentMetadata) error {
	if metadata.Tools == nil {
		return nil // No tool config is valid.
	}

	// Check for conflicts (tool in both allowed and restricted).
	allowedMap := make(map[string]bool)
	for _, tool := range metadata.Tools.Allowed {
		allowedMap[tool] = true
	}

	for _, tool := range metadata.Tools.Restricted {
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

// validatePromptStructure performs basic validation on prompt file structure.
func (v *Validator) validatePromptStructure(promptPath string) error {
	content, err := os.ReadFile(promptPath)
	if err != nil {
		return fmt.Errorf("failed to read prompt file: %w", err)
	}

	// Check for required sections (basic Markdown structure).
	promptStr := string(content)

	// Should start with a level-1 heading.
	if len(promptStr) < 2 || promptStr[0] != '#' || promptStr[1] != ' ' {
		return &ValidationError{
			Field:   "prompt",
			Message: "prompt should start with level-1 heading (# Agent: Name)",
		}
	}

	// Warn if prompt is too large (> 100KB).
	// TODO: Add warning system for prompts > 100KB.

	return nil
}
