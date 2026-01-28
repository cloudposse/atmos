package builtin

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRead(t *testing.T) {
	t.Run("reads valid SKILL.md file with frontmatter", func(t *testing.T) {
		body, err := Read("general/SKILL.md")
		require.NoError(t, err)
		assert.NotEmpty(t, body)

		// Should contain markdown body but not frontmatter.
		assert.Contains(t, body, "# Skill: General")
		assert.NotContains(t, body, "---")
		assert.NotContains(t, body, "name: general")
		assert.NotContains(t, body, "description:")
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		_, err := Read("nonexistent/SKILL.md")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read embedded prompt file")
		assert.Contains(t, err.Error(), "nonexistent/SKILL.md")
	})

	t.Run("returns error for file outside embedded directory", func(t *testing.T) {
		_, err := Read("../../../README.md")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read embedded prompt file")
	})

	t.Run("handles multiple skill files", func(t *testing.T) {
		testCases := []struct {
			name     string
			filename string
		}{
			{"general skill", "general/SKILL.md"},
			{"stack-analyzer skill", "stack-analyzer/SKILL.md"},
			{"component-refactor skill", "component-refactor/SKILL.md"},
			{"config-validator skill", "config-validator/SKILL.md"},
			{"security-auditor skill", "security-auditor/SKILL.md"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				body, err := Read(tc.filename)
				require.NoError(t, err)
				assert.NotEmpty(t, body)

				// Each skill should have meaningful content.
				assert.Contains(t, body, "# Skill:")
				// Should not contain frontmatter delimiters.
				lines := strings.Split(body, "\n")
				for _, line := range lines {
					if strings.TrimSpace(line) == "---" {
						t.Errorf("Body should not contain frontmatter delimiter '---'")
					}
				}
			})
		}
	})
}

func TestReadMetadata(t *testing.T) {
	t.Run("reads frontmatter from valid SKILL.md file", func(t *testing.T) {
		metadata, err := ReadMetadata("general/SKILL.md")
		require.NoError(t, err)
		assert.NotEmpty(t, metadata)

		// Should contain frontmatter fields.
		assert.Contains(t, metadata, "name: general")
		assert.Contains(t, metadata, "description:")
		assert.Contains(t, metadata, "license:")
		assert.Contains(t, metadata, "metadata:")

		// Should not contain markdown body.
		assert.NotContains(t, metadata, "# Skill:")
		assert.NotContains(t, metadata, "## Role")
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		_, err := ReadMetadata("nonexistent/SKILL.md")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read embedded prompt file")
		assert.Contains(t, err.Error(), "nonexistent/SKILL.md")
	})

	t.Run("extracts metadata from all skill files", func(t *testing.T) {
		testCases := []string{
			"general/SKILL.md",
			"stack-analyzer/SKILL.md",
			"component-refactor/SKILL.md",
			"config-validator/SKILL.md",
			"security-auditor/SKILL.md",
		}

		for _, filename := range testCases {
			t.Run(filename, func(t *testing.T) {
				metadata, err := ReadMetadata(filename)
				require.NoError(t, err)
				assert.NotEmpty(t, metadata)

				// Every skill should have core metadata fields.
				assert.Contains(t, metadata, "name:")
				assert.Contains(t, metadata, "description:")
			})
		}
	})
}

func TestList(t *testing.T) {
	t.Run("lists all embedded SKILL.md files", func(t *testing.T) {
		files, err := List()
		require.NoError(t, err)
		assert.NotEmpty(t, files)

		// Should contain known skill files.
		expectedSkills := []string{
			"general/SKILL.md",
			"stack-analyzer/SKILL.md",
			"component-refactor/SKILL.md",
			"config-validator/SKILL.md",
			"security-auditor/SKILL.md",
		}

		for _, expected := range expectedSkills {
			assert.Contains(t, files, expected, "List() should include %s", expected)
		}
	})

	t.Run("only lists SKILL.md files in subdirectories", func(t *testing.T) {
		files, err := List()
		require.NoError(t, err)

		// All files should end with /SKILL.md.
		for _, file := range files {
			assert.True(t, strings.HasSuffix(file, "/SKILL.md"), "File %s should end with /SKILL.md", file)

			// All files should be in a subdirectory (not root).
			assert.True(t, strings.Contains(file, "/"), "File %s should be in a subdirectory", file)
		}
	})

	t.Run("returns valid skill count", func(t *testing.T) {
		files, err := List()
		require.NoError(t, err)
		// We know there are at least 5 built-in skills.
		assert.GreaterOrEqual(t, len(files), 5, "Should have at least 5 built-in skills")
	})
}

func TestExtractMarkdownBody(t *testing.T) {
	t.Run("extracts body from content with frontmatter", func(t *testing.T) {
		content := `---
name: test
description: Test skill
---

# Test Content

This is the body.`

		body, err := extractMarkdownBody(content)
		require.NoError(t, err)
		assert.Equal(t, "# Test Content\n\nThis is the body.", body)
	})

	t.Run("returns content as-is when no frontmatter", func(t *testing.T) {
		content := `# Test Content

This is the body without frontmatter.`

		body, err := extractMarkdownBody(content)
		require.NoError(t, err)
		assert.Equal(t, content, body)
	})

	t.Run("handles empty content", func(t *testing.T) {
		body, err := extractMarkdownBody("")
		require.NoError(t, err)
		assert.Empty(t, body)
	})

	t.Run("handles content with only frontmatter", func(t *testing.T) {
		content := `---
name: test
---`

		body, err := extractMarkdownBody(content)
		require.NoError(t, err)
		assert.Empty(t, body)
	})

	t.Run("handles frontmatter with extra whitespace", func(t *testing.T) {
		content := `---
name: test
description: Test skill
---

# Test Content`

		body, err := extractMarkdownBody(content)
		require.NoError(t, err)
		assert.Equal(t, "# Test Content", body)
	})

	t.Run("handles content with --- in body", func(t *testing.T) {
		content := `---
name: test
---

# Test Content

Some text.

---

More text after horizontal rule.`

		body, err := extractMarkdownBody(content)
		require.NoError(t, err)
		// Body should include --- that appears after frontmatter.
		assert.Contains(t, body, "---")
		assert.Contains(t, body, "More text after horizontal rule.")
	})

	t.Run("handles incomplete frontmatter", func(t *testing.T) {
		content := `---
name: test
description: Missing closing delimiter

# This looks like a heading

But it's actually inside the frontmatter that never closed.`

		body, err := extractMarkdownBody(content)
		require.NoError(t, err)
		// Without closing delimiter, no frontmatter is detected and entire content is returned.
		// But since inFrontmatter flag was set and never unset, lines after --- are skipped.
		// The function's behavior is to skip everything if frontmatter is never closed.
		assert.Empty(t, body)
	})

	t.Run("handles multiple newlines", func(t *testing.T) {
		content := `---
name: test
---


# Test Content


Multiple blank lines.`

		body, err := extractMarkdownBody(content)
		require.NoError(t, err)
		// TrimSpace removes leading/trailing whitespace.
		assert.True(t, strings.HasPrefix(body, "# Test Content"))
		assert.True(t, strings.HasSuffix(body, "Multiple blank lines."))
	})

	t.Run("handles Windows line endings", func(t *testing.T) {
		content := "---\r\nname: test\r\n---\r\n\r\n# Test Content\r\n"

		body, err := extractMarkdownBody(content)
		require.NoError(t, err)
		assert.Contains(t, body, "# Test Content")
	})
}

func TestExtractFrontmatter(t *testing.T) {
	t.Run("extracts frontmatter from valid content", func(t *testing.T) {
		content := `---
name: test
description: Test skill
license: Apache-2.0
---

# Test Content`

		frontmatter, err := extractFrontmatter(content)
		require.NoError(t, err)
		assert.Contains(t, frontmatter, "name: test")
		assert.Contains(t, frontmatter, "description: Test skill")
		assert.Contains(t, frontmatter, "license: Apache-2.0")

		// Should not contain delimiters or body.
		assert.NotContains(t, frontmatter, "---")
		assert.NotContains(t, frontmatter, "# Test Content")
	})

	t.Run("returns empty string when no frontmatter", func(t *testing.T) {
		content := `# Test Content

No frontmatter here.`

		frontmatter, err := extractFrontmatter(content)
		require.NoError(t, err)
		assert.Empty(t, frontmatter)
	})

	t.Run("handles empty content", func(t *testing.T) {
		frontmatter, err := extractFrontmatter("")
		require.NoError(t, err)
		assert.Empty(t, frontmatter)
	})

	t.Run("handles content with only opening delimiter", func(t *testing.T) {
		content := `---
name: test
description: Never closes`

		frontmatter, err := extractFrontmatter(content)
		require.NoError(t, err)
		// Should extract until EOF if closing delimiter is missing.
		assert.Contains(t, frontmatter, "name: test")
		assert.Contains(t, frontmatter, "description: Never closes")
	})

	t.Run("handles frontmatter with nested structures", func(t *testing.T) {
		content := `---
name: test
metadata:
  author: cloudposse
  version: "1.0.0"
tags:
  - infrastructure
  - terraform
---

# Body`

		frontmatter, err := extractFrontmatter(content)
		require.NoError(t, err)
		assert.Contains(t, frontmatter, "name: test")
		assert.Contains(t, frontmatter, "metadata:")
		assert.Contains(t, frontmatter, "author: cloudposse")
		assert.Contains(t, frontmatter, "tags:")
		assert.Contains(t, frontmatter, "- infrastructure")
	})

	t.Run("handles frontmatter with comments", func(t *testing.T) {
		content := `---
# This is a comment in YAML
name: test  # inline comment
description: Test skill
---

# Body`

		frontmatter, err := extractFrontmatter(content)
		require.NoError(t, err)
		assert.Contains(t, frontmatter, "# This is a comment")
		assert.Contains(t, frontmatter, "name: test")
	})

	t.Run("stops at first closing delimiter", func(t *testing.T) {
		content := `---
name: test
---
description: This is body
---
more: body`

		frontmatter, err := extractFrontmatter(content)
		require.NoError(t, err)
		assert.Contains(t, frontmatter, "name: test")
		// Should not include content after first closing delimiter.
		assert.NotContains(t, frontmatter, "description: This is body")
		assert.NotContains(t, frontmatter, "more: body")
	})

	t.Run("handles whitespace around delimiters", func(t *testing.T) {
		content := `  ---
name: test
  ---

# Body`

		frontmatter, err := extractFrontmatter(content)
		require.NoError(t, err)
		assert.Contains(t, frontmatter, "name: test")
	})

	t.Run("handles delimiter not at start of file", func(t *testing.T) {
		content := `
---
name: test
---

# Body`

		frontmatter, err := extractFrontmatter(content)
		require.NoError(t, err)
		// Frontmatter must start at line 1, so this should return empty.
		assert.Empty(t, frontmatter)
	})
}

//nolint:dupl // Similar test structure is intentional for comprehensive coverage.
func TestReadIntegration(t *testing.T) {
	t.Run("Read and ReadMetadata are complementary", func(t *testing.T) {
		filename := "general/SKILL.md"

		body, err := Read(filename)
		require.NoError(t, err)

		metadata, err := ReadMetadata(filename)
		require.NoError(t, err)

		// Body and metadata should not overlap.
		assert.NotContains(t, body, "name: general")
		assert.NotContains(t, metadata, "# Skill:")

		// Both should be non-empty for valid skill file.
		assert.NotEmpty(t, body)
		assert.NotEmpty(t, metadata)
	})

	t.Run("List returns files that can be Read", func(t *testing.T) {
		files, err := List()
		require.NoError(t, err)

		// Every file from List() should be readable.
		for _, file := range files {
			body, err := Read(file)
			require.NoError(t, err, "Should be able to read %s", file)
			assert.NotEmpty(t, body, "File %s should have content", file)
		}
	})

	t.Run("List returns files with valid metadata", func(t *testing.T) {
		files, err := List()
		require.NoError(t, err)

		// Every file should have valid metadata.
		for _, file := range files {
			metadata, err := ReadMetadata(file)
			require.NoError(t, err, "Should be able to read metadata from %s", file)

			// Validate core metadata fields exist.
			assert.Contains(t, metadata, "name:", "File %s should have 'name' in metadata", file)
			assert.Contains(t, metadata, "description:", "File %s should have 'description' in metadata", file)
		}
	})
}

func TestEmbedFS(t *testing.T) {
	t.Run("Skills embed.FS is not nil", func(t *testing.T) {
		// The Skills variable should be initialized.
		assert.NotNil(t, Skills)
	})

	t.Run("Skills embed.FS contains expected files", func(t *testing.T) {
		// Try to read a known file directly from embed.FS.
		data, err := Skills.ReadFile("general/SKILL.md")
		require.NoError(t, err)
		assert.NotEmpty(t, data)
	})

	t.Run("Skills embed.FS only contains SKILL.md files", func(t *testing.T) {
		files, err := List()
		require.NoError(t, err)

		// All files should be named SKILL.md.
		for _, file := range files {
			assert.True(t, strings.HasSuffix(file, "SKILL.md"), "File %s should be named SKILL.md", file)
		}
	})
}
