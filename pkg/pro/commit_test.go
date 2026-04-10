package pro

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// initTestIO initializes the I/O context, data writer, and UI formatter
// required by functions that call data.Writeln or ui.Success/ui.Info.
func initTestIO(t *testing.T) {
	t.Helper()

	ioCtx, err := iolib.NewContext()
	require.NoError(t, err, "failed to initialize I/O context")
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
}

// withTempGitRepo creates an isolated git repository in a temp directory,
// chdir's into it, and restores the original directory on cleanup.
// Tests using this helper must NOT use t.Parallel().
func withTempGitRepo(t *testing.T, fn func(dir string)) {
	t.Helper()

	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")

	origDir, err := os.Getwd()
	require.NoError(t, err)

	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	fn(dir)
}

// runGit executes a git command in the given directory and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

// writeFile is a test helper that creates a file with the given content.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()

	p := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func TestExecuteCommit_LoopPrevention(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.GithubUsername = AtmosProBotActor

	err := ExecuteCommit(atmosConfig, "test", "", "", false)
	assert.NoError(t, err)
}

func TestValidateCommitInputs(t *testing.T) {
	t.Run("valid inputs", func(t *testing.T) {
		err := validateCommitInputs("fix formatting", "looks good")
		assert.NoError(t, err)
	})

	t.Run("empty message", func(t *testing.T) {
		err := validateCommitInputs("", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCommitMessageRequired)
	})

	t.Run("message too long", func(t *testing.T) {
		longMsg := make([]byte, 501)
		for i := range longMsg {
			longMsg[i] = 'a'
		}
		err := validateCommitInputs(string(longMsg), "")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCommitMessageTooLong)
	})

	t.Run("message at max length", func(t *testing.T) {
		maxMsg := make([]byte, 500)
		for i := range maxMsg {
			maxMsg[i] = 'a'
		}
		err := validateCommitInputs(string(maxMsg), "")
		assert.NoError(t, err)
	})

	t.Run("comment too long", func(t *testing.T) {
		longComment := make([]byte, 2001)
		for i := range longComment {
			longComment[i] = 'a'
		}
		err := validateCommitInputs("fix", string(longComment))
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCommentTooLong)
	})

	t.Run("comment at max length", func(t *testing.T) {
		maxComment := make([]byte, 2000)
		for i := range maxComment {
			maxComment[i] = 'a'
		}
		err := validateCommitInputs("fix", string(maxComment))
		assert.NoError(t, err)
	})
}

func TestValidateBranch(t *testing.T) {
	t.Run("valid branches", func(t *testing.T) {
		validBranches := []string{
			"main",
			"feature/my-branch",
			"fix/bug-123",
			"release/v1.0.0",
			"user.name/feature",
			"feature_test",
		}
		for _, branch := range validBranches {
			err := validateBranch(branch)
			assert.NoError(t, err, "branch %q should be valid", branch)
		}
	})

	t.Run("invalid branch characters", func(t *testing.T) {
		err := validateBranch("feature branch")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrBranchInvalid)
	})

	t.Run("branch too long", func(t *testing.T) {
		longBranch := make([]byte, 257)
		for i := range longBranch {
			longBranch[i] = 'a'
		}
		err := validateBranch(string(longBranch))
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrBranchInvalid)
	})

	t.Run("branch at max length", func(t *testing.T) {
		maxBranch := make([]byte, 256)
		for i := range maxBranch {
			maxBranch[i] = 'a'
		}
		err := validateBranch(string(maxBranch))
		assert.NoError(t, err)
	})
}

func TestValidatePath(t *testing.T) {
	t.Run("valid paths", func(t *testing.T) {
		validPaths := []string{
			"main.tf",
			"modules/vpc/main.tf",
			"stacks/dev.yaml",
		}
		for _, p := range validPaths {
			err := validatePath(p)
			assert.NoError(t, err, "path %q should be valid", p)
		}
	})

	t.Run("absolute path rejected", func(t *testing.T) {
		err := validatePath("/etc/passwd")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCommitInvalidFilePath)
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		err := validatePath("../secret.tf")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCommitInvalidFilePath)
	})

	t.Run("github path rejected", func(t *testing.T) {
		err := validatePath(".github/workflows/ci.yml")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCommitInvalidFilePath)
	})

	t.Run("github path rejected case insensitive", func(t *testing.T) {
		err := validatePath(".GitHub/workflows/ci.yml")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCommitInvalidFilePath)
	})
}

func TestFilterPaths(t *testing.T) {
	t.Run("filters invalid paths", func(t *testing.T) {
		paths := []string{
			"main.tf",
			".github/workflows/ci.yml",
			"modules/vpc/main.tf",
			"../escape.tf",
		}
		result := filterPaths(paths)
		assert.Equal(t, []string{"main.tf", "modules/vpc/main.tf"}, result)
	})

	t.Run("all valid paths pass through", func(t *testing.T) {
		paths := []string{"a.tf", "b.tf"}
		result := filterPaths(paths)
		assert.Equal(t, paths, result)
	})

	t.Run("empty input", func(t *testing.T) {
		result := filterPaths([]string{})
		assert.Empty(t, result)
	})
}

func TestCollectFileContents(t *testing.T) {
	t.Run("reads and encodes files", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.tf")
		content := []byte("resource \"aws_vpc\" \"main\" {}")
		err := os.WriteFile(filePath, content, 0o644)
		require.NoError(t, err)

		result, err := collectFileContents([]string{filePath})
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, filePath, result[0].Path)

		decoded, err := base64.StdEncoding.DecodeString(result[0].Contents)
		require.NoError(t, err)
		assert.Equal(t, content, decoded)
	})

	t.Run("skips files exceeding size limit", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "large.bin")
		// Create a file just over 2 MiB.
		largeContent := make([]byte, maxFileSizeBytes+1)
		err := os.WriteFile(filePath, largeContent, 0o644)
		require.NoError(t, err)

		result, err := collectFileContents([]string{filePath})
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("file at max size is included", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "exact.bin")
		content := make([]byte, maxFileSizeBytes)
		err := os.WriteFile(filePath, content, 0o644)
		require.NoError(t, err)

		result, err := collectFileContents([]string{filePath})
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("nonexistent file returns error", func(t *testing.T) {
		result, err := collectFileContents([]string{filepath.Join(t.TempDir(), "nope.tf")})
		require.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("empty input", func(t *testing.T) {
		result, err := collectFileContents([]string{})
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

// --- Tier 1: Pure function tests ---

func TestResolveBranch(t *testing.T) {
	t.Run("nil atmosConfig returns error", func(t *testing.T) {
		_, err := resolveBranch(nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrBranchRequired)
	})

	t.Run("empty GitHubHeadRef returns error", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		_, err := resolveBranch(cfg)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrBranchRequired)
	})

	t.Run("valid branch returns branch", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		cfg.Settings.Pro.GitHubHeadRef = "feature/test"
		branch, err := resolveBranch(cfg)
		require.NoError(t, err)
		assert.Equal(t, "feature/test", branch)
	})

	t.Run("invalid branch returns error", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		cfg.Settings.Pro.GitHubHeadRef = "feature branch"
		_, err := resolveBranch(cfg)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrBranchInvalid)
	})
}

func TestValidateChangeCount(t *testing.T) {
	t.Run("under limit", func(t *testing.T) {
		err := validateChangeCount(make([]string, 100), make([]string, 50))
		assert.NoError(t, err)
	})

	t.Run("at limit", func(t *testing.T) {
		err := validateChangeCount(make([]string, 150), make([]string, 50))
		assert.NoError(t, err)
	})

	t.Run("over limit", func(t *testing.T) {
		err := validateChangeCount(make([]string, 150), make([]string, 51))
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrTooManyChanges)
	})

	t.Run("zero changes", func(t *testing.T) {
		err := validateChangeCount(nil, nil)
		assert.NoError(t, err)
	})
}

func TestBuildDeletions(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		result := buildDeletions([]string{})
		assert.Empty(t, result)
	})

	t.Run("single path", func(t *testing.T) {
		result := buildDeletions([]string{"old.tf"})
		require.Len(t, result, 1)
		assert.Equal(t, dtos.CommitFileDeletion{Path: "old.tf"}, result[0])
	})

	t.Run("multiple paths", func(t *testing.T) {
		result := buildDeletions([]string{"a.tf", "b.tf", "c.tf"})
		require.Len(t, result, 3)
		assert.Equal(t, "a.tf", result[0].Path)
		assert.Equal(t, "b.tf", result[1].Path)
		assert.Equal(t, "c.tf", result[2].Path)
	})
}

// --- Tier 2: Git-based tests ---

func TestGitDiffCachedNames(t *testing.T) {
	t.Run("no staged changes", func(t *testing.T) {
		withTempGitRepo(t, func(_ string) {
			result, err := gitDiffCachedNames("AM")
			require.NoError(t, err)
			assert.Empty(t, result)
		})
	})

	t.Run("staged new file", func(t *testing.T) {
		withTempGitRepo(t, func(dir string) {
			writeFile(t, dir, "main.tf", "resource {}")
			runGit(t, dir, "add", "main.tf")

			result, err := gitDiffCachedNames("AM")
			require.NoError(t, err)
			require.Len(t, result, 1)
			assert.Equal(t, "main.tf", result[0])
		})
	})

	t.Run("staged deletion", func(t *testing.T) {
		withTempGitRepo(t, func(dir string) {
			// Create initial commit with a file.
			writeFile(t, dir, "old.tf", "resource {}")
			runGit(t, dir, "add", "old.tf")
			runGit(t, dir, "commit", "-m", "initial")

			// Delete and stage the deletion.
			require.NoError(t, os.Remove(filepath.Join(dir, "old.tf")))
			runGit(t, dir, "add", "old.tf")

			result, err := gitDiffCachedNames("D")
			require.NoError(t, err)
			require.Len(t, result, 1)
			assert.Equal(t, "old.tf", result[0])
		})
	})
}

func TestDetectChanges(t *testing.T) {
	t.Run("no changes", func(t *testing.T) {
		withTempGitRepo(t, func(_ string) {
			additions, deletions, err := detectChanges()
			require.NoError(t, err)
			assert.Empty(t, additions)
			assert.Empty(t, deletions)
		})
	})

	t.Run("additions and deletions", func(t *testing.T) {
		withTempGitRepo(t, func(dir string) {
			// Create initial commit.
			writeFile(t, dir, "old.tf", "old")
			runGit(t, dir, "add", "old.tf")
			runGit(t, dir, "commit", "-m", "initial")

			// Add a new file and delete the old one.
			writeFile(t, dir, "new.tf", "new")
			require.NoError(t, os.Remove(filepath.Join(dir, "old.tf")))
			runGit(t, dir, "add", "-A")

			additions, deletions, err := detectChanges()
			require.NoError(t, err)
			require.Len(t, additions, 1)
			assert.Equal(t, "new.tf", additions[0])
			require.Len(t, deletions, 1)
			assert.Equal(t, "old.tf", deletions[0])
		})
	})
}

func TestStageFiles(t *testing.T) {
	t.Run("no-op when no flags set", func(t *testing.T) {
		err := stageFiles("", false)
		assert.NoError(t, err)
	})

	t.Run("stage all", func(t *testing.T) {
		withTempGitRepo(t, func(dir string) {
			writeFile(t, dir, "a.tf", "a")
			writeFile(t, dir, "b.yaml", "b")

			err := stageFiles("", true)
			require.NoError(t, err)

			// Verify files are staged.
			result, err := gitDiffCachedNames("AM")
			require.NoError(t, err)
			assert.Len(t, result, 2)
		})
	})

	t.Run("stage by pattern", func(t *testing.T) {
		withTempGitRepo(t, func(dir string) {
			writeFile(t, dir, "a.tf", "a")
			writeFile(t, dir, "b.yaml", "b")

			err := stageFiles("*.tf", false)
			require.NoError(t, err)

			// Only .tf file should be staged.
			result, err := gitDiffCachedNames("AM")
			require.NoError(t, err)
			require.Len(t, result, 1)
			assert.Equal(t, "a.tf", result[0])
		})
	})
}

// --- Tier 3: Integration test for buildChanges ---

func TestBuildChanges(t *testing.T) {
	t.Run("no staged changes returns nil", func(t *testing.T) {
		withTempGitRepo(t, func(_ string) {
			changes, err := buildChanges()
			require.NoError(t, err)
			assert.Nil(t, changes)
		})
	})

	t.Run("staged additions returned", func(t *testing.T) {
		withTempGitRepo(t, func(dir string) {
			writeFile(t, dir, "main.tf", "resource {}")
			runGit(t, dir, "add", "main.tf")

			changes, err := buildChanges()
			require.NoError(t, err)
			require.NotNil(t, changes)
			require.Len(t, changes.Additions, 1)
			assert.Equal(t, "main.tf", changes.Additions[0].Path)
			assert.Empty(t, changes.Deletions)
		})
	})

	t.Run("all filtered paths returns nil", func(t *testing.T) {
		withTempGitRepo(t, func(dir string) {
			// Stage only .github/ files which get filtered out.
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755))
			writeFile(t, dir, filepath.Join(".github", "workflows", "ci.yml"), "name: CI")
			runGit(t, dir, "add", "-A")

			changes, err := buildChanges()
			require.NoError(t, err)
			assert.Nil(t, changes)
		})
	})

	t.Run("staged deletions returned", func(t *testing.T) {
		withTempGitRepo(t, func(dir string) {
			// Create initial commit.
			writeFile(t, dir, "old.tf", "old")
			runGit(t, dir, "add", "old.tf")
			runGit(t, dir, "commit", "-m", "initial")

			// Delete and stage.
			require.NoError(t, os.Remove(filepath.Join(dir, "old.tf")))
			runGit(t, dir, "add", "-A")

			changes, err := buildChanges()
			require.NoError(t, err)
			require.NotNil(t, changes)
			assert.Empty(t, changes.Additions)
			require.Len(t, changes.Deletions, 1)
			assert.Equal(t, "old.tf", changes.Deletions[0].Path)
		})
	})
}

// --- Tier 4: submitCommit and ExecuteCommit integration tests ---

// newCommitTestServer returns an httptest server that accepts commit requests
// and records the last request body for assertions.
func newCommitTestServer(t *testing.T) (*httptest.Server, *dtos.CommitRequest) {
	t.Helper()

	var lastReq dtos.CommitRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		_ = json.Unmarshal(body, &lastReq)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true, "status": 200, "data": {"sha": "test-sha-123"}}`))
	}))

	return server, &lastReq
}

func TestSubmitCommit_Success(t *testing.T) {
	initTestIO(t)

	server, lastReq := newCommitTestServer(t)
	defer server.Close()

	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.Token = "test-token"
	atmosConfig.Settings.Pro.BaseURL = server.URL
	atmosConfig.Settings.Pro.Endpoint = "api"

	changes := &dtos.CommitChanges{
		Additions: []dtos.CommitFileAddition{
			{Path: "main.tf", Contents: "dGVzdA=="},
		},
	}

	err := submitCommit(atmosConfig, "feature/test", "test commit", "nice", changes)
	require.NoError(t, err)
	assert.Equal(t, "feature/test", lastReq.Branch)
	assert.Equal(t, "test commit", lastReq.CommitMessage)
	assert.Equal(t, "nice", lastReq.Comment)
	require.Len(t, lastReq.Changes.Additions, 1)
	assert.Equal(t, "main.tf", lastReq.Changes.Additions[0].Path)
}

func TestSubmitCommit_APIError(t *testing.T) {
	initTestIO(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"success": false, "errorMessage": "bad request"}`))
	}))
	defer server.Close()

	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.Token = "test-token"
	atmosConfig.Settings.Pro.BaseURL = server.URL
	atmosConfig.Settings.Pro.Endpoint = "api"

	changes := &dtos.CommitChanges{
		Additions: []dtos.CommitFileAddition{
			{Path: "main.tf", Contents: "dGVzdA=="},
		},
	}

	err := submitCommit(atmosConfig, "feature/test", "test commit", "", changes)
	require.Error(t, err)
}

func TestExecuteCommit_HappyPath(t *testing.T) {
	initTestIO(t)

	server, _ := newCommitTestServer(t)
	defer server.Close()

	withTempGitRepo(t, func(dir string) {
		writeFile(t, dir, "main.tf", "resource {}")
		runGit(t, dir, "add", "main.tf")

		atmosConfig := &schema.AtmosConfiguration{}
		atmosConfig.Settings.Pro.Token = "test-token"
		atmosConfig.Settings.Pro.BaseURL = server.URL
		atmosConfig.Settings.Pro.Endpoint = "api"
		atmosConfig.Settings.Pro.GitHubHeadRef = "feature/test"

		err := ExecuteCommit(atmosConfig, "test commit", "", "", false)
		require.NoError(t, err)
	})
}

func TestExecuteCommit_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name    string
		message string
		branch  string
		wantErr error
	}{
		{
			name:    "empty message",
			message: "",
			branch:  "feature/test",
			wantErr: errUtils.ErrCommitMessageRequired,
		},
		{
			name:    "empty branch",
			message: "fix",
			branch:  "",
			wantErr: errUtils.ErrBranchRequired,
		},
		{
			name:    "invalid branch",
			message: "fix",
			branch:  "feature branch with spaces",
			wantErr: errUtils.ErrBranchInvalid,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}
			atmosConfig.Settings.Pro.Token = "test-token"
			atmosConfig.Settings.Pro.GitHubHeadRef = tc.branch

			err := ExecuteCommit(atmosConfig, tc.message, "", "", false)
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestExecuteCommit_NoChanges(t *testing.T) {
	initTestIO(t)

	withTempGitRepo(t, func(_ string) {
		atmosConfig := &schema.AtmosConfiguration{}
		atmosConfig.Settings.Pro.Token = "test-token"
		atmosConfig.Settings.Pro.GitHubHeadRef = "feature/test"

		err := ExecuteCommit(atmosConfig, "test commit", "", "", false)
		require.NoError(t, err)
	})
}

func TestValidateAuth(t *testing.T) {
	t.Run("static token is sufficient", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		cfg.Settings.Pro.Token = "test-token"
		require.NoError(t, validateAuth(cfg))
	})

	t.Run("valid OIDC config", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		cfg.Settings.Pro.GithubOIDC.RequestURL = "https://token.actions.githubusercontent.com"
		cfg.Settings.Pro.GithubOIDC.RequestToken = "gha-token"
		cfg.Settings.Pro.WorkspaceID = "ws-123"
		require.NoError(t, validateAuth(cfg))
	})

	t.Run("no token and no OIDC", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		err := validateAuth(cfg)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrNotInGitHubActions)
	})

	t.Run("OIDC without workspace ID", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		cfg.Settings.Pro.GithubOIDC.RequestURL = "https://token.actions.githubusercontent.com"
		cfg.Settings.Pro.GithubOIDC.RequestToken = "gha-token"
		err := validateAuth(cfg)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrOIDCWorkspaceIDRequired)
	})
}

func TestEnsureGitSafeDirectory(t *testing.T) {
	t.Run("skips when not in GitHub Actions", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "")
		err := ensureGitSafeDirectory()
		require.NoError(t, err)
	})

	t.Run("skips when GITHUB_WORKSPACE is empty", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_WORKSPACE", "")
		err := ensureGitSafeDirectory()
		require.NoError(t, err)
	})

	t.Run("adds safe directory in GitHub Actions", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_WORKSPACE", "/tmp/test-workspace")

		err := ensureGitSafeDirectory()
		require.NoError(t, err)

		// Verify git config was set.
		out, err := exec.Command("git", "config", "--global", "--get-all", "safe.directory").Output()
		require.NoError(t, err)
		assert.Contains(t, string(out), "/tmp/test-workspace")
	})
}
