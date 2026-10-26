package adapters

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/downloader"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	remoteimports "github.com/cloudposse/atmos/pkg/stack/imports"
)

// GoGetterAdapter handles remote imports using HashiCorp's go-getter.
// It supports a wide variety of URL schemes for downloading remote configurations.
type GoGetterAdapter struct{}

// Schemes returns all URL schemes/prefixes handled by go-getter.
func (g *GoGetterAdapter) Schemes() []string {
	return []string{
		// HTTP/HTTPS.
		"http://",
		"https://",
		// Git.
		"git::",
		"git@",
		// Cloud storage.
		"s3::",
		"s3://",
		"gcs::",
		"gcs://",
		// OCI registries.
		"oci://",
		// Local file URLs.
		"file://",
		// Mercurial.
		"hg::",
		// SSH-based protocols.
		"scp://",
		"sftp://",
		// Shorthand for popular hosts.
		"github.com/",
		"bitbucket.org/",
	}
}

// Resolve downloads a remote configuration and processes any nested imports.
//
//nolint:revive // argument-limit: matches ImportAdapter interface signature.
func (g *GoGetterAdapter) Resolve(
	ctx context.Context,
	importPath string,
	basePath string,
	tempDir string,
	currentDepth int,
	maxDepth int,
	atmosConfig *schema.AtmosConfiguration,
) ([]config.ResolvedPaths, error) {
	defer perf.Track(nil, "adapters.GoGetterAdapter.Resolve")()

	// Warn when a git-backed remote import is pinned to a mutable ref (a branch, or no ref at
	// all): such an import silently absorbs the upstream repo's edits on every cache refresh,
	// with no integrity check - the config-import analogue of using an unpinned GitHub Action.
	warnIfUnpinnedRef(importPath)

	// Resolve the remote configuration.
	tempFiles, err := resolveRemoteConfigFiles(importPath, atmosConfig)
	if err != nil {
		log.Debug("failed to download remote config", "path", importPath, "error", err)
		return nil, err
	}

	resolvedPaths := make([]config.ResolvedPaths, 0)
	for _, tempFile := range tempFiles {
		// Read the downloaded configuration.
		v := viper.New()
		v.SetConfigFile(tempFile)
		err = v.ReadInConfig()
		if err != nil {
			log.Debug("failed to read remote config", "path", importPath, "error", err)
			return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrReadConfig, err)
		}

		resolvedPaths = append(resolvedPaths, config.ResolvedPaths{
			FilePath:    tempFile,
			ImportPaths: importPath,
			ImportType:  config.REMOTE,
		})

		// Process nested imports from the remote file. Read the raw value (not GetStringSlice)
		// so object-form entries (skip_if_missing, when) survive; NormalizeConfigImports accepts
		// both the legacy string list and the object form.
		imports, err := config.NormalizeConfigImports(v.Get("import"), atmosConfig)
		if err != nil {
			return nil, err
		}

		if len(imports) > 0 {
			importBasePath, err := config.ResolveConfigImportBasePath(v.GetString("base_path"), tempFile, basePath)
			if err != nil {
				return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrProcessNestedImports, err)
			}
			nestedPaths, err := config.ProcessImportsFromAdapter(atmosConfig, importBasePath, imports, tempDir, currentDepth+1, maxDepth)
			if err != nil {
				log.Debug("failed to process nested imports", "import", importPath, "err", err)
				return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrProcessNestedImports, err)
			}
			resolvedPaths = append(resolvedPaths, nestedPaths...)
		}
	}

	return resolvedPaths, nil
}

// gitBackedImportPrefixes are the go-getter scheme prefixes that resolve to a Git repository and
// therefore accept a `?ref=` pin.
var gitBackedImportPrefixes = []string{"git::", "git@", "github.com/", "bitbucket.org/"}

// commitSHAPattern matches a full or abbreviated Git commit SHA (7-40 hex chars).
var commitSHAPattern = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

// versionTagPattern matches a semver-style version tag such as `v1.2.3`, `1.2`, or `v2`.
var versionTagPattern = regexp.MustCompile(`^v?\d+(\.\d+)*$`)

// warnIfUnpinnedRef emits a warning when a git-backed remote import is not pinned to a commit SHA
// or a version tag. An unpinned import (a branch ref, or no `?ref=` at all) silently propagates
// another repo's edits into this repo's config on every cache refresh, with no integrity check.
// It is advisory only - the import still resolves.
func warnIfUnpinnedRef(importPath string) {
	if !isGitBackedImport(importPath) {
		return
	}
	ref := extractRef(importPath)
	if isPinnedRef(ref) {
		return
	}
	display := ref
	if display == "" {
		display = "<none>"
	}
	log.Warn("remote config import is not pinned; it will absorb upstream edits on every cache refresh - pin it with `?ref=<commit-sha>` for an immutable, tamper-evident import, or a version tag as a conventional (movable) version reference",
		"import", config.SanitizeImport(importPath), "ref", display)
}

// isGitBackedImport reports whether importPath uses a go-getter scheme backed by Git.
func isGitBackedImport(importPath string) bool {
	for _, prefix := range gitBackedImportPrefixes {
		if strings.HasPrefix(importPath, prefix) {
			return true
		}
	}
	return false
}

// extractRef returns the `ref` query parameter from a go-getter URL, or "" when absent.
func extractRef(importPath string) string {
	i := strings.Index(importPath, "?")
	if i < 0 {
		return ""
	}
	query, err := url.ParseQuery(importPath[i+1:])
	if err != nil {
		return ""
	}
	return strings.TrimSpace(query.Get("ref"))
}

// isPinnedRef reports whether ref is an immutable pin: a commit SHA or a version tag. An empty
// ref, a branch name, or a non-version tag is treated as unpinned.
func isPinnedRef(ref string) bool {
	if ref == "" {
		return false
	}
	return commitSHAPattern.MatchString(ref) || versionTagPattern.MatchString(ref)
}

func resolveRemoteConfigFiles(url string, atmosConfig *schema.AtmosConfiguration) ([]string, error) {
	matches, err := remoteimports.ResolveRemoteImport(atmosConfig, url)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		paths = append(paths, match.Path)
	}
	return paths, nil
}

// remoteConfigTimeout bounds a single remote config-import download.
const remoteConfigTimeout = 5 * time.Second

// downloadRemoteConfig downloads a configuration file from a remote URL.
//
// It fetches through the canonical go-getter downloader (downloader.NewGoGetterDownloader)
// rather than a bare getter.Client so that remote config imports get the same GitHub
// authentication as every other remote read: the CustomGitDetector injects a token into the
// URL (ATMOS_PRO_GITHUB_TOKEN > ATMOS_GITHUB_TOKEN > GITHUB_TOKEN) and the Atmos Pro
// credential broker exports GIT_CONFIG_* for the spawned git subprocess. Without this, a
// private-repo `import:` in atmos.yaml would be fetched unauthenticated. The downloader
// enforces the timeout and manages its own context internally.
func downloadRemoteConfig(url string, tempDir string, atmosConfig *schema.AtmosConfiguration) (string, error) {
	defer perf.Track(nil, "adapters.downloadRemoteConfig")()

	// Generate unique filename for the temp file.
	fileName := fmt.Sprintf("atmos-import-%d.yaml", time.Now().UnixNano())
	tempFile := filepath.Join(tempDir, fileName)

	if err := downloader.NewGoGetterDownloader(atmosConfig).Fetch(url, tempFile, downloader.ClientModeFile, remoteConfigTimeout); err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrDownloadRemoteConfig, err)
	}

	return tempFile, nil
}
