package git

import (
	"context"
	"fmt"
	"os"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	gitsvc "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/hooks"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Provenance trailer keys appended to every published commit.
const (
	trailerStack     = "Atmos-Stack"
	trailerComponent = "Atmos-Component"
	trailerSourceSHA = "Atmos-Source-SHA"
)

// Engine publishes commits through the shared Git service (pkg/git). It is a
// thin orchestrator: repository resolution, environment composition, path
// validation, commit, and push all delegate to pkg/git; failure semantics
// delegate to the hooks engine via hooks.ApplyOnFailure.
type Engine struct {
	// Seams for tests; NewEngine wires the production implementations.
	newProvider     func(name string) (gitsvc.Provider, error)
	resolveRepo     func(cfg *schema.GitConfig, name string) (*gitsvc.ResolvedRepository, error)
	currentRepoRoot func() (string, error)
	currentSHA      func() (string, error)
	environ         func() []string
	newEnvProvider  func(execCtx *hooks.ExecContext) (gitsvc.IdentityEnvironmentProvider, error)
}

// NewEngine returns a git hook engine wired to the production Git service,
// process environment, and Atmos Auth manager.
func NewEngine() *Engine {
	return &Engine{
		newProvider:     gitsvc.NewProvider,
		resolveRepo:     gitsvc.ResolveRepository,
		currentRepoRoot: gitsvc.GetRoot,
		currentSHA:      gitsvc.GetCurrentCommitSHA,
		environ:         os.Environ,
		newEnvProvider:  newAuthEnvProvider,
	}
}

// publishTarget is the fully resolved destination for one publish operation.
type publishTarget struct {
	provider gitsvc.Provider
	repoCtx  gitsvc.RepoContext
	// clone is non-nil for managed repositories only; Clone reconciles the
	// workdir (clone when absent, fetch/fast-forward otherwise).
	clone       *gitsvc.CloneOptions
	signing     gitsvc.SigningMode
	author      *gitsvc.Author
	pushRetries int
	// sourceSHA is the current repository HEAD, recorded as provenance when
	// cheaply available (current-repository case only).
	sourceSHA string
}

// Run satisfies hooks.Engine. Resolution errors (unknown repository, path
// traversal, nil context) are configuration errors and propagate raw;
// operational errors (clone/commit/push) go through the hook's on_failure
// mode, mirroring the built-in command engine.
func (e *Engine) Run(ctx *hooks.ExecContext) (*hooks.Output, error) {
	defer perf.Track(nil, "hooks.kinds.git.Engine.Run")()

	if err := validateCtx(ctx); err != nil {
		return nil, err
	}

	target, err := e.resolveTarget(ctx)
	if err != nil {
		return nil, err
	}

	if err := e.publish(ctx, target); err != nil {
		return nil, hooks.ApplyOnFailure(ctx, err)
	}
	return nil, nil
}

// validateCtx checks the engine's preconditions on the ExecContext.
func validateCtx(ctx *hooks.ExecContext) error {
	if ctx == nil || ctx.Hook == nil {
		return errUtils.ErrNilParam
	}
	return nil
}

// resolveTarget builds the publish target for the hook: a managed repository
// when `repository` is set, the current repository otherwise. Commit paths
// are validated against the resolved workdir before any Git operation runs.
func (e *Engine) resolveTarget(ctx *hooks.ExecContext) (*publishTarget, error) {
	var target *publishTarget
	var err error
	if ctx.Hook.Repository != "" {
		target, err = e.resolveNamedTarget(ctx)
	} else {
		target, err = e.resolveCurrentTarget()
	}
	if err != nil {
		return nil, err
	}

	if err := validateCommitPaths(ctx.Hook, target.repoCtx.Workdir); err != nil {
		return nil, err
	}
	return target, nil
}

// resolveNamedTarget resolves a managed repository under git.repositories,
// composing the identity environment and preparing reconcile (clone) options.
func (e *Engine) resolveNamedTarget(ctx *hooks.ExecContext) (*publishTarget, error) {
	if ctx.AtmosConfig == nil {
		return nil, fmt.Errorf("%w: git hook with a named repository requires Atmos configuration", errUtils.ErrNilParam)
	}

	resolved, err := e.resolveRepo(&ctx.AtmosConfig.Git, ctx.Hook.Repository)
	if err != nil {
		return nil, err
	}

	env, err := e.composeIdentityEnv(ctx, resolved.Identity)
	if err != nil {
		return nil, err
	}

	provider, err := e.newProvider(resolved.Provider)
	if err != nil {
		return nil, err
	}

	repoCtx := gitsvc.RepoContext{
		Workdir: resolved.Workdir,
		Remote:  resolved.Remote,
		Branch:  resolved.Branch,
		Env:     env,
	}
	return &publishTarget{
		provider: provider,
		repoCtx:  repoCtx,
		clone: &gitsvc.CloneOptions{
			RepoContext:  repoCtx,
			URI:          resolved.URI,
			Depth:        resolved.Clone.Depth,
			Filter:       resolved.Clone.Filter,
			SingleBranch: resolved.Clone.SingleBranch,
			Submodules:   resolved.Clone.Submodules,
		},
		signing:     resolved.Signing,
		author:      resolved.Author,
		pushRetries: resolved.PushRetries,
	}, nil
}

// resolveCurrentTarget targets the repository the component already lives in:
// no clone/reconcile, no identity resolution (ambient credentials and the
// developer's own Git config apply), signing and author left to Git config.
func (e *Engine) resolveCurrentTarget() (*publishTarget, error) {
	root, err := e.currentRepoRoot()
	if err != nil {
		return nil, err
	}

	provider, err := e.newProvider("")
	if err != nil {
		return nil, err
	}

	// Source SHA is best-effort provenance; absence (e.g. an empty
	// repository) must not block publishing.
	sourceSHA, err := e.currentSHA()
	if err != nil {
		log.Debug("Could not resolve current repository HEAD for provenance", "error", err)
		sourceSHA = ""
	}

	return &publishTarget{
		provider:    provider,
		repoCtx:     gitsvc.RepoContext{Workdir: root, Env: e.environ()},
		signing:     gitsvc.SigningAuto,
		pushRetries: gitsvc.DefaultPushRetries,
		sourceSHA:   sourceSHA,
	}, nil
}

// composeIdentityEnv overlays the identity environment for the repository's
// configured Atmos Auth identity onto the process environment. An empty
// identity returns the process environment unchanged.
func (e *Engine) composeIdentityEnv(ctx *hooks.ExecContext, identity string) ([]string, error) {
	base := e.environ()
	if identity == "" {
		return base, nil
	}

	envProvider, err := e.newEnvProvider(ctx)
	if err != nil {
		return nil, err
	}
	return gitsvc.ComposeEnvironment(context.Background(), base, identity, envProvider)
}

// validateCommitPaths ensures every configured commit path stays inside the
// resolved worktree (ErrGitPathEscapesWorktree otherwise).
func validateCommitPaths(hook *hooks.Hook, workdir string) error {
	if hook.Commit == nil {
		return nil
	}
	for _, p := range hook.Commit.Paths {
		if _, err := gitsvc.ValidateRepoRelativePath(workdir, p); err != nil {
			return err
		}
	}
	return nil
}

// publish performs the Git operations against the resolved target:
// reconcile (managed repositories only), commit, and push (when requested
// and a commit was actually created). A no-change commit is a clean no-op.
func (e *Engine) publish(ctx *hooks.ExecContext, target *publishTarget) error {
	runCtx := context.Background()

	if target.clone != nil {
		if err := target.provider.Clone(runCtx, target.clone); err != nil {
			return err
		}
	}

	result, err := target.provider.Commit(runCtx, commitOptions(ctx, target))
	if err != nil {
		return err
	}
	if !result.Committed {
		log.Info(
			"No changes to publish",
			"component", componentName(ctx),
			"stack", stackName(ctx),
			"workdir", target.repoCtx.Workdir,
		)
		return nil
	}
	log.Info(
		"Published commit",
		"sha", result.SHA,
		"component", componentName(ctx),
		"stack", stackName(ctx),
	)

	if !ctx.Hook.Push {
		return nil
	}
	return target.provider.Push(runCtx, &gitsvc.PushOptions{
		RepoContext: target.repoCtx,
		Retries:     target.pushRetries,
	})
}

// commitOptions assembles the provider commit options: rendered (or default)
// message, configured paths, repository signing/author settings, and
// provenance trailers.
func commitOptions(ctx *hooks.ExecContext, target *publishTarget) *gitsvc.CommitOptions {
	var paths []string
	if ctx.Hook.Commit != nil {
		paths = ctx.Hook.Commit.Paths
	}
	return &gitsvc.CommitOptions{
		RepoContext: target.repoCtx,
		Message:     commitMessage(ctx),
		Paths:       paths,
		Signing:     target.signing,
		Author:      target.author,
		Trailers:    provenanceTrailers(ctx, target),
	}
}

// commitMessage returns the configured message (already rendered by the hooks
// engine's template pass) or the default when none is configured.
func commitMessage(ctx *hooks.ExecContext) string {
	if ctx.Hook.Commit != nil && ctx.Hook.Commit.Message != "" {
		return ctx.Hook.Commit.Message
	}
	return fmt.Sprintf("Update artifacts for %s in %s", componentName(ctx), stackName(ctx))
}

// provenanceTrailers builds the Atmos provenance trailers appended to the
// commit message by the provider.
func provenanceTrailers(ctx *hooks.ExecContext, target *publishTarget) map[string]string {
	trailers := map[string]string{
		trailerStack:     stackName(ctx),
		trailerComponent: componentName(ctx),
	}
	if target.sourceSHA != "" {
		trailers[trailerSourceSHA] = target.sourceSHA
	}
	return trailers
}

// stackName returns the stack from the execution context ("" when unknown).
func stackName(ctx *hooks.ExecContext) string {
	if ctx.Info == nil {
		return ""
	}
	return ctx.Info.Stack
}

// componentName returns the component from the execution context ("" when unknown).
func componentName(ctx *hooks.ExecContext) string {
	if ctx.Info == nil {
		return ""
	}
	return ctx.Info.ComponentFromArg
}

// newAuthEnvProvider builds an Atmos Auth manager from the component's auth
// section, mirroring how pkg/auth constructs its manager in hook context
// (pkg/auth/hooks.go). The returned manager satisfies
// gitsvc.IdentityEnvironmentProvider via EnsureIdentityEnvironment.
func newAuthEnvProvider(execCtx *hooks.ExecContext) (gitsvc.IdentityEnvironmentProvider, error) {
	var authConfig schema.AuthConfig
	if execCtx.Info != nil {
		if err := mapstructure.Decode(execCtx.Info.ComponentAuthSection, &authConfig); err != nil {
			return nil, fmt.Errorf("%w: failed to decode component auth config for git hook: %w", errUtils.ErrInvalidAuthConfig, err)
		}
	}

	cliConfigPath := ""
	if execCtx.AtmosConfig != nil {
		cliConfigPath = execCtx.AtmosConfig.CliConfigPath
	}

	manager, err := auth.NewAuthManager(
		&authConfig,
		credentials.NewCredentialStoreWithConfig(&authConfig),
		validation.NewValidator(),
		execCtx.Info,
		cliConfigPath,
	)
	if err != nil {
		return nil, fmt.Errorf("creating auth manager for git hook identity environment: %w", err)
	}
	return manager, nil
}
