package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGoModNoReplaceDirectives ensures that go.mod does not contain replace directives.
// This is critical because replace directives break `go install github.com/cloudposse/atmos@latest`.
//
// Background:
// - `go install cmd@version` intentionally does not support modules with replace or exclude directives
// - This is a fundamental design decision in Go (see golang/go#44840, #69762, #50698)
// - The Go team has repeatedly closed issues requesting this feature as "working as intended"
//
// Why this test exists:
// - `go install` is a documented installation method for Atmos.
// - Breaking this method creates user friction and support burden.
// - This test prevents accidental regressions that would break this installation path.
func TestGoModNoReplaceDirectives(t *testing.T) {
	// Find the repository root by looking for go.mod
	repoRoot, err := findRepoRoot()
	require.NoError(t, err, "Failed to find repository root")

	// Read go.mod
	goModPath := filepath.Join(repoRoot, "go.mod")
	content, err := os.ReadFile(goModPath)
	require.NoError(t, err, "Failed to read go.mod")

	// Check for replace directives
	lines := strings.Split(string(content), "\n")
	var replaceDirectives []string

	inReplaceBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect start of replace block
		if strings.HasPrefix(trimmed, "replace (") {
			inReplaceBlock = true
			continue
		}

		// Detect end of replace block
		if inReplaceBlock && trimmed == ")" {
			inReplaceBlock = false
			continue
		}

		// Detect inline replace directive
		if strings.HasPrefix(trimmed, "replace ") && !strings.HasPrefix(trimmed, "replace (") {
			replaceDirectives = append(replaceDirectives, trimmed)
		}

		// Detect replace directive inside block
		if inReplaceBlock && trimmed != "" && !strings.HasPrefix(trimmed, "//") {
			replaceDirectives = append(replaceDirectives, trimmed)
		}
	}

	// Assert no replace directives found
	require.Empty(t, replaceDirectives,
		"go.mod contains replace directives which break 'go install github.com/cloudposse/atmos@latest'.\n"+
			"Replace directives found:\n  %s\n\n"+
			"This breaks a documented installation method. If you need to replace a dependency,\n"+
			"consider alternative approaches that don't break go install compatibility.",
		strings.Join(replaceDirectives, "\n  "))
}

// findRepoRoot walks up the directory tree to find the repository root.
// It looks for the directory containing go.mod.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root without finding go.mod
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
