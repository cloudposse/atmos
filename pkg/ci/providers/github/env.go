package github

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	ghactions "github.com/cloudposse/atmos/pkg/github/actions"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Compile-time assertion that Provider exports environment changes to later steps.
var _ provider.EnvExporter = (*Provider)(nil)

// WriteEnv appends key=value to the file named by GITHUB_ENV so later workflow
// steps see the variable. Multiline values use a heredoc. It never
// falls back to stdout: without GITHUB_ENV the export cannot take effect, so it
// returns an error instead of silently dropping it.
func (p *Provider) WriteEnv(key, value string) error {
	defer perf.Track(nil, "github.Provider.WriteEnv")()

	path := ghactions.GetEnvPath()
	if path == "" {
		return fmt.Errorf("%w: GITHUB_ENV is not set", errUtils.ErrCIEnvWriteFailed)
	}
	return provider.AppendFile(path, provider.FormatOutputLine(key, value), errUtils.ErrCIEnvWriteFailed)
}

// AddPath appends dir to the file named by GITHUB_PATH so later workflow steps
// find executables in it. It returns an error when GITHUB_PATH is not set.
func (p *Provider) AddPath(dir string) error {
	defer perf.Track(nil, "github.Provider.AddPath")()

	path := ghactions.GetPathPath()
	if path == "" {
		return fmt.Errorf("%w: GITHUB_PATH is not set", errUtils.ErrCIEnvWriteFailed)
	}
	return provider.AppendFile(path, dir+"\n", errUtils.ErrCIEnvWriteFailed)
}
