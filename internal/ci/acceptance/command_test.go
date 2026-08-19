package acceptance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvironment(t *testing.T) {
	t.Setenv("ATMOS_TEST_ACCEPTANCE_ENV_VAR", "value")
	if got := environment("ATMOS_TEST_ACCEPTANCE_ENV_VAR"); got != "value" {
		t.Fatalf("environment() = %q, want %q", got, "value")
	}
	if got := environment("ATMOS_TEST_ACCEPTANCE_ENV_VAR_UNSET"); got != "" {
		t.Fatalf("environment() of an unset variable = %q, want empty", got)
	}
}

func TestFindRepoRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/findroot\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := FindRepoRoot(nested)
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	// t.TempDir() can return a path containing symlinks (e.g. macOS's
	// /var -> /private/var); resolve both sides before comparing.
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	gotRoot, err := filepath.EvalSymlinks(found)
	if err != nil {
		t.Fatal(err)
	}
	if gotRoot != wantRoot {
		t.Fatalf("FindRepoRoot(%q) = %q, want %q", nested, gotRoot, wantRoot)
	}
}

func TestFindRepoRootErrorsOutsideAnyModule(t *testing.T) {
	t.Parallel()

	if _, err := FindRepoRoot(string(filepath.Separator)); err == nil {
		t.Fatal("expected an error finding a repo root from the filesystem root")
	}
}

func TestCommandString(t *testing.T) {
	t.Parallel()

	got := commandString("go", []string{"test", "./..."})
	want := "go test ./..."
	if got != want {
		t.Fatalf("commandString() = %q, want %q", got, want)
	}
}
