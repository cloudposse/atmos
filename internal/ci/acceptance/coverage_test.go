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

func TestFilterCoverageProfilePropagatesErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	validSource := filepath.Join(root, "raw.out")
	if err := os.WriteFile(validSource, []byte("mode: atomic\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Destination whose parent directory can't be created because an
	// intermediate path component is a plain file, not a directory --
	// portable across OSes, unlike permission-based tricks.
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := filterCoverageProfile(validSource, filepath.Join(blocker, "sub", "out.txt")); err == nil {
		t.Fatal("expected an error when the destination directory can't be created")
	}

	if err := filterCoverageProfile(filepath.Join(root, "missing.out"), filepath.Join(root, "out.txt")); err == nil {
		t.Fatal("expected an error when the source file doesn't exist")
	}

	// os.Create fails when the destination path is itself an existing directory.
	existingDir := filepath.Join(root, "already-a-dir")
	if err := os.MkdirAll(existingDir, directoryPermissions); err != nil {
		t.Fatal(err)
	}
	if err := filterCoverageProfile(validSource, existingDir); err == nil {
		t.Fatal("expected an error when the destination is an existing directory")
	}
}

func TestFilterCoverageProfilePropagatesScannerError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "raw.out")
	// bufio.Scanner's default token buffer is 64KB; a single line beyond that
	// makes Scan fail with bufio.ErrTooLong, exercising scanner.Err().
	oversizedLine := strings.Repeat("x", 100_000)
	if err := os.WriteFile(source, []byte("mode: atomic\n"+oversizedLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := filterCoverageProfile(source, filepath.Join(root, "out.txt")); err == nil {
		t.Fatal("expected an error for a line exceeding the scanner's buffer")
	}
}

func TestResetDirectoryPropagatesMkdirAllError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := resetDirectory(filepath.Join(blocker, "sub")); err == nil {
		t.Fatal("expected an error when the target's parent is a plain file")
	}
}

func TestMergeCoveragePropagatesValidationError(t *testing.T) {
	t.Parallel()

	if err := MergeCoverage(t.Context(), t.TempDir(), "out", "", nil); err == nil {
		t.Fatal("expected an error for zero coverage inputs")
	}
}

func TestMergeCoveragePropagatesResetDirectoryError(t *testing.T) {
	t.Parallel()

	withMetadata := t.TempDir()
	if err := os.WriteFile(filepath.Join(withMetadata, "covmeta.abc"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// dataOut resolves (via absoluteFromRoot) to the OS separator itself,
	// which resetDirectory's safety check always rejects.
	err := MergeCoverage(t.Context(), t.TempDir(), string(filepath.Separator), "", []string{withMetadata})
	if err == nil {
		t.Fatal("expected an error for an unsafe dataOut path")
	}
}

func TestMergeCoveragePropagatesMergeCommandError(t *testing.T) {
	t.Parallel()

	// A directory containing a covmeta.* file that isn't real covdata
	// metadata passes validateCoverageInputs' presence check but makes the
	// real `go tool covdata merge` subprocess fail.
	corrupt := t.TempDir()
	if err := os.WriteFile(filepath.Join(corrupt, "covmeta.notreal"), []byte("not real covdata"), 0o600); err != nil {
		t.Fatal(err)
	}
	repoRoot := t.TempDir()
	err := MergeCoverage(t.Context(), repoRoot, filepath.Join(t.TempDir(), "out"), "", []string{corrupt})
	if err == nil {
		t.Fatal("expected an error merging corrupt coverage metadata")
	}
}

func TestMergeCoveragePropagatesWriteCoverageTextError(t *testing.T) {
	t.Parallel()

	repoRoot, err := FindRepoRoot(".")
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	shard := t.TempDir()
	generateCoverageFixture(t, shard)

	dataOut := filepath.Join(t.TempDir(), "merged")
	// A directory sitting at the destination path makes filterCoverageProfile's
	// os.Create fail, propagating up through writeCoverageText into MergeCoverage.
	textOutAsDir := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.MkdirAll(textOutAsDir, directoryPermissions); err != nil {
		t.Fatal(err)
	}

	if err := MergeCoverage(t.Context(), repoRoot, dataOut, textOutAsDir, []string{shard}); err == nil {
		t.Fatal("expected an error when the text report destination is a directory")
	}
}

func TestCollectCoverageDiscoversPackagesWhenNoneGiven(t *testing.T) {
	t.Parallel()

	root := newOrchestrationFixtureModule(t)
	dataOut := filepath.Join(t.TempDir(), "merged")
	err := CollectCoverage(t.Context(), &CoverageOptions{
		RepoRoot: root,
		Dir:      filepath.Join(t.TempDir(), "work"),
		DataOut:  dataOut,
		Timeout:  "1m",
	}, nil, nil)
	if err != nil {
		t.Fatalf("collect coverage with auto-discovered packages: %v", err)
	}
	if !hasCoverageMetadata(dataOut) {
		t.Fatalf("expected native coverage metadata under %s", dataOut)
	}
}

// TestCollectCoverageRejectsUnsafeEmptyOptions is a regression test for a
// critical data-loss bug: absoluteFromRoot("", options.Dir) resolves an empty
// Dir straight to RepoRoot (filepath.Join(root, "") == root), and
// resetDirectory does not special-case "this happens to be the repo root" --
// only literal "." or "/" -- so an omitted Dir would RemoveAll the entire
// checkout. Same failure mode for MergeCoverage's dataOut.
func TestCollectCoverageRejectsUnsafeEmptyOptions(t *testing.T) {
	t.Parallel()

	if err := CollectCoverage(t.Context(), nil, nil, nil); err == nil {
		t.Fatal("expected an error for nil options")
	}

	root := newOrchestrationFixtureModule(t)
	err := CollectCoverage(t.Context(), &CoverageOptions{
		RepoRoot: root,
		Dir:      "", // must be rejected, not silently resolved to RepoRoot
		DataOut:  filepath.Join(t.TempDir(), "merged"),
		Timeout:  "1m",
	}, []string{"./cmd"}, nil)
	if err == nil {
		t.Fatal("expected an error for an empty coverage work directory")
	}

	// The repo root itself must still exist -- confirms the bug this guards
	// against (RemoveAll(RepoRoot)) did not fire.
	if _, statErr := os.Stat(filepath.Join(root, "go.mod")); statErr != nil {
		t.Fatalf("repo root was destroyed despite the validation error: %v", statErr)
	}
}

func TestMergeCoverageRejectsEmptyDataOut(t *testing.T) {
	t.Parallel()

	root := newOrchestrationFixtureModule(t)
	withMetadata := t.TempDir()
	if err := os.WriteFile(filepath.Join(withMetadata, "covmeta.abc"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := MergeCoverage(t.Context(), root, "", "", []string{withMetadata}); err == nil {
		t.Fatal("expected an error for an empty dataOut")
	}
	if _, statErr := os.Stat(filepath.Join(root, "go.mod")); statErr != nil {
		t.Fatalf("repo root was destroyed despite the validation error: %v", statErr)
	}
}

func TestCollectCoveragePropagatesListPackagesError(t *testing.T) {
	t.Parallel()

	err := CollectCoverage(t.Context(), &CoverageOptions{
		RepoRoot: t.TempDir(), // no go.mod here, so `go list ./...` fails
		Dir:      t.TempDir(),
		DataOut:  t.TempDir(),
		Timeout:  "1m",
	}, nil, nil)
	if err == nil {
		t.Fatal("expected an error discovering packages outside any Go module")
	}
}

func TestCollectCoveragePropagatesPrepareDirectoriesError(t *testing.T) {
	t.Parallel()

	root := newOrchestrationFixtureModule(t)
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := CollectCoverage(t.Context(), &CoverageOptions{
		RepoRoot: root,
		Dir:      filepath.Join(blocker, "sub"),
		DataOut:  t.TempDir(),
		Timeout:  "1m",
	}, []string{"./cmd"}, nil)
	if err == nil {
		t.Fatal("expected an error when the coverage work directory can't be created")
	}
}

func TestCollectCoveragePropagatesTestFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/failing\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "broken"), 0o755); err != nil {
		t.Fatal(err)
	}
	failingTest := "package broken\n\nimport \"testing\"\n\nfunc TestAlwaysFails(t *testing.T) {\n\tt.Fatal(\"deliberate failure\")\n}\n"
	if err := os.WriteFile(filepath.Join(root, "broken", "broken_test.go"), []byte(failingTest), 0o600); err != nil {
		t.Fatal(err)
	}

	err := CollectCoverage(t.Context(), &CoverageOptions{
		RepoRoot: root,
		Dir:      filepath.Join(t.TempDir(), "work"),
		DataOut:  filepath.Join(t.TempDir(), "merged"),
		Timeout:  "1m",
	}, []string{"./broken"}, nil)
	if err == nil {
		t.Fatal("expected an error propagated from a failing test")
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
