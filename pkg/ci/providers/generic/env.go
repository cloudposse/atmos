package generic

import (
	"os"

	"al.essio.dev/pkg/shellescape"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	ghactions "github.com/cloudposse/atmos/pkg/github/actions"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Ensure Provider implements provider.EnvExporter.
var _ provider.EnvExporter = (*Provider)(nil)

// WriteEnv exports key=value for later steps. With ATMOS_CI_ENV set it appends to that file
// in GitHub Actions format; otherwise it renders a shell `export` line.
//
// ATMOS_CI_ENV and ATMOS_CI_PATH are read at call time. ATMOS_CI_OUTPUT and ATMOS_CI_SUMMARY
// are still read once in NewProvider; moving them to call time is a follow-up.
func (p *Provider) WriteEnv(key, value string) error {
	defer perf.Track(nil, "generic.Provider.WriteEnv")()

	if path := os.Getenv("ATMOS_CI_ENV"); path != "" {
		return provider.AppendFile(path, ghactions.FormatValue(key, value), errUtils.ErrCIEnvWriteFailed)
	}

	p.out().Writef("export %s=%s\n", key, shellescape.Quote(value))
	return nil
}

// AddPath prepends dir to PATH for later steps. With ATMOS_CI_PATH set it appends dir to that
// file; otherwise it renders a shell `export PATH=` line.
func (p *Provider) AddPath(dir string) error {
	defer perf.Track(nil, "generic.Provider.AddPath")()

	if path := os.Getenv("ATMOS_CI_PATH"); path != "" {
		return provider.AppendFile(path, dir+"\n", errUtils.ErrCIEnvWriteFailed)
	}

	p.out().Writef("export PATH=%s:\"$PATH\"\n", shellescape.Quote(dir))
	return nil
}
