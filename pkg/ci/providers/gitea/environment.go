package gitea

import (
	"fmt"
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// WriteEnv exports an environment variable to later job steps.
func (p *Provider) WriteEnv(key, value string) error {
	defer perf.Track(nil, "gitea.WriteEnv")()

	path := env("ENV")
	if path == "" || !envName.MatchString(key) {
		return fmt.Errorf("%w: missing Gitea environment file or invalid variable name", errUtils.ErrCIEnvWriteFailed)
	}
	return provider.AppendFile(path, provider.FormatOutputLine(key, value), errUtils.ErrCIEnvWriteFailed)
}

// AddPath adds a directory to PATH in later job steps.
func (p *Provider) AddPath(dir string) error {
	defer perf.Track(nil, "gitea.AddPath")()

	path := env("PATH")
	if path == "" || strings.ContainsAny(dir, "\r\n") {
		return fmt.Errorf("%w: missing Gitea path file or invalid directory", errUtils.ErrCIEnvWriteFailed)
	}
	return provider.AppendFile(path, dir+"\n", errUtils.ErrCIEnvWriteFailed)
}

// StartLogGroup opens a runner log group.
func (p *Provider) StartLogGroup(title string) error {
	defer perf.Track(nil, "gitea.StartLogGroup")()

	escaped := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(title)
	return data.Writef("::group::%s\n", escaped)
}

// EndLogGroup closes a runner log group.
func (p *Provider) EndLogGroup() error {
	defer perf.Track(nil, "gitea.EndLogGroup")()
	return data.Writeln("::endgroup::")
}
