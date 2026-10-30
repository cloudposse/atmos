// Package atmosbridge is an Atmos-maintained compatibility facade for Terragrunt
// v1.2.1. Its local module path permits access to upstream internal packages.
// This is not an upstream-published package; a relative module replacement selects it.
package atmosbridge

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/spf13/afero"
)

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

// Generate expands nested stacks through Terragrunt's real Go implementation.
// It does not execute the Terragrunt CLI or apply infrastructure.
func Generate(ctx context.Context, logger log.Logger, env *venv.Venv, path, tofu string) error {
	opts := options.NewTerragruntOptions(env.Exec)
	opts.WorkingDir = filepath.Dir(path)
	opts.RootWorkingDir = opts.WorkingDir
	opts.TerragruntStackConfigPath = path
	opts.TFPath = tofu
	return generate.NewGenerator().GenerateStacks(ctx, logger, env, opts, nil)
}

// IsDestroyOperation uses upstream boolean flag parsing, including last-value precedence.
func IsDestroyOperation(command string, arguments []string) bool {
	return command == "destroy" || iacargs.HasDestroyFlag(arguments)
}
