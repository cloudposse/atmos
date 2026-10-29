package atmosbridge

import (
	"context"
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

func (e scopedExec) LookPath(file string) (string, error) {
	if filepath.IsAbs(file) || strings.ContainsAny(file, `/\`) {
		return e.Exec.LookPath(file)
	}
	suffixes := []string{""}
	if runtime.GOOS == "windows" && filepath.Ext(file) == "" {
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
			if resolved, err := e.Exec.LookPath(candidate); err == nil {
				return resolved, nil
			}
		}
	}
	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}

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
