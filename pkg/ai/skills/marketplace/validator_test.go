package marketplace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewValidator(t *testing.T) {
	tests := []struct {
		name         string
		atmosVersion string
		expectNil    bool
	}{
		{
			name:         "valid version",
			atmosVersion: "1.0.0",
			expectNil:    false,
		},
		{
			name:         "version with prefix",
			atmosVersion: "v1.2.3",
			expectNil:    false,
		},
		{
			name:         "empty version",
			atmosVersion: "",
			expectNil:    true,
		},
		{
			name:         "invalid version",
			atmosVersion: "not-a-version",
			expectNil:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(tt.atmosVersion)
			assert.NotNil(t, v)
			if tt.expectNil {
				assert.Nil(t, v.atmosVersion)
			} else {
				assert.NotNil(t, v.atmosVersion)
			}
		})
	}
}

func TestValidate_MissingSKILLMD(t *testing.T) {
	tempDir := t.TempDir()

	v := NewValidator("1.0.0")
	metadata := &SkillMetadata{
		Name:        "test-skill",
		Description: "A test skill",
	}

	err := v.Validate(tempDir, metadata)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidMetadata))
	assert.Contains(t, err.Error(), "SKILL.md not found")
}

func TestValidate_EmptySKILLMD(t *testing.T) {
	tempDir := t.TempDir()
	skillMDPath := filepath.Join(tempDir, "SKILL.md")

	// Create empty SKILL.md.
	err := os.WriteFile(skillMDPath, []byte(""), 0o644)
	require.NoError(t, err)

	v := NewValidator("1.0.0")
	metadata := &SkillMetadata{
		Name:        "test-skill",
		Description: "A test skill",
	}

	err = v.Validate(tempDir, metadata)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrMissingPromptFile))
	assert.Contains(t, err.Error(), "SKILL.md is empty")
}

func TestValidate_ValidSkill(t *testing.T) {
	tempDir := t.TempDir()
	skillMDPath := filepath.Join(tempDir, "SKILL.md")

	content := `---
name: test-skill
description: A test skill
---

# Skill: Test Skill

This is a test skill.
`
	err := os.WriteFile(skillMDPath, []byte(content), 0o644)
	require.NoError(t, err)

	v := NewValidator("1.0.0")
	metadata := &SkillMetadata{
		Name:        "test-skill",
		Description: "A test skill",
	}

	err = v.Validate(tempDir, metadata)
	assert.NoError(t, err)
}

func TestValidate_IncompatibleVersion(t *testing.T) {
	tempDir := t.TempDir()
	skillMDPath := filepath.Join(tempDir, "SKILL.md")

	content := `---
name: test-skill
description: A test skill
---

# Skill: Test Skill

This is a test skill.
`
	err := os.WriteFile(skillMDPath, []byte(content), 0o644)
	require.NoError(t, err)

	v := NewValidator("1.0.0")
	metadata := &SkillMetadata{
		Name:        "test-skill",
		Description: "A test skill",
		Compatibility: &CompatibilityConfig{
			Atmos: ">=2.0.0",
		},
	}

	err = v.Validate(tempDir, metadata)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrIncompatibleVersion))
	assert.Contains(t, err.Error(), "skill requires Atmos >= 2.0.0")
	assert.Contains(t, err.Error(), "current version is 1.0.0")
}

func TestValidate_InvalidToolConfig(t *testing.T) {
	tempDir := t.TempDir()
	skillMDPath := filepath.Join(tempDir, "SKILL.md")

	content := `---
name: test-skill
description: A test skill
---

# Skill: Test Skill

This is a test skill.
`
	err := os.WriteFile(skillMDPath, []byte(content), 0o644)
	require.NoError(t, err)

	v := NewValidator("1.0.0")
	metadata := &SkillMetadata{
		Name:            "test-skill",
		Description:     "A test skill",
		AllowedTools:    []string{"bash", "git"},
		RestrictedTools: []string{"git", "docker"},
	}

	err = v.Validate(tempDir, metadata)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidToolConfig))
	assert.Contains(t, err.Error(), `tool "git" cannot be both allowed and restricted`)
}

func TestValidate_MissingHeading(t *testing.T) {
	tempDir := t.TempDir()
	skillMDPath := filepath.Join(tempDir, "SKILL.md")

	content := `---
name: test-skill
description: A test skill
---

This is content without a heading.
`
	err := os.WriteFile(skillMDPath, []byte(content), 0o644)
	require.NoError(t, err)

	v := NewValidator("1.0.0")
	metadata := &SkillMetadata{
		Name:        "test-skill",
		Description: "A test skill",
	}

	err = v.Validate(tempDir, metadata)
	require.Error(t, err)
	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr))
	assert.Equal(t, "prompt", validationErr.Field)
	assert.Contains(t, validationErr.Message, "should start with a level-1 heading")
}

func TestValidateVersionCompatibility_NoVersionRequirement(t *testing.T) {
	v := NewValidator("1.0.0")
	metadata := &SkillMetadata{
		Name:        "test-skill",
		Description: "A test skill",
	}

	err := v.validateVersionCompatibility(metadata)
	assert.NoError(t, err)
}

func TestValidateVersionCompatibility_InvalidMinVersion(t *testing.T) {
	v := NewValidator("1.0.0")
	metadata := &SkillMetadata{
		Name:        "test-skill",
		Description: "A test skill",
		Compatibility: &CompatibilityConfig{
			Atmos: ">=invalid-version",
		},
	}

	err := v.validateVersionCompatibility(metadata)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrIncompatibleVersion))
	assert.Contains(t, err.Error(), "invalid compatibility.atmos")
}

func TestValidateVersionCompatibility_NilAtmosVersion(t *testing.T) {
	v := NewValidator("")
	metadata := &SkillMetadata{
		Name:        "test-skill",
		Description: "A test skill",
		Compatibility: &CompatibilityConfig{
			Atmos: ">=1.0.0",
		},
	}

	// When atmosVersion is nil, version check is skipped.
	err := v.validateVersionCompatibility(metadata)
	assert.NoError(t, err)
}

func TestValidateVersionCompatibility_CompatibleVersion(t *testing.T) {
	tests := []struct {
		name         string
		atmosVersion string
		minVersion   string
		expectError  bool
	}{
		{
			name:         "exact match",
			atmosVersion: "1.0.0",
			minVersion:   ">=1.0.0",
			expectError:  false,
		},
		{
			name:         "newer than required",
			atmosVersion: "2.0.0",
			minVersion:   ">=1.0.0",
			expectError:  false,
		},
		{
			name:         "older than required",
			atmosVersion: "0.9.0",
			minVersion:   ">=1.0.0",
			expectError:  true,
		},
		{
			name:         "version without prefix",
			atmosVersion: "1.5.0",
			minVersion:   "1.0.0",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(tt.atmosVersion)
			metadata := &SkillMetadata{
				Name:        "test-skill",
				Description: "A test skill",
				Compatibility: &CompatibilityConfig{
					Atmos: tt.minVersion,
				},
			}

			err := v.validateVersionCompatibility(metadata)
			if tt.expectError {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrIncompatibleVersion))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateToolConfig_NoConflicts(t *testing.T) {
	v := NewValidator("1.0.0")

	tests := []struct {
		name            string
		allowedTools    []string
		restrictedTools []string
	}{
		{
			name:            "empty lists",
			allowedTools:    []string{},
			restrictedTools: []string{},
		},
		{
			name:            "only allowed",
			allowedTools:    []string{"bash", "git"},
			restrictedTools: []string{},
		},
		{
			name:            "only restricted",
			allowedTools:    []string{},
			restrictedTools: []string{"docker", "kubectl"},
		},
		{
			name:            "no overlap",
			allowedTools:    []string{"bash", "git"},
			restrictedTools: []string{"docker", "kubectl"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := &SkillMetadata{
				Name:            "test-skill",
				Description:     "A test skill",
				AllowedTools:    tt.allowedTools,
				RestrictedTools: tt.restrictedTools,
			}

			err := v.validateToolConfig(metadata)
			assert.NoError(t, err)
		})
	}
}

//nolint:dupl
func TestValidateToolConfig_WithConflicts(t *testing.T) {
	v := NewValidator("1.0.0")

	tests := []struct {
		name            string
		allowedTools    []string
		restrictedTools []string
		conflictTool    string
	}{
		{
			name:            "single conflict",
			allowedTools:    []string{"bash", "git"},
			restrictedTools: []string{"git"},
			conflictTool:    "git",
		},
		{
			name:            "multiple conflicts",
			allowedTools:    []string{"bash", "git", "docker"},
			restrictedTools: []string{"git", "docker", "kubectl"},
			conflictTool:    "git", // Will detect first one.
		},
		{
			name:            "same tool in both",
			allowedTools:    []string{"tool"},
			restrictedTools: []string{"tool"},
			conflictTool:    "tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := &SkillMetadata{
				Name:            "test-skill",
				Description:     "A test skill",
				AllowedTools:    tt.allowedTools,
				RestrictedTools: tt.restrictedTools,
			}

			err := v.validateToolConfig(metadata)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidToolConfig))
			assert.Contains(t, err.Error(), "cannot be both allowed and restricted")
		})
	}
}

func TestValidatePromptStructure_ValidStructure(t *testing.T) {
	tempDir := t.TempDir()
	skillMDPath := filepath.Join(tempDir, "SKILL.md")

	content := `---
name: test-skill
description: A test skill
---

# Skill: Test Skill

This is a test skill with proper structure.
`
	err := os.WriteFile(skillMDPath, []byte(content), 0o644)
	require.NoError(t, err)

	v := NewValidator("1.0.0")
	err = v.validatePromptStructure(skillMDPath)
	assert.NoError(t, err)
}

func TestValidatePromptStructure_MissingFrontmatter(t *testing.T) {
	tempDir := t.TempDir()
	skillMDPath := filepath.Join(tempDir, "SKILL.md")

	content := `# Skill: Test Skill

This file has no frontmatter.
`
	err := os.WriteFile(skillMDPath, []byte(content), 0o644)
	require.NoError(t, err)

	v := NewValidator("1.0.0")
	err = v.validatePromptStructure(skillMDPath)
	require.Error(t, err)
	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr))
	assert.Equal(t, "frontmatter", validationErr.Field)
	assert.Contains(t, validationErr.Message, "must have YAML frontmatter")
}

func TestValidatePromptStructure_IncompleteFrontmatter(t *testing.T) {
	tempDir := t.TempDir()
	skillMDPath := filepath.Join(tempDir, "SKILL.md")

	content := `---
name: test-skill
description: A test skill

# Missing closing delimiter

# Skill: Test Skill
`
	err := os.WriteFile(skillMDPath, []byte(content), 0o644)
	require.NoError(t, err)

	v := NewValidator("1.0.0")
	err = v.validatePromptStructure(skillMDPath)
	require.Error(t, err)
	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr))
	assert.Equal(t, "frontmatter", validationErr.Field)
}

func TestValidatePromptStructure_EmptyLinesAfterFrontmatter(t *testing.T) {
	tempDir := t.TempDir()
	skillMDPath := filepath.Join(tempDir, "SKILL.md")

	// Empty lines after frontmatter should be allowed.
	content := `---
name: test-skill
description: A test skill
---


# Skill: Test Skill

Content here.
`
	err := os.WriteFile(skillMDPath, []byte(content), 0o644)
	require.NoError(t, err)

	v := NewValidator("1.0.0")
	err = v.validatePromptStructure(skillMDPath)
	assert.NoError(t, err)
}

func TestValidatePromptStructure_NonHeadingContent(t *testing.T) {
	tempDir := t.TempDir()
	skillMDPath := filepath.Join(tempDir, "SKILL.md")

	content := `---
name: test-skill
description: A test skill
---

Some text that is not a heading.
`
	err := os.WriteFile(skillMDPath, []byte(content), 0o644)
	require.NoError(t, err)

	v := NewValidator("1.0.0")
	err = v.validatePromptStructure(skillMDPath)
	require.Error(t, err)
	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr))
	assert.Equal(t, "prompt", validationErr.Field)
	assert.Contains(t, validationErr.Message, "should start with a level-1 heading")
}

func TestValidatePromptStructure_FileOpenError(t *testing.T) {
	// Non-existent file.
	skillMDPath := filepath.Join(t.TempDir(), "nonexistent.md")

	v := NewValidator("1.0.0")
	err := v.validatePromptStructure(skillMDPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open SKILL.md")
}

func TestValidatePromptStructure_Level2Heading(t *testing.T) {
	tempDir := t.TempDir()
	skillMDPath := filepath.Join(tempDir, "SKILL.md")

	// Level 2 heading should fail.
	content := `---
name: test-skill
description: A test skill
---

## Skill: Test Skill

This should fail because it's not level-1.
`
	err := os.WriteFile(skillMDPath, []byte(content), 0o644)
	require.NoError(t, err)

	v := NewValidator("1.0.0")
	err = v.validatePromptStructure(skillMDPath)
	require.Error(t, err)
	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr))
	assert.Equal(t, "prompt", validationErr.Field)
}

func TestValidatePromptStructure_TripleDelimiterInContent(t *testing.T) {
	tempDir := t.TempDir()
	skillMDPath := filepath.Join(tempDir, "SKILL.md")

	// Triple delimiter in content (not at line 1) should not confuse parser.
	content := `---
name: test-skill
description: A test skill
---

# Skill: Test Skill

This content has --- in it, which should be fine.
`
	err := os.WriteFile(skillMDPath, []byte(content), 0o644)
	require.NoError(t, err)

	v := NewValidator("1.0.0")
	err = v.validatePromptStructure(skillMDPath)
	assert.NoError(t, err)
}

func TestValidate_FullIntegration(t *testing.T) {
	tempDir := t.TempDir()
	skillMDPath := filepath.Join(tempDir, "SKILL.md")

	tests := []struct {
		name        string
		content     string
		metadata    *SkillMetadata
		atmosVer    string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid skill with all features",
			content: `---
name: test-skill
description: A test skill
license: MIT
compatibility:
  atmos: ">=1.0.0"
metadata:
  version: "1.0.0"
  author: "Test Author"
allowed-tools:
  - bash
  - git
---

# Skill: Test Skill

This is a comprehensive test skill.
`,
			metadata: &SkillMetadata{
				Name:        "test-skill",
				Description: "A test skill",
				License:     "MIT",
				Compatibility: &CompatibilityConfig{
					Atmos: ">=1.0.0",
				},
				Metadata: &ExtendedMetadata{
					Version: "1.0.0",
					Author:  "Test Author",
				},
				AllowedTools: []string{"bash", "git"},
			},
			atmosVer:    "1.5.0",
			expectError: false,
		},
		{
			name: "incompatible version",
			content: `---
name: test-skill
description: A test skill
---

# Skill: Test Skill

Content.
`,
			metadata: &SkillMetadata{
				Name:        "test-skill",
				Description: "A test skill",
				Compatibility: &CompatibilityConfig{
					Atmos: ">=3.0.0",
				},
			},
			atmosVer:    "1.0.0",
			expectError: true,
			errorMsg:    "skill requires Atmos >= 3.0.0",
		},
		{
			name: "tool conflict",
			content: `---
name: test-skill
description: A test skill
---

# Skill: Test Skill

Content.
`,
			metadata: &SkillMetadata{
				Name:            "test-skill",
				Description:     "A test skill",
				AllowedTools:    []string{"bash", "git"},
				RestrictedTools: []string{"bash"},
			},
			atmosVer:    "1.0.0",
			expectError: true,
			errorMsg:    `"bash" cannot be both allowed and restricted`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.WriteFile(skillMDPath, []byte(tt.content), 0o644)
			require.NoError(t, err)

			v := NewValidator(tt.atmosVer)
			err = v.Validate(tempDir, tt.metadata)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}

			// Clean up for next iteration.
			os.Remove(skillMDPath)
		})
	}
}
