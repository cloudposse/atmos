package git

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	gitsvc "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels: fail the build immediately if the schema fields the
// tests rely on are renamed.
var (
	_ = schema.GitRepository{URI: "", Workdir: "", Auth: schema.GitAuthConfig{Identity: ""}}
	_ = schema.GitConfig{Repositories: nil}
)

// stubProvider is a local gitsvc.Provider stub that records calls. It is
// injected through the Engine seams — the "cli" registry entry is untouched.
type stubProvider struct {
	cloneErr     error
	commitErr    error
	pushErr      error
	commitResult gitsvc.CommitResult

	cloneCalls  []*gitsvc.CloneOptions
	commitCalls []*gitsvc.CommitOptions
	pushCalls   []*gitsvc.PushOptions
}

func (s *stubProvider) Clone(_ context.Context, opts *gitsvc.CloneOptions) error {
	s.cloneCalls = append(s.cloneCalls, opts)
	return s.cloneErr
}

func (s *stubProvider) Pull(_ context.Context, _ *gitsvc.PullOptions) error { return nil }

func (s *stubProvider) Status(_ context.Context, _ *gitsvc.StatusOptions) (*gitsvc.StatusResult, error) {
	return &gitsvc.StatusResult{Clean: true}, nil
}

func (s *stubProvider) Diff(_ context.Context, _ *gitsvc.DiffOptions) (*gitsvc.DiffResult, error) {
	return &gitsvc.DiffResult{}, nil
}

func (s *stubProvider) Commit(_ context.Context, opts *gitsvc.CommitOptions) (*gitsvc.CommitResult, error) {
	s.commitCalls = append(s.commitCalls, opts)
	if s.commitErr != nil {
		return nil, s.commitErr
	}
	result := s.commitResult
	return &result, nil
}

func (s *stubProvider) Push(_ context.Context, opts *gitsvc.PushOptions) error {
	s.pushCalls = append(s.pushCalls, opts)
	return s.pushErr
}

// stubEnvProvider is a local gitsvc.IdentityEnvironmentProvider stub.
type stubEnvProvider struct {
	env map[string]string
	err error
}

func (s stubEnvProvider) EnsureIdentityEnvironment(_ context.Context, _ string) (map[string]string, error) {
	return s.env, s.err
}

// newTestEngine wires an Engine to the stub provider and deterministic seams.
func newTestEngine(stub *stubProvider, root string) *Engine {
	return &Engine{
		newProvider:     func(string) (gitsvc.Provider, error) { return stub, nil },
		resolveRepo:     gitsvc.ResolveRepository,
		currentRepoRoot: func() (string, error) { return root, nil },
		currentSHA:      func() (string, error) { return "deadbeef", nil },
		environ:         func() []string { return []string{"BASE=1"} },
		newEnvProvider: func(*hooks.ExecContext) (gitsvc.IdentityEnvironmentProvider, error) {
			return stubEnvProvider{}, nil
		},
	}
}

// newExecCtx builds an ExecContext bound to the registered git kind so
// on_failure defaults resolve exactly as they do in production RunAll.
func newExecCtx(t *testing.T, hook *hooks.Hook, cfg *schema.AtmosConfiguration) *hooks.ExecContext {
	t.Helper()
	kind, ok := hooks.GetKind(kindName)
	require.True(t, ok, "git kind must be registered")
	return &hooks.ExecContext{
		Hook:        hook,
		Kind:        kind,
		AtmosConfig: cfg,
		Info:        &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev"},
	}
}

func namedRepoConfig(workdir string) *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"artifacts": {
					URI:     "https://example.com/acme/artifacts.git",
					Workdir: workdir,
				},
			},
		},
	}
}

func TestKindRegistration(t *testing.T) {
	kind, ok := hooks.GetKind(kindName)
	require.True(t, ok)
	assert.Equal(t, kindName, kind.Name)
	assert.Equal(t, hooks.OnFailureFail, kind.OnFailure)
	assert.NotNil(t, kind.Engine)
	assert.Empty(t, kind.Command, "git kind must not resolve a binary through the command preflight")
}

func TestEngineRunNilContext(t *testing.T) {
	engine := newTestEngine(&stubProvider{}, t.TempDir())

	_, err := engine.Run(nil)
	require.ErrorIs(t, err, errUtils.ErrNilParam)

	_, err = engine.Run(&hooks.ExecContext{})
	require.ErrorIs(t, err, errUtils.ErrNilParam)
}

func TestEngineRunCurrentRepo(t *testing.T) {
	root := t.TempDir()
	stub := &stubProvider{commitResult: gitsvc.CommitResult{Committed: true, SHA: "abc123"}}
	engine := newTestEngine(stub, root)

	hook := &hooks.Hook{
		Kind:   kindName,
		Commit: &hooks.GitCommitSpec{Paths: []string{"generated/dev/vpc", "docs/inventory.md"}},
	}
	_, err := engine.Run(newExecCtx(t, hook, nil))
	require.NoError(t, err)

	assert.Empty(t, stub.cloneCalls, "current repository must not be cloned/reconciled")
	require.Len(t, stub.commitCalls, 1)
	commit := stub.commitCalls[0]
	assert.Equal(t, root, commit.Workdir)
	assert.Equal(t, []string{"BASE=1"}, commit.Env)
	assert.Equal(t, gitsvc.SigningAuto, commit.Signing)
	assert.Nil(t, commit.Author, "current repository commits use Git's own author config")
	assert.Equal(t, "Update artifacts for vpc in dev", commit.Message)
	require.Len(t, commit.Paths, 2)
	assert.Equal(t, "generated/dev/vpc", commit.Paths[0])
	assert.Equal(t, "docs/inventory.md", commit.Paths[1])
	assert.Equal(t, map[string]string{
		"Atmos-Stack":      "dev",
		"Atmos-Component":  "vpc",
		"Atmos-Source-SHA": "deadbeef",
	}, commit.Trailers)
	assert.Empty(t, stub.pushCalls, "push not requested")
}

func TestEngineRunCurrentRepoWithoutHEAD(t *testing.T) {
	stub := &stubProvider{commitResult: gitsvc.CommitResult{Committed: true, SHA: "abc123"}}
	engine := newTestEngine(stub, t.TempDir())
	engine.currentSHA = func() (string, error) { return "", errors.New("empty repository") }

	hook := &hooks.Hook{Kind: kindName}
	_, err := engine.Run(newExecCtx(t, hook, nil))
	require.NoError(t, err, "missing HEAD must not block publishing")

	require.Len(t, stub.commitCalls, 1)
	assert.NotContains(t, stub.commitCalls[0].Trailers, "Atmos-Source-SHA")
}

func TestEngineRunNamedRepo(t *testing.T) {
	workdir := t.TempDir()
	stub := &stubProvider{commitResult: gitsvc.CommitResult{Committed: true, SHA: "abc123"}}
	engine := newTestEngine(stub, t.TempDir())

	hook := &hooks.Hook{
		Kind:       kindName,
		Repository: "artifacts",
		Commit:     &hooks.GitCommitSpec{Message: "publish artifacts"},
		Push:       true,
	}
	_, err := engine.Run(newExecCtx(t, hook, namedRepoConfig(workdir)))
	require.NoError(t, err)

	require.Len(t, stub.cloneCalls, 1)
	clone := stub.cloneCalls[0]
	assert.Equal(t, "https://example.com/acme/artifacts.git", clone.URI)
	assert.Equal(t, workdir, clone.Workdir)
	assert.Equal(t, gitsvc.DefaultRemote, clone.Remote)

	require.Len(t, stub.commitCalls, 1)
	commit := stub.commitCalls[0]
	assert.Equal(t, workdir, commit.Workdir)
	assert.Equal(t, "publish artifacts", commit.Message)
	assert.NotContains(t, commit.Trailers, "Atmos-Source-SHA", "source SHA is current-repo-only provenance")
	assert.Equal(t, "dev", commit.Trailers["Atmos-Stack"])
	assert.Equal(t, "vpc", commit.Trailers["Atmos-Component"])

	require.Len(t, stub.pushCalls, 1)
	assert.Equal(t, gitsvc.DefaultPushRetries, stub.pushCalls[0].Retries)
	assert.Equal(t, workdir, stub.pushCalls[0].Workdir)
}

func TestEngineRunMissingNamedRepo(t *testing.T) {
	stub := &stubProvider{}
	engine := newTestEngine(stub, t.TempDir())

	hook := &hooks.Hook{Kind: kindName, Repository: "nope"}
	cfg := &schema.AtmosConfiguration{}
	_, err := engine.Run(newExecCtx(t, hook, cfg))
	require.ErrorIs(t, err, errUtils.ErrGitRepositoryNotFound)
	assert.Empty(t, stub.cloneCalls)
	assert.Empty(t, stub.commitCalls)
}

func TestEngineRunNamedRepoRequiresAtmosConfig(t *testing.T) {
	engine := newTestEngine(&stubProvider{}, t.TempDir())

	hook := &hooks.Hook{Kind: kindName, Repository: "artifacts"}
	execCtx := newExecCtx(t, hook, nil)
	_, err := engine.Run(execCtx)
	require.ErrorIs(t, err, errUtils.ErrNilParam)
}

func TestEngineRunDefaultMessageWhenCommitOmitted(t *testing.T) {
	stub := &stubProvider{commitResult: gitsvc.CommitResult{Committed: true}}
	engine := newTestEngine(stub, t.TempDir())

	hook := &hooks.Hook{Kind: kindName}
	_, err := engine.Run(newExecCtx(t, hook, nil))
	require.NoError(t, err)

	require.Len(t, stub.commitCalls, 1)
	assert.Equal(t, "Update artifacts for vpc in dev", stub.commitCalls[0].Message)
	assert.Empty(t, stub.commitCalls[0].Paths)
}

func TestEngineRunConfiguredMessagePassesThrough(t *testing.T) {
	stub := &stubProvider{commitResult: gitsvc.CommitResult{Committed: true}}
	engine := newTestEngine(stub, t.TempDir())

	// Templates are rendered by the hooks engine before Run; the engine must
	// pass the (already rendered) message through verbatim.
	hook := &hooks.Hook{
		Kind:   kindName,
		Commit: &hooks.GitCommitSpec{Message: "Update generated artifacts for vpc in dev"},
	}
	_, err := engine.Run(newExecCtx(t, hook, nil))
	require.NoError(t, err)

	require.Len(t, stub.commitCalls, 1)
	assert.Equal(t, "Update generated artifacts for vpc in dev", stub.commitCalls[0].Message)
}

func TestEngineRunNoChangesIsCleanNoOp(t *testing.T) {
	stub := &stubProvider{commitResult: gitsvc.CommitResult{Committed: false}}
	engine := newTestEngine(stub, t.TempDir())

	hook := &hooks.Hook{Kind: kindName, Push: true}
	_, err := engine.Run(newExecCtx(t, hook, nil))
	require.NoError(t, err, "no changes must be a clean no-op")

	require.Len(t, stub.commitCalls, 1)
	assert.Empty(t, stub.pushCalls, "no push without a commit")
}

func TestEngineRunPushOnlyWhenRequested(t *testing.T) {
	tests := []struct {
		name      string
		push      bool
		wantPush  int
		committed bool
	}{
		{name: "push requested and committed", push: true, committed: true, wantPush: 1},
		{name: "push not requested", push: false, committed: true, wantPush: 0},
		{name: "push requested but nothing committed", push: true, committed: false, wantPush: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubProvider{commitResult: gitsvc.CommitResult{Committed: tt.committed, SHA: "abc"}}
			engine := newTestEngine(stub, t.TempDir())

			hook := &hooks.Hook{Kind: kindName, Push: tt.push}
			_, err := engine.Run(newExecCtx(t, hook, nil))
			require.NoError(t, err)
			assert.Len(t, stub.pushCalls, tt.wantPush)
		})
	}
}

func TestEngineRunRejectsPathEscapingWorktree(t *testing.T) {
	stub := &stubProvider{}
	engine := newTestEngine(stub, t.TempDir())

	hook := &hooks.Hook{
		Kind:   kindName,
		Commit: &hooks.GitCommitSpec{Paths: []string{"../outside"}},
	}
	_, err := engine.Run(newExecCtx(t, hook, nil))
	require.ErrorIs(t, err, errUtils.ErrGitPathEscapesWorktree)
	assert.Empty(t, stub.cloneCalls, "no Git operation may run after path validation fails")
	assert.Empty(t, stub.commitCalls, "no Git operation may run after path validation fails")
}

func TestEngineRunIdentityEnvironment(t *testing.T) {
	workdir := t.TempDir()
	stub := &stubProvider{commitResult: gitsvc.CommitResult{Committed: true}}
	engine := newTestEngine(stub, t.TempDir())

	envProviderCalls := 0
	engine.newEnvProvider = func(*hooks.ExecContext) (gitsvc.IdentityEnvironmentProvider, error) {
		envProviderCalls++
		return stubEnvProvider{env: map[string]string{"GIT_CONFIG_KEY_0": "credential.helper"}}, nil
	}

	cfg := namedRepoConfig(workdir)
	repo := cfg.Git.Repositories["artifacts"]
	repo.Auth = schema.GitAuthConfig{Identity: "ci"}
	cfg.Git.Repositories["artifacts"] = repo

	hook := &hooks.Hook{Kind: kindName, Repository: "artifacts"}
	_, err := engine.Run(newExecCtx(t, hook, cfg))
	require.NoError(t, err)

	assert.Equal(t, 1, envProviderCalls)
	require.Len(t, stub.commitCalls, 1)
	assert.Contains(t, stub.commitCalls[0].Env, "BASE=1")
	assert.Contains(t, stub.commitCalls[0].Env, "GIT_CONFIG_KEY_0=credential.helper")
}

func TestEngineRunNoIdentitySkipsEnvProvider(t *testing.T) {
	workdir := t.TempDir()
	stub := &stubProvider{commitResult: gitsvc.CommitResult{Committed: true}}
	engine := newTestEngine(stub, t.TempDir())

	envProviderCalls := 0
	engine.newEnvProvider = func(*hooks.ExecContext) (gitsvc.IdentityEnvironmentProvider, error) {
		envProviderCalls++
		return stubEnvProvider{}, nil
	}

	hook := &hooks.Hook{Kind: kindName, Repository: "artifacts"}
	_, err := engine.Run(newExecCtx(t, hook, namedRepoConfig(workdir)))
	require.NoError(t, err)

	assert.Zero(t, envProviderCalls, "no identity configured: plain process env, no auth manager")
	require.Len(t, stub.commitCalls, 1)
	assert.Equal(t, []string{"BASE=1"}, stub.commitCalls[0].Env)
}

func TestEngineRunEnvProviderErrorPropagates(t *testing.T) {
	workdir := t.TempDir()
	stub := &stubProvider{}
	engine := newTestEngine(stub, t.TempDir())

	wantErr := errors.New("auth manager construction failed")
	engine.newEnvProvider = func(*hooks.ExecContext) (gitsvc.IdentityEnvironmentProvider, error) {
		return nil, wantErr
	}

	cfg := namedRepoConfig(workdir)
	repo := cfg.Git.Repositories["artifacts"]
	repo.Auth = schema.GitAuthConfig{Identity: "ci"}
	cfg.Git.Repositories["artifacts"] = repo

	hook := &hooks.Hook{Kind: kindName, Repository: "artifacts"}
	_, err := engine.Run(newExecCtx(t, hook, cfg))
	require.ErrorIs(t, err, wantErr)
	assert.Empty(t, stub.cloneCalls)
}

func TestEngineRunCurrentRepoRootErrorPropagates(t *testing.T) {
	stub := &stubProvider{}
	engine := newTestEngine(stub, t.TempDir())

	wantErr := errors.New("not a git repository")
	engine.currentRepoRoot = func() (string, error) { return "", wantErr }

	hook := &hooks.Hook{Kind: kindName}
	_, err := engine.Run(newExecCtx(t, hook, nil))
	require.ErrorIs(t, err, wantErr)
	assert.Empty(t, stub.commitCalls)
}

func TestEngineRunOnFailureModes(t *testing.T) {
	commitErr := errors.New("commit failed")
	tests := []struct {
		name      string
		onFailure string
		wantErr   bool
	}{
		{name: "kind default fails", onFailure: "", wantErr: true},
		{name: "explicit fail", onFailure: hooks.OnFailureFail, wantErr: true},
		{name: "warn swallows", onFailure: hooks.OnFailureWarn, wantErr: false},
		{name: "ignore swallows", onFailure: hooks.OnFailureIgnore, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubProvider{commitErr: commitErr}
			engine := newTestEngine(stub, t.TempDir())

			hook := &hooks.Hook{Kind: kindName, OnFailure: tt.onFailure}
			_, err := engine.Run(newExecCtx(t, hook, nil))
			if tt.wantErr {
				require.ErrorIs(t, err, commitErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestEngineRunCloneErrorAppliesOnFailure(t *testing.T) {
	workdir := t.TempDir()
	cloneErr := errors.New("clone failed")
	stub := &stubProvider{cloneErr: cloneErr}
	engine := newTestEngine(stub, t.TempDir())

	hook := &hooks.Hook{Kind: kindName, Repository: "artifacts", OnFailure: hooks.OnFailureFail}
	_, err := engine.Run(newExecCtx(t, hook, namedRepoConfig(workdir)))
	require.ErrorIs(t, err, cloneErr)
	assert.Empty(t, stub.commitCalls, "commit must not run when reconcile fails")
}

func TestEngineRunPushErrorAppliesOnFailure(t *testing.T) {
	pushErr := errors.New("push rejected")
	stub := &stubProvider{
		commitResult: gitsvc.CommitResult{Committed: true, SHA: "abc"},
		pushErr:      pushErr,
	}
	engine := newTestEngine(stub, t.TempDir())

	hook := &hooks.Hook{Kind: kindName, Push: true, OnFailure: hooks.OnFailureWarn}
	_, err := engine.Run(newExecCtx(t, hook, nil))
	require.NoError(t, err, "on_failure: warn must swallow push errors")
	require.Len(t, stub.pushCalls, 1)
}

func TestNewEngineWiresProductionSeams(t *testing.T) {
	engine := NewEngine()
	require.NotNil(t, engine.newProvider)
	require.NotNil(t, engine.resolveRepo)
	require.NotNil(t, engine.currentRepoRoot)
	require.NotNil(t, engine.currentSHA)
	require.NotNil(t, engine.environ)
	require.NotNil(t, engine.newEnvProvider)

	// The default provider seam resolves the registered "cli" provider.
	provider, err := engine.newProvider("")
	require.NoError(t, err)
	assert.NotNil(t, provider)
}

func TestNewAuthEnvProviderBuildsManager(t *testing.T) {
	execCtx := &hooks.ExecContext{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			ComponentAuthSection: map[string]any{},
		},
	}
	provider, err := newAuthEnvProvider(execCtx)
	require.NoError(t, err)
	assert.NotNil(t, provider)
}

func TestNewAuthEnvProviderRejectsInvalidAuthSection(t *testing.T) {
	execCtx := &hooks.ExecContext{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			ComponentAuthSection: map[string]any{
				"identities": "not-a-map",
			},
		},
	}
	_, err := newAuthEnvProvider(execCtx)
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
}
