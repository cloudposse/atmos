package adapters

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/downloader"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
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

	// Download the remote configuration.
	tempFile, err := downloadRemoteConfig(importPath, tempDir, atmosConfig)
	if err != nil {
		log.Debug("failed to download remote config", "path", importPath, "error", err)
		return nil, err
	}

	// Read the downloaded configuration.
	v := viper.New()
	v.SetConfigFile(tempFile)
	err = v.ReadInConfig()
	if err != nil {
		log.Debug("failed to read remote config", "path", importPath, "error", err)
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrReadConfig, err)
	}

	resolvedPaths := make([]config.ResolvedPaths, 0)
	resolvedPaths = append(resolvedPaths, config.ResolvedPaths{
		FilePath:    tempFile,
		ImportPaths: importPath,
		ImportType:  config.REMOTE,
	})

	// Process nested imports from the remote file.
	imports := v.GetStringSlice("import")
	importBasePath := v.GetString("base_path")
	if importBasePath == "" {
		importBasePath = basePath
	}

	if len(imports) > 0 {
		nestedPaths, err := config.ProcessImportsFromAdapter(atmosConfig, importBasePath, imports, tempDir, currentDepth+1, maxDepth)
		if err != nil {
			log.Debug("failed to process nested imports", "import", importPath, "err", err)
			return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrProcessNestedImports, err)
		}
		resolvedPaths = append(resolvedPaths, nestedPaths...)
	}

	return resolvedPaths, nil
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
