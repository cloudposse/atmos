package acceptance

import (
	"errors"
	"os"
	"os/exec"
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

func TestDefaultValue(t *testing.T) {
	t.Parallel()

	if got := defaultValue("", "fallback"); got != "fallback" {
		t.Fatalf("defaultValue() of empty input = %q, want %q", got, "fallback")
	}
	if got := defaultValue("set", "fallback"); got != "set" {
		t.Fatalf("defaultValue() of set input = %q, want %q", got, "set")
	}
}

func TestPrepareCoverageDirectories(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "coverage-work")
	absIntegration, absUnit, err := prepareCoverageDirectories(root)
	if err != nil {
		t.Fatalf("prepare coverage directories: %v", err)
	}
	if !filepath.IsAbs(absIntegration) || !filepath.IsAbs(absUnit) {
		t.Fatalf("expected absolute paths, got integration=%q unit=%q", absIntegration, absUnit)
	}
	for _, dir := range []string{absIntegration, absUnit} {
		info, statErr := os.Stat(dir)
		if statErr != nil {
			t.Fatalf("expected %s to exist: %v", dir, statErr)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", dir)
		}
	}
	if filepath.Base(absIntegration) != "integration" || filepath.Base(absUnit) != "unit" {
		t.Fatalf("unexpected directory names: integration=%q unit=%q", absIntegration, absUnit)
	}

	if _, _, err := prepareCoverageDirectories(""); err == nil {
		t.Fatal("expected an error for an unsafe root")
	}
}

func TestValidateCoverageInputs(t *testing.T) {
	t.Parallel()

	if err := validateCoverageInputs(nil); err == nil {
		t.Fatal("expected an error for zero input directories")
	}

	withMetadata := t.TempDir()
	if err := os.WriteFile(filepath.Join(withMetadata, "covmeta.abc"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateCoverageInputs([]string{withMetadata}); err != nil {
		t.Fatalf("expected a directory with covmeta.* to validate: %v", err)
	}

	if err := validateCoverageInputs([]string{withMetadata, t.TempDir()}); err == nil {
		t.Fatal("expected an error when one of several inputs has no coverage metadata")
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

func TestMergeCoverageShardsErrorsWhenNoShardHasCoverage(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	err := MergeCoverageShards(t.Context(), CoverageShardOptions{
		RepoRoot:  repoRoot,
		Count:     2,
		ShardsDir: filepath.Join("coverage", "shards"),
		DataOut:   filepath.Join("coverage", "merged"),
		TextOut:   filepath.Join("coverage", "coverage.out"),
	})
	if !errors.Is(err, errCoverageData) {
		t.Fatalf("expected errCoverageData, got %v", err)
	}
}

// TestMergeCoverageShardsSkipsEmptyShardsAndNormalizesRelativePaths generates one
// real covmeta fixture (via `go test -covermode=atomic`) to prove that a mix of a
// missing shard directory, an empty shard directory, and one real shard still
// merges successfully, and that relative ShardsDir/DataOut/TextOut resolve
// against RepoRoot rather than the test process's working directory.
func TestMergeCoverageShardsSkipsEmptyShardsAndNormalizesRelativePaths(t *testing.T) {
	repoRoot := t.TempDir()
	shardsDir := filepath.Join("coverage", "shards")

	shard1 := filepath.Join(repoRoot, shardsDir, "shard-1")
	if err := os.MkdirAll(shard1, directoryPermissions); err != nil {
		t.Fatal(err)
	}
	generateCoverageFixture(t, shard1)

	// shard-2's directory is intentionally never created (missing entirely).
	shard3 := filepath.Join(repoRoot, shardsDir, "shard-3")
	if err := os.MkdirAll(shard3, directoryPermissions); err != nil {
		t.Fatal(err)
	}

	dataOut := filepath.Join("coverage", "merged")
	textOut := filepath.Join("coverage", "coverage.out")

	if err := MergeCoverageShards(t.Context(), CoverageShardOptions{
		RepoRoot:  repoRoot,
		Count:     3,
		ShardsDir: shardsDir,
		DataOut:   dataOut,
		TextOut:   textOut,
	}); err != nil {
		t.Fatalf("merge coverage shards: %v", err)
	}

	if !hasCoverageMetadata(filepath.Join(repoRoot, dataOut)) {
		t.Fatalf("expected merged coverage metadata under repo root %s, not the test process's working directory", repoRoot)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, textOut)); err != nil {
		t.Fatalf("expected coverage text report under repo root %s: %v", repoRoot, err)
	}
}

// generateCoverageFixture produces one real covmeta.* fixture by running `go test`
// against this package's own (fast, side-effect-free) tests, matching this
// package's existing convention of exercising real `go` tooling in tests instead
// of mocking it (see plan_test.go's TestGoCommandEnvironmentDisablesCGO).
func generateCoverageFixture(t *testing.T, gocoverdir string) {
	t.Helper()
	repoRoot, err := FindRepoRoot(".")
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	cmd := exec.Command("go", "test", "-covermode=atomic", "-coverpkg=./internal/ci/acceptance",
		"-run", "TestSplitCommandLinePreservesQuoting", "./internal/ci/acceptance",
		"-args", "-test.gocoverdir="+gocoverdir)
	cmd.Dir = repoRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate coverage fixture: %v\n%s", err, output)
	}
}
