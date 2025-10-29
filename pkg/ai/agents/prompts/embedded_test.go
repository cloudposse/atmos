package prompts

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRead_ExistingFile(t *testing.T) {
	// Test reading an existing embedded prompt file.
	content, err := Read("general.md")
	require.NoError(t, err)
	assert.NotEmpty(t, content)

	// Verify it's the general agent prompt.
	assert.True(t, strings.HasPrefix(content, "# Agent: General"))
	assert.Contains(t, content, "## Role")
	assert.Contains(t, content, "## Your Expertise")
}

func TestRead_NonExistentFile(t *testing.T) {
	// Test error handling when file doesn't exist.
	content, err := Read("nonexistent.md")
	assert.Error(t, err)
	assert.Empty(t, content)
	assert.Contains(t, err.Error(), "failed to read embedded prompt file")
	assert.Contains(t, err.Error(), "nonexistent.md")
}

func TestRead_AllBuiltInPrompts(t *testing.T) {
	// Test that all built-in agent prompts can be read.
	promptFiles := []string{
		"general.md",
		"stack-analyzer.md",
		"component-refactor.md",
		"security-auditor.md",
		"config-validator.md",
	}

	for _, filename := range promptFiles {
		t.Run(filename, func(t *testing.T) {
			content, err := Read(filename)
			require.NoError(t, err, "Failed to read %s", filename)
			assert.NotEmpty(t, content, "Content should not be empty for %s", filename)

			// Verify basic structure.
			assert.True(t, strings.HasPrefix(content, "# Agent:"), "Should start with '# Agent:' in %s", filename)
			assert.Contains(t, content, "## Role", "Should have '## Role' section in %s", filename)
			assert.Contains(t, content, "## Your Expertise", "Should have '## Your Expertise' section in %s", filename)

			// Verify substantial content (at least 1KB).
			assert.Greater(t, len(content), 1024, "Prompt should be substantial (>1KB) in %s", filename)
		})
	}
}

func TestList(t *testing.T) {
	// Test listing all embedded prompt files.
	files, err := List()
	require.NoError(t, err)
	assert.NotEmpty(t, files, "Should have at least one prompt file")

	// Verify we have all expected prompt files.
	expectedFiles := []string{
		"general.md",
		"stack-analyzer.md",
		"component-refactor.md",
		"security-auditor.md",
		"config-validator.md",
	}

	for _, expected := range expectedFiles {
		assert.Contains(t, files, expected, "Should contain %s", expected)
	}

	// Verify README.md is excluded (we only want agent prompts).
	assert.NotContains(t, files, "README.md", "Should not contain README.md")

	// Verify all listed files end with .md.
	for _, file := range files {
		assert.True(t, strings.HasSuffix(file, ".md"), "File %s should end with .md", file)
	}
}

func TestList_FilesAreReadable(t *testing.T) {
	// Test that all files returned by List() can be read.
	files, err := List()
	require.NoError(t, err)

	for _, filename := range files {
		t.Run(filename, func(t *testing.T) {
			content, err := Read(filename)
			require.NoError(t, err, "Should be able to read %s", filename)
			assert.NotEmpty(t, content, "Content should not be empty for %s", filename)
		})
	}
}

func TestPromptContent_Consistency(t *testing.T) {
	// Test that prompt content is consistent across multiple reads.
	filename := "general.md"

	content1, err1 := Read(filename)
	require.NoError(t, err1)

	content2, err2 := Read(filename)
	require.NoError(t, err2)

	assert.Equal(t, content1, content2, "Content should be identical across reads")
}

func TestPromptContent_NoEmptyLines(t *testing.T) {
	// Test that prompts don't have excessive empty lines at the end.
	files, err := List()
	require.NoError(t, err)

	for _, filename := range files {
		t.Run(filename, func(t *testing.T) {
			content, err := Read(filename)
			require.NoError(t, err)

			// Check that content doesn't end with more than 2 newlines.
			trimmed := strings.TrimRight(content, "\n")
			assert.LessOrEqual(t, len(content)-len(trimmed), 2, "Should not have excessive trailing newlines in %s", filename)
		})
	}
}

func TestPromptContent_ValidMarkdown(t *testing.T) {
	// Test basic Markdown structure validity.
	files, err := List()
	require.NoError(t, err)

	for _, filename := range files {
		t.Run(filename, func(t *testing.T) {
			content, err := Read(filename)
			require.NoError(t, err)

			// Should have a level-1 heading at the start.
			assert.True(t, strings.HasPrefix(content, "# "), "Should start with level-1 heading in %s", filename)

			// Should have multiple level-2 headings.
			level2Count := strings.Count(content, "\n## ")
			assert.GreaterOrEqual(t, level2Count, 3, "Should have at least 3 level-2 headings in %s", filename)

			// Should not have malformed headings (e.g., "###" without space).
			assert.NotContains(t, content, "###Tool", "Should have space after ### in %s", filename)
			assert.NotContains(t, content, "##Role", "Should have space after ## in %s", filename)
		})
	}
}

func TestPromptContent_Size(t *testing.T) {
	// Test that prompts are reasonably sized (not too small, not too large).
	files, err := List()
	require.NoError(t, err)

	for _, filename := range files {
		t.Run(filename, func(t *testing.T) {
			content, err := Read(filename)
			require.NoError(t, err)

			size := len(content)

			// Should be at least 1KB (substantial content).
			assert.GreaterOrEqual(t, size, 1024, "Prompt should be at least 1KB in %s", filename)

			// Should be less than 20KB (keep prompts focused).
			assert.LessOrEqual(t, size, 20*1024, "Prompt should be less than 20KB in %s", filename)
		})
	}
}
