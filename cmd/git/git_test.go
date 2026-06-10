package git

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ---- URI parsing tests ----

func TestIsURI(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want bool
	}{
		{"https", "https://github.com/acme/repo.git", true},
		{"http", "http://internal/repo.git", true},
		{"git getter prefix", "git::https://github.com/acme/repo.git", true},
		{"ssh scheme", "ssh://git@github.com/acme/repo.git", true},
		{"git scheme", "git://github.com/acme/repo.git", true},
		{"scp style", "git@github.com:acme/repo.git", true},
		{"plain name", "flux-deploy", false},
		{"local path", "./deployments", false},
		{"abs path", "/home/user/repo", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsURI(tt.arg))
		})
	}
}

func TestParseCloneURI(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantURI    string
		wantBranch string
		wantDepth  int
		wantRepo   string
		wantErrMsg string
	}{
		{
			name:     "plain https",
			raw:      "https://github.com/acme/repo.git",
			wantURI:  "https://github.com/acme/repo.git",
			wantRepo: "repo",
		},
		{
			name:     "git getter prefix stripped",
			raw:      "git::https://github.com/acme/myrepo.git",
			wantURI:  "https://github.com/acme/myrepo.git",
			wantRepo: "myrepo",
		},
		{
			name:       "ref query param",
			raw:        "git::https://github.com/acme/repo.git?ref=main",
			wantURI:    "https://github.com/acme/repo.git",
			wantBranch: "main",
			wantRepo:   "repo",
		},
		{
			name:      "depth query param",
			raw:       "git::https://github.com/acme/repo.git?depth=1",
			wantURI:   "https://github.com/acme/repo.git",
			wantDepth: 1,
			wantRepo:  "repo",
		},
		{
			name:       "ref and depth",
			raw:        "git::https://github.com/acme/repo.git?ref=main&depth=1",
			wantURI:    "https://github.com/acme/repo.git",
			wantBranch: "main",
			wantDepth:  1,
			wantRepo:   "repo",
		},
		{
			name:     "scp style",
			raw:      "git@github.com:acme/repo.git",
			wantURI:  "git@github.com:acme/repo.git",
			wantRepo: "repo",
		},
		{
			name:       "unknown query param",
			raw:        "git::https://github.com/acme/repo.git?foo=bar",
			wantErrMsg: "unknown query parameter",
		},
		{
			name:       "invalid depth",
			raw:        "git::https://github.com/acme/repo.git?depth=notanumber",
			wantErrMsg: "non-negative integer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCloneURI(tt.raw)
			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantURI, got.URI)
			assert.Equal(t, tt.wantBranch, got.Branch)
			assert.Equal(t, tt.wantDepth, got.Depth)
			assert.Equal(t, tt.wantRepo, got.RepoName)
		})
	}
}

// TestParseCloneURI_FlagBeatsQueryParam verifies flag > query param precedence.
func TestParseCloneURI_FlagBeatsQueryParam(t *testing.T) {
	parsed, err := ParseCloneURI("git::https://github.com/acme/repo.git?ref=main&depth=1")
	require.NoError(t, err)

	// Simulate flag override: flag wins over query param.
	branch := resolveStringPrecedence("feature-branch", parsed.Branch)
	depth := resolveIntPrecedence(5, parsed.Depth)

	assert.Equal(t, "feature-branch", branch, "flag branch should beat URI ref param")
	assert.Equal(t, 5, depth, "flag depth should beat URI depth param")
}

// ---- Argument classification tests ----

func TestClassifyArg(t *testing.T) {
	cfg := &schema.GitConfig{
		Repositories: map[string]schema.GitRepository{
			"flux-deploy": {URI: "https://github.com/acme/flux.git"},
		},
	}

	tests := []struct {
		name string
		arg  string
		want argKind
	}{
		{"configured name", "flux-deploy", argKindName},
		{"path prefix", "./deployments", argKindPath},
		{"abs path", "/tmp/repo", argKindPath},
		{"https uri", "https://github.com/acme/repo.git", argKindURI},
		{"scp uri", "git@github.com:acme/repo.git", argKindURI},
		{"getter prefix", "git::https://github.com/acme/repo.git", argKindURI},
		{"unknown name as path", "unknown-repo", argKindPath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, classifyArg(tt.arg, cfg))
		})
	}
}

func TestClassifyArg_NilConfig(t *testing.T) {
	assert.Equal(t, argKindURI, classifyArg("https://github.com/acme/repo.git", nil))
	assert.Equal(t, argKindPath, classifyArg("some-name", nil))
}

// TestClassifyArg_PathForcesName ensures ./ prefix forces path even for configured names.
func TestClassifyArg_PathForcesName(t *testing.T) {
	cfg := &schema.GitConfig{
		Repositories: map[string]schema.GitRepository{
			"flux-deploy": {URI: "https://github.com/acme/flux.git"},
		},
	}
	// ./flux-deploy should be treated as a path, not a configured name.
	assert.Equal(t, argKindPath, classifyArg("./flux-deploy", cfg))
}

// ---- Signing mode resolution tests ----

func TestResolveSigningMode(t *testing.T) {
	repoAuto := &atmosgit.ResolvedRepository{Signing: atmosgit.SigningAuto}
	repoAlways := &atmosgit.ResolvedRepository{Signing: atmosgit.SigningAlways}

	tests := []struct {
		name     string
		sign     bool
		noSign   bool
		resolved *atmosgit.ResolvedRepository
		want     atmosgit.SigningMode
	}{
		{"--sign wins over repo always", true, false, repoAlways, atmosgit.SigningAlways},
		{"--no-sign wins over repo always", false, true, repoAlways, atmosgit.SigningNever},
		{"repo always when no flags", false, false, repoAlways, atmosgit.SigningAlways},
		{"auto fallback from repo auto", false, false, repoAuto, atmosgit.SigningAuto},
		{"nil resolved falls back to auto", false, false, nil, atmosgit.SigningAuto},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveSigningMode(tt.sign, tt.noSign, tt.resolved)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---- Precedence helpers tests ----

func TestResolveStringPrecedence(t *testing.T) {
	assert.Equal(t, "flag", resolveStringPrecedence("flag", "config"))
	assert.Equal(t, "config", resolveStringPrecedence("", "config"))
	assert.Equal(t, "", resolveStringPrecedence("", ""))
}

func TestResolveIntPrecedence(t *testing.T) {
	assert.Equal(t, 5, resolveIntPrecedence(5, 10))
	assert.Equal(t, 10, resolveIntPrecedence(0, 10))
	assert.Equal(t, 0, resolveIntPrecedence(0, 0))
}

// ---- runClone --all mutual exclusion test ----

func TestRunClone_AllMutualExclusionWithArg(t *testing.T) {
	opts := &cloneOptions{All: true}
	err := runClone(context.Background(), opts, []string{"some-repo"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig), "expected ErrInvalidConfig, got: %v", err)
}

// ---- No-arg clone outside CI → ErrGitRepositoryRequired ----

func TestRunCloneNoArg_NoCIEnabled(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })

	// nil config means CI is not enabled.
	atmosConfigPtr = nil

	err := runCloneNoArg(context.Background(), &cloneOptions{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryRequired), "expected ErrGitRepositoryRequired, got: %v", err)
}

func TestRunCloneNoArg_CIDisabled(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })

	atmosConfigPtr = &schema.AtmosConfiguration{
		CI: schema.CIConfig{Enabled: false},
	}

	err := runCloneNoArg(context.Background(), &cloneOptions{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryRequired), "expected ErrGitRepositoryRequired, got: %v", err)
}

// ---- runPull --all mutual exclusion test ----

func TestRunPull_AllMutualExclusionWithArg(t *testing.T) {
	err := runPull(context.Background(), true, "", "", []string{"some-repo"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig), "expected ErrInvalidConfig, got: %v", err)
}

// ---- runStatus --all mutual exclusion test ----

func TestRunStatus_AllMutualExclusionWithArg(t *testing.T) {
	err := runStatus(context.Background(), true, []string{"some-repo"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig), "expected ErrInvalidConfig, got: %v", err)
}

// ---- --all errors.Join aggregation test ----

// TestRunConcurrent_AttemptAllErrorsJoin verifies that runConcurrent attempts
// all items even when some fail, and returns a joined error.
func TestRunConcurrent_AttemptAllErrorsJoin(t *testing.T) {
	errA := errors.New("failure-a")
	errB := errors.New("failure-b")
	names := []string{"a", "b", "c"}

	callCount := 0
	err := runConcurrent(context.Background(), names, func(_ context.Context, name string) error {
		callCount++
		switch name {
		case "a":
			return errA
		case "b":
			return errB
		default:
			return nil
		}
	})

	// All 3 were attempted.
	assert.Equal(t, 3, callCount, "all repositories should be attempted")

	// Combined error wraps both failures.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failure-a")
	assert.Contains(t, err.Error(), "failure-b")

	// Underlying sentinels are unwrappable from the joined error.
	assert.True(t, errors.Is(err, errA))
	assert.True(t, errors.Is(err, errB))
}

// TestRunConcurrent_Success verifies no error when all succeed.
func TestRunConcurrent_Success(t *testing.T) {
	names := []string{"a", "b", "c"}
	err := runConcurrent(context.Background(), names, func(_ context.Context, _ string) error {
		return nil
	})
	assert.NoError(t, err)
}

// ---- wrapRepoNotFound test ----

func TestWrapRepoNotFound(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })

	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"flux-deploy": {URI: "https://github.com/acme/flux.git"},
			},
		},
	}

	base := errUtils.ErrGitRepositoryNotFound
	err := wrapRepoNotFound(base, "unknown-repo")
	require.Error(t, err)
	// The error chain should unwrap to the base sentinel.
	assert.True(t, errors.Is(err, base), "expected ErrGitRepositoryNotFound in chain, got: %v", err)
	// The base sentinel text is preserved.
	assert.Contains(t, err.Error(), "git repository not configured")
}

// ---- gitConfig helper ----

func TestGitConfig_NilConfig(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })

	atmosConfigPtr = nil
	assert.Nil(t, gitConfig())
}

func TestGitConfig_ReturnsGitSection(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })

	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"my-repo": {URI: "https://example.com/repo.git"},
			},
		},
	}

	cfg := gitConfig()
	require.NotNil(t, cfg)
	assert.Contains(t, cfg.Repositories, "my-repo")
}

// ---- ciRepoURI test ----

func TestCIRepoURI(t *testing.T) {
	uri := ciRepoURI("acme/flux-deploy")
	assert.Equal(t, "https://github.com/acme/flux-deploy.git", uri)
}

// ---- resolveAdHocWorkdir test ----

func TestResolveAdHocWorkdir_FlagOverrides(t *testing.T) {
	workdir, err := resolveAdHocWorkdir("/tmp/custom", "ignored-name")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/custom", workdir)
}

func TestResolveAdHocWorkdir_FallsBackToCwd(t *testing.T) {
	workdir, err := resolveAdHocWorkdir("", "myrepo")
	require.NoError(t, err)
	assert.NotEmpty(t, workdir)
	// Should end in the repo name.
	assert.Contains(t, workdir, "myrepo")
}
