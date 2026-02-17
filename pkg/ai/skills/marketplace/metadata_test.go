package marketplace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseSkillMetadata_ValidFile tests parsing a valid SKILL.md file.
func TestParseSkillMetadata_ValidFile(t *testing.T) {
	content := `---
name: test-skill
description: A test skill for validation
license: MIT
compatibility:
  atmos: ">=1.0.0"
metadata:
  display_name: Test Skill
  version: 1.2.3
  author: Test Author
  category: analysis
  repository: https://github.com/example/test-skill
allowed-tools:
  - read
  - write
restricted-tools:
  - delete
---

# Test Skill

This is the skill content.
`

	tmpDir := t.TempDir()
	skillFile := filepath.Join(tmpDir, "SKILL.md")
	err := os.WriteFile(skillFile, []byte(content), 0o644)
	require.NoError(t, err)

	metadata, err := ParseSkillMetadata(skillFile)
	require.NoError(t, err)
	assert.NotNil(t, metadata)

	assert.Equal(t, "test-skill", metadata.Name)
	assert.Equal(t, "A test skill for validation", metadata.Description)
	assert.Equal(t, "MIT", metadata.License)

	require.NotNil(t, metadata.Compatibility)
	assert.Equal(t, ">=1.0.0", metadata.Compatibility.Atmos)

	require.NotNil(t, metadata.Metadata)
	assert.Equal(t, "Test Skill", metadata.Metadata.DisplayName)
	assert.Equal(t, "1.2.3", metadata.Metadata.Version)
	assert.Equal(t, "Test Author", metadata.Metadata.Author)
	assert.Equal(t, "analysis", metadata.Metadata.Category)
	assert.Equal(t, "https://github.com/example/test-skill", metadata.Metadata.Repository)

	assert.Equal(t, []string{"read", "write"}, metadata.AllowedTools)
	assert.Equal(t, []string{"delete"}, metadata.RestrictedTools)
}

// TestParseSkillMetadata_MinimalFile tests parsing a file with only required fields.
func TestParseSkillMetadata_MinimalFile(t *testing.T) {
	content := `---
name: minimal-skill
description: Minimal skill with required fields only
---

# Minimal Skill
`

	tmpDir := t.TempDir()
	skillFile := filepath.Join(tmpDir, "SKILL.md")
	err := os.WriteFile(skillFile, []byte(content), 0o644)
	require.NoError(t, err)

	metadata, err := ParseSkillMetadata(skillFile)
	require.NoError(t, err)
	assert.NotNil(t, metadata)

	assert.Equal(t, "minimal-skill", metadata.Name)
	assert.Equal(t, "Minimal skill with required fields only", metadata.Description)
	assert.Empty(t, metadata.License)
	assert.Nil(t, metadata.Compatibility)
	assert.Nil(t, metadata.Metadata)
	assert.Nil(t, metadata.AllowedTools)
	assert.Nil(t, metadata.RestrictedTools)
}

// TestParseSkillMetadata_FileNotFound tests error when file doesn't exist.
func TestParseSkillMetadata_FileNotFound(t *testing.T) {
	// Use cross-platform non-existent path.
	nonExistentPath := filepath.Join(t.TempDir(), "nonexistent", "SKILL.md")
	metadata, err := ParseSkillMetadata(nonExistentPath)
	assert.Error(t, err)
	assert.Nil(t, metadata)
	assert.Contains(t, err.Error(), "failed to read SKILL.md file")
}

// TestParseSkillMetadata_NoFrontmatter tests error when frontmatter is missing.
func TestParseSkillMetadata_NoFrontmatter(t *testing.T) {
	content := `# Test Skill

This file has no frontmatter.
`

	tmpDir := t.TempDir()
	skillFile := filepath.Join(tmpDir, "SKILL.md")
	err := os.WriteFile(skillFile, []byte(content), 0o644)
	require.NoError(t, err)

	metadata, err := ParseSkillMetadata(skillFile)
	assert.Error(t, err)
	assert.Nil(t, metadata)
	assert.Contains(t, err.Error(), "failed to extract frontmatter")
	assert.Contains(t, err.Error(), "no YAML frontmatter found")
}

// TestParseSkillMetadata_InvalidYAML tests error with malformed YAML.
func TestParseSkillMetadata_InvalidYAML(t *testing.T) {
	content := `---
name: test
description: [invalid yaml structure
---

# Test
`

	tmpDir := t.TempDir()
	skillFile := filepath.Join(tmpDir, "SKILL.md")
	err := os.WriteFile(skillFile, []byte(content), 0o644)
	require.NoError(t, err)

	metadata, err := ParseSkillMetadata(skillFile)
	assert.Error(t, err)
	assert.Nil(t, metadata)
	assert.Contains(t, err.Error(), "failed to parse YAML frontmatter")
}

// TestParseSkillMetadata_MissingName tests validation error for missing name.
func TestParseSkillMetadata_MissingName(t *testing.T) {
	content := `---
description: Missing name field
---

# Test
`

	tmpDir := t.TempDir()
	skillFile := filepath.Join(tmpDir, "SKILL.md")
	err := os.WriteFile(skillFile, []byte(content), 0o644)
	require.NoError(t, err)

	metadata, err := ParseSkillMetadata(skillFile)
	assert.Error(t, err)
	assert.Nil(t, metadata)

	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr))
	assert.Equal(t, "name", validationErr.Field)
	assert.Contains(t, validationErr.Message, "name is required")
}

// TestParseSkillMetadata_MissingDescription tests validation error for missing description.
func TestParseSkillMetadata_MissingDescription(t *testing.T) {
	content := `---
name: test-skill
---

# Test
`

	tmpDir := t.TempDir()
	skillFile := filepath.Join(tmpDir, "SKILL.md")
	err := os.WriteFile(skillFile, []byte(content), 0o644)
	require.NoError(t, err)

	metadata, err := ParseSkillMetadata(skillFile)
	assert.Error(t, err)
	assert.Nil(t, metadata)

	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr))
	assert.Equal(t, "description", validationErr.Field)
	assert.Contains(t, validationErr.Message, "description is required")
}

// TestParseSkillMetadata_InvalidCategory tests validation error for invalid category.
func TestParseSkillMetadata_InvalidCategory(t *testing.T) {
	content := `---
name: test-skill
description: Test skill
metadata:
  category: invalid-category
---

# Test
`

	tmpDir := t.TempDir()
	skillFile := filepath.Join(tmpDir, "SKILL.md")
	err := os.WriteFile(skillFile, []byte(content), 0o644)
	require.NoError(t, err)

	metadata, err := ParseSkillMetadata(skillFile)
	assert.Error(t, err)
	assert.Nil(t, metadata)

	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr))
	assert.Equal(t, "metadata.category", validationErr.Field)
	assert.Contains(t, validationErr.Message, "invalid category")
}

// TestParseSkillMetadata_ValidCategories tests all valid categories.
func TestParseSkillMetadata_ValidCategories(t *testing.T) {
	validCategories := []string{"general", "analysis", "refactor", "security", "validation", "optimization"}

	for _, category := range validCategories {
		t.Run(category, func(t *testing.T) {
			content := `---
name: test-skill
description: Test skill
metadata:
  category: ` + category + `
---

# Test
`

			tmpDir := t.TempDir()
			skillFile := filepath.Join(tmpDir, "SKILL.md")
			err := os.WriteFile(skillFile, []byte(content), 0o644)
			require.NoError(t, err)

			metadata, err := ParseSkillMetadata(skillFile)
			require.NoError(t, err)
			assert.NotNil(t, metadata)
			assert.Equal(t, category, metadata.Metadata.Category)
		})
	}
}

// TestExtractFrontmatter_ValidFrontmatter tests extracting valid frontmatter.
func TestExtractFrontmatter_ValidFrontmatter(t *testing.T) {
	content := `---
name: test
description: Test description
---

# Content
`

	frontmatter, err := extractFrontmatter(content)
	require.NoError(t, err)
	assert.Contains(t, frontmatter, "name: test")
	assert.Contains(t, frontmatter, "description: Test description")
}

// TestExtractFrontmatter_WithWhitespace tests frontmatter with surrounding whitespace.
func TestExtractFrontmatter_WithWhitespace(t *testing.T) {
	content := `---
  name: test
  description: Test
---

# Content
`

	frontmatter, err := extractFrontmatter(content)
	require.NoError(t, err)
	assert.NotEmpty(t, frontmatter)
}

// TestExtractFrontmatter_NoStartDelimiter tests error when frontmatter doesn't start with ---.
func TestExtractFrontmatter_NoStartDelimiter(t *testing.T) {
	content := `name: test
description: Test
---

# Content
`

	frontmatter, err := extractFrontmatter(content)
	assert.Error(t, err)
	assert.Empty(t, frontmatter)
	assert.Contains(t, err.Error(), "no YAML frontmatter found")
}

// TestExtractFrontmatter_NoEndDelimiter tests extracting frontmatter without closing delimiter.
// The function extracts everything until EOF if no closing delimiter is found.
func TestExtractFrontmatter_NoEndDelimiter(t *testing.T) {
	content := `---
name: test
description: Test

# Content without closing delimiter
`

	frontmatter, err := extractFrontmatter(content)
	require.NoError(t, err)
	assert.NotEmpty(t, frontmatter)
	assert.Contains(t, frontmatter, "name: test")
	assert.Contains(t, frontmatter, "description: Test")
}

// TestExtractFrontmatter_EmptyFrontmatter tests error with empty frontmatter.
func TestExtractFrontmatter_EmptyFrontmatter(t *testing.T) {
	content := `---
---

# Content
`

	frontmatter, err := extractFrontmatter(content)
	assert.Error(t, err)
	assert.Empty(t, frontmatter)
	assert.Contains(t, err.Error(), "no YAML frontmatter found")
}

// TestExtractFrontmatter_MultipleDelimiters tests frontmatter with content after closing.
func TestExtractFrontmatter_MultipleDelimiters(t *testing.T) {
	content := `---
name: test
description: Test
---

# Content

---
This should not be included in frontmatter.
---
`

	frontmatter, err := extractFrontmatter(content)
	require.NoError(t, err)
	assert.Contains(t, frontmatter, "name: test")
	assert.NotContains(t, frontmatter, "This should not be included")
}

// TestValidateSkillMetadata_ValidMetadata tests validation of valid metadata.
func TestValidateSkillMetadata_ValidMetadata(t *testing.T) {
	metadata := &SkillMetadata{
		Name:        "test-skill",
		Description: "Test description",
	}

	err := validateSkillMetadata(metadata)
	assert.NoError(t, err)
}

// TestValidateSkillMetadata_EmptyName tests validation fails with empty name.
func TestValidateSkillMetadata_EmptyName(t *testing.T) {
	metadata := &SkillMetadata{
		Name:        "",
		Description: "Test description",
	}

	err := validateSkillMetadata(metadata)
	assert.Error(t, err)

	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr))
	assert.Equal(t, "name", validationErr.Field)
}

// TestValidateSkillMetadata_EmptyDescription tests validation fails with empty description.
func TestValidateSkillMetadata_EmptyDescription(t *testing.T) {
	metadata := &SkillMetadata{
		Name:        "test-skill",
		Description: "",
	}

	err := validateSkillMetadata(metadata)
	assert.Error(t, err)

	var validationErr *ValidationError
	assert.True(t, errors.As(err, &validationErr))
	assert.Equal(t, "description", validationErr.Field)
}

// TestGetDisplayName tests the GetDisplayName method.
func TestGetDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		metadata *SkillMetadata
		expected string
	}{
		{
			name: "with display name",
			metadata: &SkillMetadata{
				Name: "test-skill",
				Metadata: &ExtendedMetadata{
					DisplayName: "Test Skill",
				},
			},
			expected: "Test Skill",
		},
		{
			name: "without display name",
			metadata: &SkillMetadata{
				Name: "test-skill",
			},
			expected: "test-skill",
		},
		{
			name: "with empty display name",
			metadata: &SkillMetadata{
				Name: "test-skill",
				Metadata: &ExtendedMetadata{
					DisplayName: "",
				},
			},
			expected: "test-skill",
		},
		{
			name: "with metadata but no display name",
			metadata: &SkillMetadata{
				Name: "test-skill",
				Metadata: &ExtendedMetadata{
					Version: "1.0.0",
				},
			},
			expected: "test-skill",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.metadata.GetDisplayName()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetVersion tests the GetVersion method.
func TestGetVersion(t *testing.T) {
	tests := []struct {
		name     string
		metadata *SkillMetadata
		expected string
	}{
		{
			name: "with version",
			metadata: &SkillMetadata{
				Metadata: &ExtendedMetadata{
					Version: "1.2.3",
				},
			},
			expected: "1.2.3",
		},
		{
			name:     "without metadata",
			metadata: &SkillMetadata{},
			expected: "0.0.0",
		},
		{
			name: "with empty version",
			metadata: &SkillMetadata{
				Metadata: &ExtendedMetadata{
					Version: "",
				},
			},
			expected: "0.0.0",
		},
		{
			name: "with metadata but no version",
			metadata: &SkillMetadata{
				Metadata: &ExtendedMetadata{
					Author: "Test Author",
				},
			},
			expected: "0.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.metadata.GetVersion()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetCategory tests the GetCategory method.
func TestGetCategory(t *testing.T) {
	tests := []struct {
		name     string
		metadata *SkillMetadata
		expected string
	}{
		{
			name: "with category",
			metadata: &SkillMetadata{
				Metadata: &ExtendedMetadata{
					Category: "analysis",
				},
			},
			expected: "analysis",
		},
		{
			name:     "without metadata",
			metadata: &SkillMetadata{},
			expected: "general",
		},
		{
			name: "with empty category",
			metadata: &SkillMetadata{
				Metadata: &ExtendedMetadata{
					Category: "",
				},
			},
			expected: "general",
		},
		{
			name: "with metadata but no category",
			metadata: &SkillMetadata{
				Metadata: &ExtendedMetadata{
					Author: "Test Author",
				},
			},
			expected: "general",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.metadata.GetCategory()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetAuthor tests the GetAuthor method.
func TestGetAuthor(t *testing.T) {
	tests := []struct {
		name     string
		metadata *SkillMetadata
		expected string
	}{
		{
			name: "with author",
			metadata: &SkillMetadata{
				Metadata: &ExtendedMetadata{
					Author: "Test Author",
				},
			},
			expected: "Test Author",
		},
		{
			name:     "without metadata",
			metadata: &SkillMetadata{},
			expected: "",
		},
		{
			name: "with empty author",
			metadata: &SkillMetadata{
				Metadata: &ExtendedMetadata{
					Author: "",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.metadata.GetAuthor()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetRepository tests the GetRepository method.
func TestGetRepository(t *testing.T) {
	tests := []struct {
		name     string
		metadata *SkillMetadata
		expected string
	}{
		{
			name: "with repository",
			metadata: &SkillMetadata{
				Metadata: &ExtendedMetadata{
					Repository: "https://github.com/example/skill",
				},
			},
			expected: "https://github.com/example/skill",
		},
		{
			name:     "without metadata",
			metadata: &SkillMetadata{},
			expected: "",
		},
		{
			name: "with empty repository",
			metadata: &SkillMetadata{
				Metadata: &ExtendedMetadata{
					Repository: "",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.metadata.GetRepository()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetMinAtmosVersion tests the GetMinAtmosVersion method.
func TestGetMinAtmosVersion(t *testing.T) {
	tests := []struct {
		name     string
		metadata *SkillMetadata
		expected string
	}{
		{
			name: "with >= prefix",
			metadata: &SkillMetadata{
				Compatibility: &CompatibilityConfig{
					Atmos: ">=1.2.3",
				},
			},
			expected: "1.2.3",
		},
		{
			name: "with > prefix",
			metadata: &SkillMetadata{
				Compatibility: &CompatibilityConfig{
					Atmos: ">2.0.0",
				},
			},
			expected: "2.0.0",
		},
		{
			name: "without prefix",
			metadata: &SkillMetadata{
				Compatibility: &CompatibilityConfig{
					Atmos: "1.0.0",
				},
			},
			expected: "1.0.0",
		},
		{
			name: "with spaces",
			metadata: &SkillMetadata{
				Compatibility: &CompatibilityConfig{
					Atmos: ">= 1.5.0",
				},
			},
			expected: "1.5.0",
		},
		{
			name:     "without compatibility",
			metadata: &SkillMetadata{},
			expected: "",
		},
		{
			name: "with empty atmos version",
			metadata: &SkillMetadata{
				Compatibility: &CompatibilityConfig{
					Atmos: "",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.metadata.GetMinAtmosVersion()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseSkillMetadata_ComplexScenarios tests complex edge cases.
func TestParseSkillMetadata_ComplexScenarios(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expectError bool
		validate    func(t *testing.T, metadata *SkillMetadata)
	}{
		{
			name: "with all optional fields populated",
			content: `---
name: full-skill
description: Full skill with all fields
license: Apache-2.0
compatibility:
  atmos: ">=1.0.0"
metadata:
  display_name: Full Skill
  version: 2.0.0
  author: Full Author
  category: security
  repository: https://github.com/example/full
allowed-tools:
  - read
  - write
  - execute
restricted-tools:
  - delete
  - format
---

# Full Skill
`,
			expectError: false,
			validate: func(t *testing.T, metadata *SkillMetadata) {
				assert.Equal(t, "full-skill", metadata.Name)
				assert.Equal(t, "Full skill with all fields", metadata.Description)
				assert.Equal(t, "Apache-2.0", metadata.License)
				assert.Equal(t, ">=1.0.0", metadata.Compatibility.Atmos)
				assert.Equal(t, "Full Skill", metadata.Metadata.DisplayName)
				assert.Equal(t, "2.0.0", metadata.Metadata.Version)
				assert.Equal(t, "Full Author", metadata.Metadata.Author)
				assert.Equal(t, "security", metadata.Metadata.Category)
				assert.Equal(t, "https://github.com/example/full", metadata.Metadata.Repository)
				assert.Len(t, metadata.AllowedTools, 3)
				assert.Len(t, metadata.RestrictedTools, 2)
			},
		},
		{
			name: "frontmatter at start with extra content",
			content: `---
name: test
description: Test
---

# Skill Content

Some markdown content here.

## Section 2

More content.
`,
			expectError: false,
			validate: func(t *testing.T, metadata *SkillMetadata) {
				assert.Equal(t, "test", metadata.Name)
				assert.Equal(t, "Test", metadata.Description)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			skillFile := filepath.Join(tmpDir, "SKILL.md")
			err := os.WriteFile(skillFile, []byte(tt.content), 0o644)
			require.NoError(t, err)

			metadata, err := ParseSkillMetadata(skillFile)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, metadata)
			} else {
				require.NoError(t, err)
				require.NotNil(t, metadata)
				if tt.validate != nil {
					tt.validate(t, metadata)
				}
			}
		})
	}
}
