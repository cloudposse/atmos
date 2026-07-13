// Package source resolves scaffold templates from local paths or remote
// sources (git, https, s3) into a templates.Configuration ready for generation.
// It is the seam that lets `atmos init`/`atmos scaffold` distribute templates
// remotely while reusing the existing generator engine.
package source

import (
	"fmt"
	"os"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
	"github.com/cloudposse/atmos/pkg/vendor"
)

// DefaultFetchTimeout bounds how long a remote scaffold fetch may take.
const DefaultFetchTimeout = 5 * time.Minute

// IsTemplateSource reports whether an init/scaffold template argument looks
// like a direct source rather than a catalog or embedded template key.
func IsTemplateSource(value string) bool {
	defer perf.Track(nil, "source.IsTemplateSource")()

	if value == "" {
		return false
	}
	return vendor.IsFileURI(value) ||
		vendor.IsOCIURI(value) ||
		vendor.IsS3URI(value) ||
		vendor.IsGitURI(value) ||
		vendor.HasLocalPathPrefix(value)
}

// WithRef applies --ref sugar to a go-getter source. Existing ref query
// parameters win; local paths and file/OCI/S3 sources are returned unchanged.
func WithRef(src, ref string) string {
	defer perf.Track(nil, "source.WithRef")()

	if src == "" || ref == "" {
		return src
	}
	if !vendor.IsGitURI(src) || strings.Contains(src, "ref=") {
		return src
	}
	sep := "?"
	if strings.Contains(src, "?") {
		sep = "&"
	}
	return src + sep + "ref=" + ref
}

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
		conf, err := resolveLocal(name, src)
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

	normalized := vendor.NormalizeURI(src)
	// Keep the spinner message short: the full go-getter URL (subdir + ref)
	// can be long enough to wrap across terminal rows, which breaks
	// bubbletea's in-place redraw and makes the spinner scroll a new line
	// per tick instead of overwriting.
	progressMsg := fmt.Sprintf("Fetching scaffold template `%s`", name)
	completedMsg := fmt.Sprintf("Fetched scaffold template `%s`", name)
	fetchErr := spinner.ExecWithSpinner(progressMsg, completedMsg, func() error {
		return downloader.NewGoGetterDownloader(atmosConfig).Fetch(normalized, tempDir, downloader.ClientModeDir, timeout)
	})
	if fetchErr != nil {
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
	if err := requireScaffoldConfig(conf, src); err != nil {
		cleanup()
		return nil, noop, err
	}
	return conf, cleanup, nil
}

func resolveLocal(name, src string) (*templates.Configuration, error) {
	path := strings.TrimPrefix(src, "file://")
	conf, err := templates.LoadConfigurationFromDir(name, path)
	if err != nil {
		return nil, err
	}
	if err := requireScaffoldConfig(conf, src); err != nil {
		return nil, err
	}
	return conf, nil
}

func requireScaffoldConfig(conf *templates.Configuration, src string) error {
	if conf != nil && templates.HasScaffoldConfig(conf.Files) {
		return nil
	}
	return errUtils.Build(errUtils.ErrScaffoldConfigMissing).
		WithExplanationf("Template source `%s` does not contain scaffold.yaml at its root", src).
		WithHint("Point the source at a scaffold template directory, or use go-getter //subdir syntax").
		WithContext("source", src).
		WithExitCode(2).
		Err()
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
