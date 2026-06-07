// Package mirror implements eager, multi-platform pre-seeding of the Terraform
// registry cache. It is the eager counterpart to the lazy proxy in
// pkg/terraform/cache: where the proxy populates the cache on demand for the host
// platform during a normal `init`, the mirror uses `terraform/tofu providers
// mirror` to pre-fetch a component's required providers for every configured
// platform into the same canonical filesystem_mirror layout. The shared directory
// then serves three ways: lazily (proxy), eagerly (mirror), and offline
// (filesystem_mirror) — the foundation for air-gapped reproducible builds.
//
// Mirroring providers has no inter-component dependencies, so multi-component runs
// (--all/--components/--query) iterate the selected components in no particular
// order, unlike the dependency-ordered plan/apply scheduler.
package mirror

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/dustin/go-humanize"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	tfcache "github.com/cloudposse/atmos/pkg/terraform/cache"
	"github.com/cloudposse/atmos/pkg/ui"
)

// mirrorDirPerm is the permission for the providers mirror directory.
const mirrorDirPerm = 0o755

// providersDirName is the sub-directory of the cache root holding mirrored providers.
// It matches the canonical layout used by the proxy (pkg/terraform/registry).
const providersDirName = "providers"

// Options configures a mirror run. Targeting mirrors the multi-component flags of
// `atmos terraform plan`: a single component, --all, --components, or --query, with
// an optional --stack scope.
type Options struct {
	// Component is the single Atmos component to mirror. Empty when a multi-component
	// selector (All/Components/Query) or a bare stack is used.
	Component string
	// Stack is the Atmos stack. Required for a single component; an optional scope for
	// the multi-component selectors.
	Stack string
	// PlatformsFlag is the optional CLI override for target platforms. When empty,
	// the configured platforms (else the host platform) are used.
	PlatformsFlag []string
	// All mirrors every component in every stack (or in Stack, when set).
	All bool
	// Components filters the mirror to specific components.
	Components []string
	// Query filters the mirror with a YQ expression.
	Query string
	// Format selects the output: "" (human) streams the mirror progress and prints a
	// summary; "json"/"yaml" suppress the progress and emit a structured result.
	Format string
}

// Target identifies a mirrored component/stack in the result.
type Target struct {
	Component string `json:"component" yaml:"component"`
	Stack     string `json:"stack" yaml:"stack"`
}

// CacheStats is the on-disk cache summary included in the result.
type CacheStats struct {
	Objects        int    `json:"objects" yaml:"objects"`
	Providers      int    `json:"providers" yaml:"providers"`
	Modules        int    `json:"modules" yaml:"modules"`
	TotalSizeBytes int64  `json:"total_size_bytes" yaml:"total_size_bytes"`
	TotalSize      string `json:"total_size" yaml:"total_size"`
}

// Result is the structured outcome of a mirror run, for --format json|yaml.
type Result struct {
	Components []Target   `json:"components" yaml:"components"`
	Platforms  []string   `json:"platforms" yaml:"platforms"`
	Cache      CacheStats `json:"cache" yaml:"cache"`
}

// Run resolves the target platforms and cache root, then mirrors the selected
// components' required providers into <root>/providers for every platform by driving
// `terraform/tofu providers mirror` through the standard Atmos execution pipeline.
func Run(opts Options) error { //nolint:gocritic // Options is the public command input; value semantics are intentional.
	defer perf.Track(nil, "tfmirror.Run")()

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
		ComponentFromArg: opts.Component,
		Stack:            opts.Stack,
	}, true)
	if err != nil {
		return err
	}

	platforms := ResolvePlatforms(opts.PlatformsFlag, atmosConfig.Components.Terraform.Platforms)

	root, err := tfcache.ResolveRoot(&atmosConfig)
	if err != nil {
		return err
	}
	providersDir := filepath.Join(root, providersDirName)
	if err := os.MkdirAll(providersDir, mirrorDirPerm); err != nil {
		return fmt.Errorf("%w: failed to create mirror directory %q: %w", errUtils.ErrInvalidConfig, providersDir, err)
	}

	args := buildMirrorArgs(platforms, providersDir)

	targets, err := resolveTargets(&atmosConfig, &opts)
	if err != nil {
		return err
	}

	// Start the registry cache proxy ONCE for the whole run (its "listening"/"cert"
	// messages print here, before any TUI), shared across every component, and closed
	// once after the run.
	cacheSetup, closeCache, err := startSharedCache(context.Background(), &atmosConfig)
	if err != nil {
		return err
	}
	defer closeCache()

	if err := runMirror(opts.Format, targets, args, cacheSetup, len(platforms)); err != nil {
		return err
	}
	return emitResult(opts.Format, root, targets, platforms)
}

// buildMirrorArgs builds the `providers mirror` arguments: repeatable -platform flags
// followed by the target directory (positional, after the flags).
func buildMirrorArgs(platforms []string, providersDir string) []string {
	args := make([]string, 0, len(platforms)+1)
	for _, p := range platforms {
		args = append(args, "-platform="+p)
	}
	return append(args, providersDir)
}

// startSharedCache starts the registry cache proxy once and returns it plus a cleanup
// func. Returns a no-op cleanup when caching is disabled.
func startSharedCache(ctx context.Context, atmosConfig *schema.AtmosConfiguration) (*tfcache.Setup, func(), error) {
	setup, err := tfcache.Start(ctx, atmosConfig)
	if err != nil {
		return nil, func() {}, err
	}
	if setup == nil {
		return nil, func() {}, nil
	}
	cleanup := func() {
		if closeErr := setup.Close(ctx); closeErr != nil {
			log.Debug("Failed to shut down Terraform registry cache", "error", closeErr)
		}
	}
	if trustErr := setup.VerifyTrust(ctx); trustErr != nil {
		cleanup()
		return nil, func() {}, trustErr
	}
	return setup, cleanup, nil
}

// runMirror mirrors every target: sequentially (suppressing output) for json/yaml, or
// through the package-manager-style TUI for human output.
func runMirror(format string, targets []Target, args []string, cacheSetup *tfcache.Setup, platforms int) error {
	if format == "json" || format == "yaml" {
		for i := range targets {
			t := targets[i]
			if err := mirrorComponent(t, args, cacheSetup, io.Discard); err != nil {
				return fmt.Errorf("%w: component=%s stack=%s: %w", errUtils.ErrTerraformExecFailed, t.Component, t.Stack, err)
			}
		}
		return nil
	}
	return executeMirrorModel(targets, args, cacheSetup, platforms)
}

// Reports whether a multi-component selector (--all/--components/--query) was given.
func (o *Options) hasSelector() bool {
	return o.All || len(o.Components) > 0 || o.Query != ""
}

// resolveTargets determines the component/stack pairs to mirror from the options.
func resolveTargets(atmosConfig *schema.AtmosConfiguration, opts *Options) ([]Target, error) {
	// Single component: a component arg with no multi-component selector.
	if opts.Component != "" && !opts.hasSelector() {
		if opts.Stack == "" {
			return nil, errUtils.ErrMissingStack
		}
		return []Target{{Component: opts.Component, Stack: opts.Stack}}, nil
	}

	// No component and no selector requires at least a stack to scope the mirror.
	if !opts.hasSelector() && opts.Stack == "" {
		return nil, fmt.Errorf("%w: specify a component, --all, --components, --query, or --stack", errUtils.ErrMissingComponent)
	}

	found, err := e.ListTerraformComponentTargets(atmosConfig, opts.Stack, opts.Components, opts.Query)
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("%w: no terraform components matched the selection", errUtils.ErrMissingComponent)
	}
	targets := make([]Target, 0, len(found))
	for i := range found {
		targets = append(targets, Target{Component: found[i].Component, Stack: found[i].Stack})
	}
	return targets, nil
}

// mirrorComponent runs `providers mirror` for a single component/stack into the cache.
// SubCommand is the space-joined "providers mirror"; the pipeline splits it with
// strings.Fields, matching the existing `atmos terraform providers mirror` passthrough
// (see internal/exec/cli_utils.go parseCompoundSubcommand). Init is skipped: providers
// mirror reads required_providers directly and does not need an initialized workdir.
//
// The shared cacheSetup is reused (TerraformCacheExternal) so a single proxy serves
// every component. The stdout writer receives the subprocess output: io.Discard for
// structured output, or a streaming scanner that drives the TUI. Pass nil to inherit
// os.Stdout.
func mirrorComponent(t Target, args []string, cacheSetup *tfcache.Setup, stdout io.Writer) error {
	info := schema.ConfigAndStacksInfo{
		ComponentFromArg:       t.Component,
		ComponentType:          cfg.TerraformComponentType,
		Stack:                  t.Stack,
		StackFromArg:           t.Stack,
		SubCommand:             "providers mirror",
		AdditionalArgsAndFlags: args,
		SkipInit:               true,
		TerraformCache:         cacheSetup,
		TerraformCacheExternal: true,
	}

	var shellOpts []e.ShellCommandOption
	if stdout != nil {
		// Override (not tee) so the provider-mirror progress does not reach the
		// terminal directly — it is parsed (TUI) or discarded (structured output).
		shellOpts = append(shellOpts, e.WithStdoutOverride(stdout))
	}
	return e.ExecuteTerraform(info, shellOpts...)
}

// Print the run summary: a human cache-size line, or a structured Result for
// --format json|yaml.
func emitResult(format, root string, targets []Target, platforms []string) error {
	summary, err := tfcache.Summarize(root)
	if err != nil {
		return err
	}
	//nolint:gosec // cache size is non-negative.
	stats := CacheStats{
		Objects:        summary.ObjectCount,
		Providers:      summary.Providers,
		Modules:        summary.Modules,
		TotalSizeBytes: summary.TotalSize,
		TotalSize:      humanize.Bytes(uint64(summary.TotalSize)),
	}

	switch format {
	case "json":
		return data.WriteJSON(Result{Components: targets, Platforms: platforms, Cache: stats})
	case "yaml":
		return data.WriteYAML(Result{Components: targets, Platforms: platforms, Cache: stats})
	default:
		ui.Success(fmt.Sprintf("Cache holds %d object(s) (%d providers, %d modules), %s on disk",
			stats.Objects, stats.Providers, stats.Modules, stats.TotalSize))
		return nil
	}
}

// ResolvePlatforms applies the platform precedence: the CLI flag wins, then the
// configured components.terraform.platforms, then the current host platform
// (<os>_<arch>).
func ResolvePlatforms(flag, configured []string) []string {
	defer perf.Track(nil, "mirror.ResolvePlatforms")()

	if len(flag) > 0 {
		return flag
	}
	if len(configured) > 0 {
		return configured
	}
	return []string{runtime.GOOS + "_" + runtime.GOARCH}
}
