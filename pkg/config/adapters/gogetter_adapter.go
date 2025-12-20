package adapters

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-getter"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
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
func (g *GoGetterAdapter) Resolve(
	ctx context.Context,
	importPath string,
	basePath string,
	tempDir string,
	currentDepth int,
	maxDepth int,
) ([]config.ResolvedPaths, error) {
	defer perf.Track(nil, "adapters.GoGetterAdapter.Resolve")()

	// Download the remote configuration.
	tempFile, err := downloadRemoteConfig(ctx, importPath, tempDir)
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
		return nil, err
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
		nestedPaths, err := config.ProcessImportsFromAdapter(importBasePath, imports, tempDir, currentDepth+1, maxDepth)
		if err != nil {
			log.Debug("failed to process nested imports", "import", importPath, "err", err)
			return nil, err
		}
		resolvedPaths = append(resolvedPaths, nestedPaths...)
	}

	return resolvedPaths, nil
}

// downloadRemoteConfig downloads a configuration file from a remote URL.
func downloadRemoteConfig(ctx context.Context, url string, tempDir string) (string, error) {
	defer perf.Track(nil, "adapters.downloadRemoteConfig")()

	// Generate unique filename for the temp file.
	fileName := fmt.Sprintf("atmos-import-%d.yaml", time.Now().UnixNano())
	tempFile := filepath.Join(tempDir, fileName)

	// Use context with timeout if none provided.
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}

	client := &getter.Client{
		Ctx:  ctx,
		Src:  url,
		Dst:  tempFile,
		Mode: getter.ClientModeFile,
	}

	err := client.Get()
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrDownloadRemoteConfig, err)
	}

	return tempFile, nil
}
