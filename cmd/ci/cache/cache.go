// Package cache implements the `atmos ci cache` subcommand group
// (restore/save/list/delete) for the CI build cache. The commands are attached
// to the parent `ci` command via Command(); business logic lives in
// pkg/ci/cache so the CLI and the automatic lifecycle hooks share one
// implementation.
package cache

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cipkg "github.com/cloudposse/atmos/pkg/ci"
	cachepkg "github.com/cloudposse/atmos/pkg/ci/cache"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fieldKey is the shared literal for the "key" flag name and "key" log field.
const fieldKey = "key"

// cacheCmd is the parent for `atmos ci cache` subcommands.
var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage the CI build cache (restore/save/list/delete)",
	Long: `Manage the CI build cache.

The CI cache restores a well-known cache directory (the toolchain install path
and anything else under the Atmos XDG cache root) and saves it back, using the
active CI provider's cache store (e.g. the GitHub Actions cache). The full
lifecycle can run in a single Atmos invocation (auto restore-on-start +
save-on-end) or be spread across steps with the restore/save subcommands.

Saving and restoring content require running inside a supported CI provider
(GitHub Actions today); outside CI these commands report that the cache is
unavailable.`,
}

// Command returns the `cache` command group with its subcommands attached.
// The parent `ci` command calls this to mount the group.
func Command() *cobra.Command {
	return cacheCmd
}

func init() {
	cacheCmd.AddCommand(cacheRestoreCmd)
	cacheCmd.AddCommand(cacheSaveCmd)
	cacheCmd.AddCommand(cacheListCmd)
	cacheCmd.AddCommand(cacheDeleteCmd)
	cacheCmd.AddCommand(cachePathsCmd)
}

// buildConfigAndStacksInfo creates ConfigAndStacksInfo from global flags so the
// cache commands honor --config-path, --base-path, --profile, etc.
func buildConfigAndStacksInfo(globalFlags *global.Flags) schema.ConfigAndStacksInfo {
	if globalFlags == nil {
		return schema.ConfigAndStacksInfo{}
	}
	return schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		ProfilesFromArg:         globalFlags.Profile,
	}
}

// resolveCacheConfig loads config, validates that the cache is enabled, applies
// CLI overrides, and resolves the cache configuration. It does NOT detect a
// backend or require a CI runtime token, so it is usable by `atmos ci cache
// paths` (which only needs the key/paths) as well as forming the first half of
// cacheSetup. It is a package-level var so tests can stub it.
var resolveCacheConfig = func(cmd *cobra.Command, overrides cacheOverrides) (*cachepkg.Config, error) {
	globalFlags := flags.ParseGlobalFlags(cmd, viper.GetViper())

	atmosConfig, err := cfg.InitCliConfig(buildConfigAndStacksInfo(&globalFlags), false)
	if err != nil {
		return nil, errUtils.Build(err).
			WithHint("Verify your atmos.yaml syntax and configuration").
			Err()
	}

	if !atmosConfig.CI.Cache.Enabled {
		return nil, errUtils.Build(errUtils.ErrCacheUnavailable).
			WithExplanation("The CI cache is disabled in configuration").
			WithHint("Set ci.cache.enabled: true in atmos.yaml (or ATMOS_CI_CACHE_ENABLED=true) to use the cache").
			Err()
	}

	// Apply CLI overrides onto a local copy of the cache config before resolving.
	overrides.apply(&atmosConfig.CI.Cache)

	return cachepkg.ResolveConfig(&atmosConfig)
}

// cacheSetup resolves the cache configuration and detects the active CI
// provider's cache backend, returning a ready Manager. It is the in-runner path
// used by the save/restore subcommands: it requires an actively-detected CI
// provider, so running `atmos ci cache save`/`restore` outside a runner reports a
// clear, actionable error. It is a package-level var so tests can stub it to
// exercise the command bodies without a live CI runner.
var cacheSetup = func(cmd *cobra.Command, overrides cacheOverrides) (*cachepkg.Manager, *cachepkg.Config, error) {
	resolved, err := resolveCacheConfig(cmd, overrides)
	if err != nil {
		return nil, nil, err
	}

	backend, err := cipkg.DetectCache()
	if err != nil {
		return nil, nil, errUtils.Build(errUtils.ErrCacheUnavailable).
			WithCause(err).
			WithExplanation("Saving and restoring cache content runs only inside a supported CI runner (e.g. GitHub Actions)").
			WithHint("Run save/restore from a CI workflow; to manage the cache from your workstation use `atmos ci cache list` and `atmos ci cache delete`").
			Err()
	}

	return cachepkg.NewManager(backend, resolved), resolved, nil
}

// cacheAdminSetup resolves the cache configuration and an admin-capable cache
// backend (list/delete) that works outside a CI runner. Cache administration
// uses the provider's public API plus a token, so it must be usable locally —
// it does not require an active CI runtime. It is shared by the list/delete
// subcommands and is a package-level var so tests can stub it.
var cacheAdminSetup = func(cmd *cobra.Command, overrides cacheOverrides) (*cachepkg.Manager, *cachepkg.Config, error) {
	resolved, err := resolveCacheConfig(cmd, overrides)
	if err != nil {
		return nil, nil, err
	}

	backend, err := cipkg.ResolveAdminCache()
	if err != nil {
		return nil, nil, errUtils.Build(errUtils.ErrCacheUnavailable).
			WithCause(err).
			WithExplanation("No cache-capable CI provider could be resolved for this repository").
			WithHint("Run inside a GitHub repository with a token available (GITHUB_TOKEN/ATMOS_GITHUB_TOKEN or `gh auth login`) so the cache can be administered").
			Err()
	}

	return cachepkg.NewManager(backend, resolved), resolved, nil
}

// cacheOverrides carries CLI flag overrides for the resolved cache config.
type cacheOverrides struct {
	key   string
	root  string
	paths []string
}

// apply copies any set overrides onto the schema cache config.
func (o cacheOverrides) apply(cc *schema.CICacheConfig) {
	if o.key != "" {
		cc.Key = o.key
	}
	if o.root != "" {
		cc.Root = o.root
	}
	if len(o.paths) > 0 {
		cc.Paths = o.paths
	}
}
