package process

import (
	"path/filepath"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"

	"github.com/cloudposse/atmos/pkg/perf"
)

// LookPath resolves name to an executable using the PATH (and, on Windows, PATHEXT) found in
// env rather than the process environment, so callers see exactly what a process started with
// that env would run. Names containing a path separator resolve relative to dir. It returns an
// error when nothing executable matches.
func LookPath(dir string, env []string, name string) (string, error) {
	defer perf.Track(nil, "process.LookPath")()

	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return interp.LookPathDir(abs, expand.ListEnviron(env...), name)
}
