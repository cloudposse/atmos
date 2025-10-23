package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSections(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		expectedCount int
		expectedKeys  []string
		checkContent  map[string]string // key -> expected content substring
	}{
		{
			name: "parses multiple sections",
			content: `# Atmos Project Memory

> This is a description

## Project Context

This is the project context.

## Common Commands

These are common commands.

## Stack Patterns

These are stack patterns.
`,
			expectedCount: 3,
			expectedKeys:  []string{"project_context", "common_commands", "stack_patterns"},
			checkContent: map[string]string{
				"project_context": "This is the project context",
				"common_commands": "These are common commands",
				"stack_patterns":  "These are stack patterns",
			},
		},
		{
			name: "handles empty sections",
			content: `## Project Context

## Common Commands

Some content here.
`,
			expectedCount: 2,
			expectedKeys:  []string{"project_context", "common_commands"},
			checkContent: map[string]string{
				"project_context": "",
				"common_commands": "Some content here",
			},
		},
		{
			name: "preserves multiline content",
			content: `## Project Context

Line 1
Line 2
Line 3

## Common Commands

Command 1
Command 2
`,
			expectedCount: 2,
			expectedKeys:  []string{"project_context", "common_commands"},
			checkContent: map[string]string{
				"project_context": "Line 1\nLine 2\nLine 3",
				"common_commands": "Command 1\nCommand 2",
			},
		},
		{
			name:          "handles sections with code blocks",
			content:       "## Project Context\n\nSome text\n\n```yaml\nkey: value\n```\n\n## Common Commands\n\n```bash\natmos validate\n```\n",
			expectedCount: 2,
			expectedKeys:  []string{"project_context", "common_commands"},
			checkContent: map[string]string{
				"project_context": "```yaml",
				"common_commands": "```bash",
			},
		},
		{
			name:          "handles empty content",
			content:       "",
			expectedCount: 0,
			expectedKeys:  []string{},
		},
		{
			name:          "handles content without sections",
			content:       "# Title\n\nSome content without ## sections.",
			expectedCount: 0,
			expectedKeys:  []string{},
		},
		{
			name: "ignores level 1 headers",
			content: `# Atmos Project Memory

Some intro text.

## Project Context

Real section content.
`,
			expectedCount: 1,
			expectedKeys:  []string{"project_context"},
			checkContent: map[string]string{
				"project_context": "Real section content",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sections, err := ParseSections(tt.content)
			require.NoError(t, err)

			assert.Len(t, sections, tt.expectedCount)

			for _, key := range tt.expectedKeys {
				assert.Contains(t, sections, key, "Expected section key '%s' not found", key)
			}

			for key, expectedContent := range tt.checkContent {
				section, ok := sections[key]
				require.True(t, ok, "Section '%s' not found", key)
				if expectedContent == "" {
					assert.Empty(t, section.Content, "Section '%s' should be empty", key)
				} else {
					assert.Contains(t, section.Content, expectedContent, "Section '%s' content mismatch", key)
				}
			}
		})
	}
}

func TestTitleToKey(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		expected string
	}{
		{
			name:     "converts known title",
			title:    "Project Context",
			expected: "project_context",
		},
		{
			name:     "converts with ampersand",
			title:    "Frequent Issues & Solutions",
			expected: "frequent_issues",
		},
		{
			name:     "handles custom title",
			title:    "My Custom Section",
			expected: "my_custom_section",
		},
		{
			name:     "removes special characters",
			title:    "Stack Patterns (Advanced)",
			expected: "stack_patterns_advanced",
		},
		{
			name:     "converts to lowercase",
			title:    "UPPERCASE TITLE",
			expected: "uppercase_title",
		},
		{
			name:     "handles multiple spaces",
			title:    "Multiple   Spaces   Here",
			expected: "multiple_spaces_here",
		},
		{
			name:     "removes leading/trailing spaces",
			title:    "  Trimmed Title  ",
			expected: "trimmed_title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := titleToKey(tt.title)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMergeContent(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		update   string
		expected string
	}{
		{
			name:     "merges non-empty content",
			existing: "Existing content",
			update:   "New content",
			expected: "Existing content\n\nNew content",
		},
		{
			name:     "returns update when existing is empty",
			existing: "",
			update:   "New content",
			expected: "New content",
		},
		{
			name:     "returns existing when update is empty",
			existing: "Existing content",
			update:   "",
			expected: "Existing content",
		},
		{
			name:     "handles both empty",
			existing: "",
			update:   "",
			expected: "",
		},
		{
			name:     "preserves whitespace in content",
			existing: "  Existing  ",
			update:   "  Update  ",
			expected: "  Existing  \n\n  Update  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeContent(tt.existing, tt.update)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractSection(t *testing.T) {
	content := `# Atmos Project Memory

## Project Context

Project context content here.

## Common Commands

Commands content here.
`

	tests := []struct {
		name        string
		sectionKey  string
		expected    string
		expectEmpty bool
	}{
		{
			name:       "extracts existing section",
			sectionKey: "project_context",
			expected:   "Project context content here",
		},
		{
			name:       "extracts another section",
			sectionKey: "common_commands",
			expected:   "Commands content here",
		},
		{
			name:        "returns empty for non-existent section",
			sectionKey:  "non_existent",
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractSection(content, tt.sectionKey)
			require.NoError(t, err)

			if tt.expectEmpty {
				assert.Empty(t, result)
			} else {
				assert.Contains(t, result, tt.expected)
			}
		})
	}
}

func TestParseSections_SectionOrder(t *testing.T) {
	content := `## Recent Learnings

Learning content.

## Project Context

Context content.

## Common Commands

Commands content.
`

	sections, err := ParseSections(content)
	require.NoError(t, err)

	// Check that sections have proper order values.
	assert.Equal(t, SectionOrder["project_context"], sections["project_context"].Order)
	assert.Equal(t, SectionOrder["common_commands"], sections["common_commands"].Order)
	assert.Equal(t, SectionOrder["recent_learnings"], sections["recent_learnings"].Order)
}

func TestParseSections_PreservesFormatting(t *testing.T) {
	content := `## Project Context

**Bold text** and *italic text*

- Bullet 1
- Bullet 2

` + "```yaml" + `
key: value
nested:
  - item1
  - item2
` + "```" + `
`

	sections, err := ParseSections(content)
	require.NoError(t, err)

	section := sections["project_context"]
	require.NotNil(t, section)

	// Check that formatting is preserved.
	assert.Contains(t, section.Content, "**Bold text**")
	assert.Contains(t, section.Content, "*italic text*")
	assert.Contains(t, section.Content, "- Bullet 1")
	assert.Contains(t, section.Content, "```yaml")
	assert.Contains(t, section.Content, "key: value")
}

func TestTitleToKey_KnownTitles(t *testing.T) {
	// Test all known section titles map correctly.
	for key, title := range SectionTitles {
		t.Run(title, func(t *testing.T) {
			result := titleToKey(title)
			assert.Equal(t, key, result, "Title '%s' should map to key '%s'", title, key)
		})
	}
}
