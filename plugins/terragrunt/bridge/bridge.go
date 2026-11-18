// Package atmosbridge is an Atmos-maintained compatibility facade for Terragrunt
// v1.2.1. Its local module path permits access to upstream internal packages.
// This is not an upstream-published package; a relative module replacement selects it.
package atmosbridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/spf13/afero"
)

// Venv is the pinned upstream virtual environment. The alias lets evaluator
// phases pass it between helpers without importing upstream internals.
type Venv = venv.Venv

// Environment isolates invocation environments and output writers.
func Environment(env map[string]string, stdout, stderr io.Writer) *venv.Venv {
	// Invocation environments must not replace the plugin's process-wide environment.
	return venv.OSVenv().WithExec(scopedExec{Exec: vexec.NewOSExec(), environment: env}).WithEnv(env).WithReplaceEnviron(func(map[string]string) error { return nil }).WithWriter(stdout).WithErrWriter(stderr)
}

// Discovery denies process execution, network access, and decryption. Only an
// explicit stack generation phase may write its generated files.
func Discovery(v *venv.Venv, generate bool) *venv.Venv {
	v = v.WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		return vexec.Result{Err: fmt.Errorf("read-only discovery cannot execute %s", inv.Name)}
	}).WithHTTP(vhttp.NewNoNetworkClient()).WithSops(vsops.NewMemDecrypter(func(_ map[string]string, path, _ string) ([]byte, error) {
		return nil, fmt.Errorf("read-only discovery cannot decrypt %s", path)
	}))
	if !generate {
		v = v.WithFS(afero.NewReadOnlyFs(v.FS))
	}
	return v
}

// WorkingDir uses upstream cache identity without downloading or changing state.
func WorkingDir(logger log.Logger, env *venv.Venv, source, download, directory string) (string, error) {
	resolved, err := tf.NewSource(logger, env.FS, source, download, directory, false)
	if err != nil {
		return "", err
	}
	return resolved.WorkingDir, nil
}

// GenerateStack materializes the units and nested stack files declared by one
// stack file through Terragrunt's real Go implementation. Unlike directory-wide
// generation, it never lists or generates other stack files under the same
// directory; callers recurse into the nested stacks the file declares. It does
// not execute the Terragrunt CLI or apply infrastructure.
func GenerateStack(ctx context.Context, logger log.Logger, env *venv.Venv, path, tofu string) error {
	opts := options.NewTerragruntOptions(env.Exec)
	opts.WorkingDir = filepath.Dir(path)
	opts.RootWorkingDir = opts.WorkingDir
	opts.TerragruntStackConfigPath = path
	opts.TFPath = tofu
	scoped, pctx, err := configbridge.NewParsingContext(opts).WithConfigPath(logger, path)
	if err != nil {
		return err
	}
	pool := worker.NewWorkerPool(opts.Parallelism)
	defer pool.Stop()
	if err := config.GenerateStackFile(ctx, scoped, env, pctx, pool, path); err != nil {
		return errors.Join(err, pool.Wait())
	}
	return pool.Wait()
}

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

// IsDestroyOperation uses upstream boolean flag parsing, including last-value precedence.
func IsDestroyOperation(command string, arguments []string) bool {
	return command == "destroy" || iacargs.HasDestroyFlag(arguments)
}
