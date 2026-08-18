package acceptance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestFilterCoverageProfile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "raw.out")
	destination := filepath.Join(root, "nested", "coverage.out")
	content := "mode: atomic\nexample/real.go:1.1,2.1 1 1\nexample/mock_generated.go:1.1,2.1 1 1\n"
	if err := os.WriteFile(source, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := filterCoverageProfile(source, destination); err != nil {
		t.Fatalf("filter coverage: %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "mock_") {
		t.Fatalf("filtered profile contains mock coverage: %s", got)
	}
	if !strings.Contains(string(got), "example/real.go") {
		t.Fatalf("filtered profile dropped real coverage: %s", got)
	}
}

func TestResetDirectoryRejectsUnsafePaths(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"", ".", string(filepath.Separator)} {
		if err := resetDirectory(path); err == nil {
			t.Fatalf("expected unsafe path %q to be rejected", path)
		}
	}
}

func regexpMustCompile(t *testing.T, value string) *regexp.Regexp {
	t.Helper()
	compiled, err := regexp.Compile(value)
	if err != nil {
		t.Fatalf("compile regexp %q: %v", value, err)
	}
	return compiled
}
