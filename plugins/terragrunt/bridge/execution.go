package atmosbridge

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
)

// scopedExec resolves child commands against the invocation's PATH, never the
// plugin process environment. It does not mutate process-global environment.
type scopedExec struct {
	vexec.Exec
	environment map[string]string
}

//nolint:lintroller // The bridge is a separate module and cannot import the host performance tracker.
func (e scopedExec) LookPath(file string) (string, error) {
	return e.lookPath(file, runtime.GOOS)
}

func (e scopedExec) lookPath(file, platform string) (string, error) {
	if filepath.IsAbs(file) || strings.ContainsAny(file, `/\`) {
		return e.Exec.LookPath(file)
	}
	suffixes := []string{""}
	if platform == "windows" && filepath.Ext(file) == "" {
		extensions := e.environment["PATHEXT"]
		if extensions == "" {
			extensions = ".COM;.EXE;.BAT;.CMD"
		}
		suffixes = strings.Split(extensions, ";")
	}
	for _, directory := range filepath.SplitList(e.environment["PATH"]) {
		// Match Go's protection against implicit execution from the working directory.
		if !filepath.IsAbs(directory) {
			continue
		}
		for _, suffix := range suffixes {
			candidate := filepath.Join(directory, file+suffix)
			if resolved, err := e.exactCandidate(candidate, platform); err == nil {
				return resolved, nil
			}
		}
	}
	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}

//nolint:lintroller // The bridge is a separate module and cannot import the host performance tracker.
func (e scopedExec) Command(ctx context.Context, name string, args ...string) vexec.Cmd {
	// Explicit relative commands belong to the unit's directory, which upstream
	// assigns with SetDir after constructing the command. Let os/exec resolve
	// them at start time; looking them up here would use the plugin's directory.
	if !filepath.IsAbs(name) && strings.ContainsAny(name, `/\`) {
		return e.Exec.Command(ctx, name, args...)
	}
	path, err := e.LookPath(name)
	if err != nil {
		return vexec.NewMemExec(func(context.Context, vexec.Invocation) vexec.Result { return vexec.Result{Err: err} }).Command(ctx, name, args...)
	}
	return e.Exec.Command(ctx, path, args...)
}

// exactCandidate avoids os/exec applying ambient PATHEXT to an already selected
// Windows suffix. Invocation suffix order must determine the executable.
func (e scopedExec) exactCandidate(candidate, platform string) (string, error) {
	if platform == "windows" {
		info, err := os.Stat(candidate)
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() {
			return "", exec.ErrNotFound
		}
		return candidate, nil
	}
	return e.Exec.LookPath(candidate)
}
