package source

import (
	"net/url"
	"os"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendor"
)

// Directory is a fetched source whose contents have not been interpreted.
// Its path remains valid until the FetchDirectory cleanup function is called.
type Directory struct {
	Path        string
	ResolvedRef string
}

// FetchDirectory acquires a local or remote directory without requiring a scaffold manifest.
// The returned cleanup function is always safe to call, including after errors.
func FetchDirectory(atmosConfig *schema.AtmosConfiguration, name, src string, timeout time.Duration) (*Directory, func(), error) {
	defer perf.Track(nil, "source.FetchDirectory")()

	if timeout <= 0 {
		timeout = DefaultFetchTimeout
	}
	if vendor.IsFileURI(src) || vendor.IsLocalPath(src) {
		directory, err := localDirectory(src)
		return directory, func() {}, err
	}
	if vendor.IsOCIURI(src) {
		return fetchOCI(atmosConfig, src, timeout)
	}
	return fetchRemoteDirectory(atmosConfig, name, src, timeout)
}

func localDirectory(src string) (*Directory, error) {
	localPath := src
	if vendor.IsFileURI(src) {
		parsed, err := url.Parse(src)
		if err != nil {
			return nil, err
		}
		if parsed.Host != "" && parsed.Host != "localhost" {
			return nil, errUtils.Build(errUtils.ErrInvalidFormat).
				WithExplanation("Local file sources must not specify a remote host").Err()
		}
		localPath = parsed.Path
	}
	info, err := os.Stat(localPath)
	if err != nil || !info.IsDir() {
		return nil, errUtils.Build(errUtils.ErrScaffoldCreateFromPath).
			WithCause(err).WithExplanationf("Source directory does not exist: `%s`", localPath).Err()
	}
	return &Directory{Path: localPath}, nil
}

// LoadScaffold interprets a fetched directory using the strict scaffold contract.
func (d *Directory) LoadScaffold(name, src string) (*templates.Configuration, error) {
	defer perf.Track(nil, "source.Directory.LoadScaffold")()

	conf, err := templates.LoadConfigurationFromDir(name, d.Path)
	if err != nil {
		return nil, err
	}
	if err := requireScaffoldConfig(conf, src); err != nil {
		return nil, err
	}
	// LocalDir keeps relative !include paths usable after Source is replaced
	// with provenance, including when the source was downloaded temporarily.
	conf.LocalDir = d.Path
	conf.Source = src
	if vendor.IsFileURI(src) {
		// Persist the decoded local path, matching local-source provenance and
		// keeping later scaffold updates valid for paths containing spaces.
		conf.Source = d.Path
	}
	conf.ResolvedRef = d.ResolvedRef
	return conf, nil
}
