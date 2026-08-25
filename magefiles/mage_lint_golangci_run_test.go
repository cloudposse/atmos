//go:build mage

package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGolangciLintArgs(t *testing.T) {
	cases := []struct {
		name       string
		concurrent string
		parallel   string
		want       []string
	}{
		{
			name: "defaults",
			want: []string{"run", "--config=.golangci.yml", "--allow-serial-runners"},
		},
		{
			name:       "concurrency override",
			concurrent: "4",
			want:       []string{"run", "--config=.golangci.yml", "--concurrency=4", "--allow-serial-runners"},
		},
		{
			name:     "allow parallel runners",
			parallel: "1",
			want:     []string{"run", "--config=.golangci.yml", "--allow-serial-runners", "--allow-parallel-runners"},
		},
		{
			name:     "allow parallel explicitly disabled",
			parallel: "0",
			want:     []string{"run", "--config=.golangci.yml", "--allow-serial-runners"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOLANGCI_CONCURRENCY", tc.concurrent)
			t.Setenv("GOLANGCI_ALLOW_PARALLEL", tc.parallel)
			assert.Equal(t, tc.want, golangciLintArgs())
		})
	}
}

func TestGitDiffCachedQuiet(t *testing.T) {
	t.Run("no staged changes", func(t *testing.T) {
		root := initGitRepoFixture(t)
		assert.True(t, gitDiffCachedQuiet(root))
	})

	t.Run("staged go file", func(t *testing.T) {
		root := initGitRepoFixture(t)
		require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644))
		runGit(t, root, "add", "main.go")
		assert.False(t, gitDiffCachedQuiet(root))
	})

	t.Run("staged non-go file only", func(t *testing.T) {
		root := initGitRepoFixture(t)
		require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("docs"), 0o644))
		runGit(t, root, "add", "README.md")
		assert.True(t, gitDiffCachedQuiet(root))
	})
}

func TestGitHasMergeHead(t *testing.T) {
	t.Run("no merge in progress", func(t *testing.T) {
		root := initGitRepoFixture(t)
		assert.False(t, gitHasMergeHead(root))
	})

	t.Run("merge head present", func(t *testing.T) {
		root := initGitRepoFixture(t)
		sha := gitRevParseHead(t, root)
		require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "MERGE_HEAD"), []byte(sha+"\n"), 0o644))
		assert.True(t, gitHasMergeHead(root))
	})
}

func gitRevParseHead(t *testing.T, root string) string {
	t.Helper()
	out, err := execGitOutput(root, "rev-parse", "HEAD")
	require.NoError(t, err)
	return out
}

func TestShouldUseNewFromRev(t *testing.T) {
	t.Run("no staged go changes", func(t *testing.T) {
		root := initGitRepoFixture(t)
		assert.True(t, shouldUseNewFromRev(root))
	})

	t.Run("staged go changes, no merge", func(t *testing.T) {
		root := initGitRepoFixture(t)
		require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644))
		runGit(t, root, "add", "main.go")
		assert.False(t, shouldUseNewFromRev(root))
	})

	t.Run("staged go changes but merge in progress takes precedence", func(t *testing.T) {
		root := initGitRepoFixture(t)
		require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644))
		runGit(t, root, "add", "main.go")
		sha := gitRevParseHead(t, root)
		require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "MERGE_HEAD"), []byte(sha+"\n"), 0o644))
		assert.True(t, shouldUseNewFromRev(root))
	})
}

func TestUniqueSortedPackageDirs(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  []string
	}{
		{"nil input", nil, []string{}},
		{"empty input", []string{}, []string{}},
		{"single file", []string{"pkg/foo/bar.go"}, []string{"./pkg/foo"}},
		{"same dir deduped", []string{"pkg/foo/a.go", "pkg/foo/b.go"}, []string{"./pkg/foo"}},
		{"different dirs sorted", []string{"pkg/z/a.go", "pkg/a/b.go"}, []string{"./pkg/a", "./pkg/z"}},
		{"root level file", []string{"main.go"}, []string{"."}},
		{"stray empty entries skipped", []string{"", "pkg/foo/a.go", ""}, []string{"./pkg/foo"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, uniqueSortedPackageDirs(tc.files))
		})
	}
}

func TestBuildStagedPatchAndPackages(t *testing.T) {
	t.Run("single staged file", func(t *testing.T) {
		root := initGitRepoFixture(t)
		require.NoError(t, os.WriteFile(filepath.Join(root, "pkg_x_file.go"), []byte("package main"), 0o644))
		runGit(t, root, "add", "pkg_x_file.go")

		patch, cleanup, err := buildStagedPatchAndPackages(root, t.TempDir())
		require.NoError(t, err)
		defer cleanup()

		assert.Equal(t, []string{"."}, patch.packages)
		data, readErr := os.ReadFile(patch.path)
		require.NoError(t, readErr)
		assert.NotEmpty(t, data)
	})

	t.Run("cleanup removes the patch file", func(t *testing.T) {
		root := initGitRepoFixture(t)
		require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644))
		runGit(t, root, "add", "main.go")

		patch, cleanup, err := buildStagedPatchAndPackages(root, t.TempDir())
		require.NoError(t, err)
		cleanup()

		_, statErr := os.Stat(patch.path)
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("not a git repo", func(t *testing.T) {
		root := t.TempDir()
		_, _, err := buildStagedPatchAndPackages(root, t.TempDir())
		require.Error(t, err)
	})

	t.Run("tmpDir does not exist", func(t *testing.T) {
		root := initGitRepoFixture(t)
		_, _, err := buildStagedPatchAndPackages(root, filepath.Join(t.TempDir(), "missing"))
		require.Error(t, err)
	})
}

// withFakeUserCacheDir points userCacheDirFunc at dir for the duration of
// the calling test, restoring the original on cleanup.
func withFakeUserCacheDir(t *testing.T, dir string) {
	t.Helper()
	restore := userCacheDirFunc
	userCacheDirFunc = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userCacheDirFunc = restore })
}

func TestSetWorktreeIsolationEnv(t *testing.T) {
	t.Run("shared cache opt-out returns empty map", func(t *testing.T) {
		t.Setenv("ATMOS_LINT_SHARED_CACHE", "1")
		env, err := setWorktreeIsolationEnv(t.TempDir())
		require.NoError(t, err)
		assert.Empty(t, env)
	})

	t.Run("default isolates cache and tmp under the user cache dir, not root", func(t *testing.T) {
		t.Setenv("ATMOS_LINT_SHARED_CACHE", "")
		t.Setenv("GOLANGCI_LINT_CACHE", "")
		fakeUserCache := t.TempDir()
		withFakeUserCacheDir(t, fakeUserCache)

		root := t.TempDir()
		env, err := setWorktreeIsolationEnv(root)
		require.NoError(t, err)

		wantBase := filepath.Join(fakeUserCache, "atmos-lint")
		cacheDir := env["GOLANGCI_LINT_CACHE"]
		tmpDir := env["TMPDIR"]
		assert.True(t, strings.HasPrefix(cacheDir, wantBase+string(filepath.Separator)))
		assert.Equal(t, "cache", filepath.Base(cacheDir))
		assert.True(t, strings.HasPrefix(tmpDir, wantBase+string(filepath.Separator)))
		assert.Equal(t, "tmp", filepath.Base(tmpDir))
		assert.Equal(t, filepath.Dir(cacheDir), filepath.Dir(tmpDir), "cache and tmp share the same hashed worktree dir")
		assert.False(t, strings.HasPrefix(cacheDir, root), "cache dir must not live inside the worktree")

		info, statErr := os.Stat(tmpDir)
		require.NoError(t, statErr)
		assert.True(t, info.IsDir())

		if runtime.GOOS == "windows" {
			assert.Equal(t, tmpDir, env["TMP"])
			assert.Equal(t, tmpDir, env["TEMP"])
		}
	})

	t.Run("GOLANGCI_LINT_CACHE override respected", func(t *testing.T) {
		t.Setenv("ATMOS_LINT_SHARED_CACHE", "")
		withFakeUserCacheDir(t, t.TempDir())
		custom := filepath.Join(t.TempDir(), "custom-cache")
		t.Setenv("GOLANGCI_LINT_CACHE", custom)
		env, err := setWorktreeIsolationEnv(t.TempDir())
		require.NoError(t, err)
		assert.Equal(t, custom, env["GOLANGCI_LINT_CACHE"])
	})

	t.Run("mkdir failure when cache root path component is a file", func(t *testing.T) {
		t.Setenv("ATMOS_LINT_SHARED_CACHE", "")
		t.Setenv("GOLANGCI_LINT_CACHE", "")
		blocker := t.TempDir()
		filePath := filepath.Join(blocker, "not-a-dir")
		require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o644))
		withFakeUserCacheDir(t, filePath)

		_, err := setWorktreeIsolationEnv(t.TempDir())
		require.Error(t, err)
	})

	t.Run("propagates user cache dir resolution failure", func(t *testing.T) {
		t.Setenv("ATMOS_LINT_SHARED_CACHE", "")
		t.Setenv("GOLANGCI_LINT_CACHE", "")
		restore := userCacheDirFunc
		userCacheDirFunc = func() (string, error) { return "", errors.New("no cache dir") }
		t.Cleanup(func() { userCacheDirFunc = restore })

		_, err := setWorktreeIsolationEnv(t.TempDir())
		require.Error(t, err)
	})
}

func TestWorktreeLintCacheRoot(t *testing.T) {
	fakeUserCache := t.TempDir()
	withFakeUserCacheDir(t, fakeUserCache)

	rootA := t.TempDir()
	rootB := t.TempDir()

	dirA, err := worktreeLintCacheRoot(rootA)
	require.NoError(t, err)
	dirB, err := worktreeLintCacheRoot(rootB)
	require.NoError(t, err)

	assert.NotEqual(t, dirA, dirB, "distinct worktrees must not collide on the same cache dir")
	assert.True(t, strings.HasPrefix(dirA, filepath.Join(fakeUserCache, "atmos-lint")))

	dirAAgain, err := worktreeLintCacheRoot(rootA)
	require.NoError(t, err)
	assert.Equal(t, dirA, dirAAgain, "the same worktree root must hash to the same dir")
}

func TestRunGolangciLintPrecommit(t *testing.T) {
	t.Run("no staged go changes uses new-from-rev", func(t *testing.T) {
		root := initGitRepoFixture(t)
		binPath := customGCLBinaryPath(root)
		argsFile := setUpFakeBin(t, binPath)
		t.Setenv("ATMOS_LINT_SHARED_CACHE", "1")

		require.NoError(t, runGolangciLintPrecommit(root, binPath))

		args := readFakeBinArgs(t, argsFile)
		assert.Contains(t, args, "--new-from-rev=origin/main")
	})

	t.Run("GOLANGCI_NEW_FROM_REV override respected", func(t *testing.T) {
		root := initGitRepoFixture(t)
		binPath := customGCLBinaryPath(root)
		argsFile := setUpFakeBin(t, binPath)
		t.Setenv("ATMOS_LINT_SHARED_CACHE", "1")
		t.Setenv("GOLANGCI_NEW_FROM_REV", "HEAD~1")

		require.NoError(t, runGolangciLintPrecommit(root, binPath))

		args := readFakeBinArgs(t, argsFile)
		assert.Contains(t, args, "--new-from-rev=HEAD~1")
	})

	t.Run("staged go changes uses new-from-patch and package args", func(t *testing.T) {
		root := initGitRepoFixture(t)
		require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644))
		runGit(t, root, "add", "main.go")

		binPath := customGCLBinaryPath(root)
		argsFile := setUpFakeBin(t, binPath)
		t.Setenv("ATMOS_LINT_SHARED_CACHE", "1")

		require.NoError(t, runGolangciLintPrecommit(root, binPath))

		args := readFakeBinArgs(t, argsFile)
		foundPatch := false
		for _, a := range args {
			if len(a) > len("--new-from-patch=") && a[:len("--new-from-patch=")] == "--new-from-patch=" {
				foundPatch = true
			}
		}
		assert.True(t, foundPatch, "expected --new-from-patch= arg, got %v", args)
		assert.Contains(t, args, ".")
	})

	t.Run("fake binary non-zero exit propagates as error", func(t *testing.T) {
		root := initGitRepoFixture(t)
		binPath := customGCLBinaryPath(root)
		setUpFakeBin(t, binPath)
		t.Setenv("ATMOS_LINT_SHARED_CACHE", "1")
		t.Setenv(fakeBinExitEnv, "1")

		err := runGolangciLintPrecommit(root, binPath)
		require.Error(t, err)
	})

	// TestRunGolangciLintPrecommit/isolation env error propagates covers the
	// setWorktreeIsolationEnv failure branch: the user-cache-dir root
	// resolves to a file, not a directory, so MkdirAll(".../tmp") under it
	// fails before any subprocess is invoked.
	t.Run("isolation env error propagates", func(t *testing.T) {
		t.Setenv("ATMOS_LINT_SHARED_CACHE", "")
		t.Setenv("GOLANGCI_LINT_CACHE", "")
		blocker := t.TempDir()
		filePath := filepath.Join(blocker, "not-a-dir")
		require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o644))
		withFakeUserCacheDir(t, filePath)

		root := initGitRepoFixture(t)
		err := runGolangciLintPrecommit(root, "irrelevant-bin")
		require.Error(t, err)
	})

	// TestRunGolangciLintPrecommit/staged patch error propagates covers the
	// buildStagedPatchAndPackages failure branch reached via the non-git-repo
	// path (no staged .go changes AND no merge in progress AND `git diff
	// --cached --binary` itself fails because root isn't a git repo).
	t.Run("staged patch error propagates", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("ATMOS_LINT_SHARED_CACHE", "1")

		err := runGolangciLintPrecommit(root, "irrelevant-bin")
		require.Error(t, err)
	})
}
