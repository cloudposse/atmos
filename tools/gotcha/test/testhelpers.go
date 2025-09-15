package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTemplate represents a test file template.
type TestTemplate struct {
	Name    string
	Content string
}

// GetTestTemplates returns predefined test templates for gotcha testing.
func GetTestTemplates() map[string]TestTemplate {
	return map[string]TestTemplate{
		"failing": {
			Name: "failing_test.go",
			Content: `package testpkg

import "testing"

func TestFail1(t *testing.T) {
	t.Fatal("This test fails intentionally")
}

func TestFail2(t *testing.T) {
	t.Error("This test also fails")
}`,
		},
		"passing": {
			Name: "passing_test.go",
			Content: `package testpkg

import "testing"

func TestPass1(t *testing.T) {
	// This test passes
	if 1+1 != 2 {
		t.Fatal("math is broken")
	}
}

func TestPass2(t *testing.T) {
	// Another passing test
}

func TestPass3(t *testing.T) {
	// Yet another passing test
}`,
		},
		"skipping": {
			Name: "skipping_test.go",
			Content: `package testpkg

import "testing"

func TestSkip1(t *testing.T) {
	t.Skip("This test is skipped")
}

func TestSkip2(t *testing.T) {
	t.Skip("Another skipped test")
}`,
		},
		"mixed": {
			Name: "mixed_test.go",
			Content: `package testpkg

import (
	"testing"
	"time"
)

func TestPass1(t *testing.T) {
	// This passes
}

func TestPass2(t *testing.T) {
	// Another passing test
	time.Sleep(10 * time.Millisecond)
}

func TestFail1(t *testing.T) {
	t.Fatal("This test fails intentionally")
}

func TestSkip1(t *testing.T) {
	t.Skip("This test is skipped")
}`,
		},
	}
}

// CreateTestFiles creates test files from templates in the specified directory.
func CreateTestFiles(t *testing.T, dir string, templateNames ...string) {
	templates := GetTestTemplates()

	for _, name := range templateNames {
		tmpl, ok := templates[name]
		require.True(t, ok, "Unknown template: %s", name)

		filePath := filepath.Join(dir, tmpl.Name)
		err := os.WriteFile(filePath, []byte(tmpl.Content), 0o644)
		require.NoError(t, err, "Failed to write test file: %s", filePath)
	}

	// Create go.mod file if it doesn't exist
	goModPath := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		goModContent := `module testpkg

go 1.21`
		err := os.WriteFile(goModPath, []byte(goModContent), 0o644)
		require.NoError(t, err, "Failed to write go.mod")
	}
}

// CreateMixedTestFile creates a test file with custom content.
func CreateMixedTestFile(t *testing.T, dir string, filename string, content string) {
	filePath := filepath.Join(dir, filename)
	err := os.WriteFile(filePath, []byte(content), 0o644)
	require.NoError(t, err, "Failed to write test file: %s", filePath)

	// Create go.mod file if it doesn't exist
	goModPath := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		goModContent := `module testpkg

go 1.21`
		err := os.WriteFile(goModPath, []byte(goModContent), 0o644)
		require.NoError(t, err, "Failed to write go.mod")
	}
}

// CreateGotchaCommand creates an exec.Cmd for running gotcha with GITHUB_STEP_SUMMARY cleared.
// This prevents test executions from polluting GitHub Actions step summaries.
func CreateGotchaCommand(binary string, args ...string) *exec.Cmd {
	cmd := exec.Command(binary, args...)
	// Clear GITHUB_STEP_SUMMARY to prevent test output from polluting CI summary
	// This ensures that when tests run gotcha on temporary test packages,
	// those sub-runs don't append their summaries to the main GitHub step summary
	cmd.Env = append(os.Environ(), "GITHUB_STEP_SUMMARY=")
	return cmd
}
