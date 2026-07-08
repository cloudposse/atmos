package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cachepkg "github.com/cloudposse/atmos/pkg/ci/cache"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/flags"
	ghactions "github.com/cloudposse/atmos/pkg/github/actions"
)

// formatGitHub is the default output format: append key/path/restore-keys to
// $GITHUB_OUTPUT for a following actions/cache step.
const formatGitHub = "github"

var cachePathsParser *flags.StandardParser

// cachePathsCmd prints the resolved cache key, paths, and restore-keys so they
// can be fed to a cache mechanism (e.g. GitHub's actions/cache). It needs no CI
// provider/runtime token — only the resolved ci.cache configuration.
var cachePathsCmd = &cobra.Command{
	Use:   "paths",
	Short: "Print the cache key and paths (for use with actions/cache)",
	Long: `Print the resolved cache key, paths, and restore-keys.

This lets a native cache (such as GitHub's actions/cache) do the storage while
Atmos supplies what to cache from your ci.cache configuration. It requires no CI
provider or runtime token, so it works on any OS/CI.

With --format=github the values are written to $GITHUB_OUTPUT (key, path,
restore-keys) so a following actions/cache step can reference them as step
outputs.`,
	Args: cobra.NoArgs,
	RunE: runCachePaths,
}

// cachePathsData is the structured form used for json/yaml output.
type cachePathsData struct {
	Key          string   `json:"key" yaml:"key"`
	Paths        []string `json:"paths" yaml:"paths"`
	ExcludePaths []string `json:"exclude_paths,omitempty" yaml:"exclude_paths,omitempty"`
	RestoreKeys  []string `json:"restore_keys" yaml:"restore_keys"`
}

func runCachePaths(cmd *cobra.Command, _ []string) error {
	v := viper.GetViper()
	if err := cachePathsParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	formatStr := v.GetString("format")
	if formatStr == "" {
		formatStr = formatGitHub
	}

	resolved, err := resolveCacheConfig(cmd, cacheOverrides{
		key:   v.GetString(fieldKey),
		root:  v.GetString("root"),
		paths: v.GetStringSlice("path"),
	})
	if err != nil {
		return err
	}

	paths := cacheResolvedPaths(resolved)
	excludes := cacheResolvedExcludes(resolved)
	return emitCachePaths(formatStr, resolved, paths, excludes)
}

// cacheResolvedPaths returns the absolute paths a cache should store: the whole
// cache root when no includes are configured, otherwise each include resolved
// against the root.
func cacheResolvedPaths(cfg *cachepkg.Config) []string {
	if len(cfg.Includes) == 0 {
		return []string{cfg.Root}
	}
	out := make([]string, 0, len(cfg.Includes))
	for _, inc := range cfg.Includes {
		out = append(out, filepath.Join(cfg.Root, inc))
	}
	return out
}

// cacheResolvedExcludes returns the absolute directories that must never be
// cached, unless the user explicitly opted out via
// ci.cache.allow_unsafe_auth_cache.
func cacheResolvedExcludes(cfg *cachepkg.Config) []string {
	if cfg.AllowUnsafeAuthCache {
		return nil
	}
	defaults := cachepkg.DefaultExcludedPaths()
	out := make([]string, 0, len(defaults))
	for _, ex := range defaults {
		out = append(out, filepath.Join(cfg.Root, ex))
	}
	return out
}

// githubGlobLines renders paths/excludes as actions/cache-compatible glob
// lines for the `path:` input, pairing include/exclude patterns at the same
// wildcard depth. @actions/glob (which actions/cache uses to resolve `path:`)
// only honors a `!`-prefixed exclusion of a subdirectory when the paired
// include is also a wildcard pattern at the same depth — a bare directory
// include makes actions/cache use implicitDescendants:false, under which the
// exclusion silently matches nothing (see the still-open
// https://github.com/actions/toolkit/issues/713). Suffixing both include and
// exclude with "/**" is the documented workaround.
func githubGlobLines(paths, excludes []string) []string {
	lines := make([]string, 0, len(paths)+len(excludes))
	for _, p := range paths {
		lines = append(lines, filepath.ToSlash(filepath.Join(p, "**")))
	}
	for _, ex := range excludes {
		lines = append(lines, "!"+filepath.ToSlash(filepath.Join(ex, "**")))
	}
	return lines
}

// emitCachePaths renders the key/paths/restore-keys in the requested format.
func emitCachePaths(formatStr string, cfg *cachepkg.Config, paths []string, excludes []string) error {
	switch formatStr {
	case formatGitHub:
		// key, path (multiline), restore-keys (multiline) → $GITHUB_OUTPUT
		// (heredoc handled by pkg/env); falls back to stdout when not in CI.
		return env.Output(map[string]string{
			"key":          cfg.Key,
			"path":         strings.Join(githubGlobLines(paths, excludes), "\n"),
			"restore-keys": strings.Join(cfg.RestoreKeys, "\n"),
		}, formatGitHub, ghactions.GetOutputPath())
	case "env":
		return env.Output(map[string]string{
			"ATMOS_CI_CACHE_KEY":           cfg.Key,
			"ATMOS_CI_CACHE_PATHS":         strings.Join(paths, string(os.PathListSeparator)),
			"ATMOS_CI_CACHE_EXCLUDE_PATHS": strings.Join(excludes, string(os.PathListSeparator)),
			"ATMOS_CI_CACHE_RESTORE_KEYS":  strings.Join(cfg.RestoreKeys, "\n"),
		}, "env", "")
	case "json":
		return data.WriteJSON(cachePathsData{Key: cfg.Key, Paths: paths, ExcludePaths: excludes, RestoreKeys: cfg.RestoreKeys})
	case "yaml":
		return data.WriteYAML(cachePathsData{Key: cfg.Key, Paths: paths, ExcludePaths: excludes, RestoreKeys: cfg.RestoreKeys})
	default:
		return errUtils.Build(errUtils.ErrInvalidFormat).
			WithExplanationf("unsupported format: %s", formatStr).
			WithHint("Use one of: github, json, yaml, env").
			Err()
	}
}

func init() {
	cachePathsParser = flags.NewStandardParser(
		flags.WithStringFlag(fieldKey, "k", "", "Exact cache key (defaults to a key derived from the toolchain lockfile)"),
		flags.WithStringSliceFlag("path", "p", nil, "Root-relative subpaths (defaults to the entire cache root)"),
		flags.WithStringFlag("root", "", "", "Override the cache root directory"),
		flags.WithStringFlag("format", "", formatGitHub, "Output format: github, json, yaml, env"),
		flags.WithEnvVars(fieldKey, "ATMOS_CI_CACHE_KEY"),
		flags.WithEnvVars("path", "ATMOS_CI_CACHE_PATHS"),
		flags.WithEnvVars("root", "ATMOS_CI_CACHE_ROOT"),
		flags.WithEnvVars("format", "ATMOS_CI_CACHE_FORMAT"),
	)
	cachePathsParser.RegisterFlags(cachePathsCmd)
	if err := cachePathsParser.BindToViper(viper.GetViper()); err != nil {
		panic(fmt.Sprintf("failed to bind cache paths flags: %v", err))
	}
}
