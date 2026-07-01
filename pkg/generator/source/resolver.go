// Package source resolves scaffold templates from local paths or remote
// sources (git, https, s3) into a templates.Configuration ready for generation.
// It is the seam that lets `atmos init`/`atmos scaffold` distribute templates
// remotely while reusing the existing generator engine.
package source

import (
	"os"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendor"
)

// DefaultFetchTimeout bounds how long a remote scaffold fetch may take.
const DefaultFetchTimeout = 5 * time.Minute

// Resolve fetches a scaffold template from src (a local path, file://, or a
// go-getter remote such as git/https/s3) into a usable templates.Configuration.
// The returned cleanup function removes any temporary download directory; it is
// never nil and is always safe to call.
func Resolve(atmosConfig *schema.AtmosConfiguration, name, src string, timeout time.Duration) (*templates.Configuration, func(), error) {
	defer perf.Track(nil, "source.Resolve")()

	noop := func() {}
	if timeout <= 0 {
		timeout = DefaultFetchTimeout
	}

	// OCI distribution is not wired up yet; fail clearly rather than silently.
	if vendor.IsOCIURI(src) {
		return nil, noop, errUtils.Build(errUtils.ErrScaffoldSourceUnsupported).
			WithExplanationf("OCI scaffold sources are not supported yet: `%s`", src).
			WithHint("Use a git, https, or local source for now").
			WithContext("source", src).
			WithExitCode(2).
			Err()
	}

	// Local sources (relative/absolute path or file://) load directly, no fetch.
	if vendor.IsFileURI(src) || vendor.IsLocalPath(src) {
		path := strings.TrimPrefix(src, "file://")
		conf, err := templates.LoadConfigurationFromDir(name, path)
		return conf, noop, err
	}

	// Remote sources: download into a temp dir via go-getter, then load.
	tempDir, err := os.MkdirTemp("", "atmos-scaffold-")
	if err != nil {
		return nil, noop, errUtils.Build(errUtils.ErrCreateTempDirectory).
			WithCause(err).
			WithExplanation("Failed to create a temporary directory for the scaffold download").
			WithExitCode(1).
			Err()
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

	if fetchErr := downloader.NewGoGetterDownloader(atmosConfig).Fetch(src, tempDir, downloader.ClientModeDir, timeout); fetchErr != nil {
		cleanup()
		return nil, noop, errUtils.Build(errUtils.ErrScaffoldFetchSource).
			WithCause(fetchErr).
			WithExplanationf("Failed to fetch scaffold template from `%s`", src).
			WithHint("Check the source URL and your network connection").
			WithHint("For private repositories set `ATMOS_GITHUB_TOKEN` (or the host-specific token)").
			WithContext("source", src).
			WithExitCode(1).
			Err()
	}

	conf, err := templates.LoadConfigurationFromDir(name, tempDir)
	if err != nil {
		cleanup()
		return nil, noop, err
	}
	return conf, cleanup, nil
}

// Hydrate materializes a catalog/remote stub (a Configuration with no Files but
// a Source) into a full template by fetching its Source. Full templates
// (embedded or already-loaded local) are returned unchanged with a no-op
// cleanup. The returned cleanup must be called after generation completes.
func Hydrate(stub *templates.Configuration, override string) (func(), error) {
	defer perf.Track(nil, "source.Hydrate")()

	noop := func() {}
	if len(stub.Files) > 0 || stub.Source == "" {
		return noop, nil
	}

	// Only remote sources need the downloader (and thus a loaded config for
	// token injection). Local/override sources resolve without touching config.
	var atmosConfig schema.AtmosConfiguration
	if !vendor.IsLocalPath(stub.Source) && !vendor.IsFileURI(stub.Source) {
		loaded, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return noop, err
		}
		atmosConfig = loaded
	}

	resolved, cleanup, err := Resolve(&atmosConfig, stub.Name, stub.Source, DefaultFetchTimeout)
	if err != nil {
		return noop, err
	}
	*stub = *resolved
	return cleanup, nil
}
