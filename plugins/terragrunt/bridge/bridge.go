// Package atmosbridge is an Atmos-maintained compatibility facade for Terragrunt
// v1.2.1. Its local module path permits access to upstream internal packages.
// This is not an upstream-published package; a relative module replacement selects it.
package atmosbridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
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
//
//nolint:lintroller // The bridge is a separate module and cannot import the host performance tracker.
func Environment(env map[string]string, stdout, stderr io.Writer) *venv.Venv {
	// Invocation environments must not replace the plugin's process-wide environment.
	return venv.OSVenv().WithExec(scopedExec{Exec: vexec.NewOSExec(), environment: env}).WithEnv(env).WithReplaceEnviron(func(map[string]string) error { return nil }).WithWriter(stdout).WithErrWriter(stderr)
}

// Discovery denies process execution, network access, and decryption. Only an
// explicit stack generation phase may write its generated files.
//
//nolint:lintroller // The bridge is a separate module and cannot import the host performance tracker.
func Discovery(v *venv.Venv, generate bool) *venv.Venv {
	return DiscoveryObserved(v, generate, func() {})
}

var (
	errDiscoveryExecute = errors.New("read-only discovery cannot execute")
	errDiscoveryDecrypt = errors.New("read-only discovery cannot decrypt")
	// Preserve the classification identities used by the live-log bridge.
	errDiscoveryExecution  = errDiscoveryExecute
	errDiscoveryDecryption = errDiscoveryDecrypt
)

// DiscoveryObserved reports denied effects even if an HCL try expression swallows the error.
//
//nolint:lintroller // Standalone compatibility module cannot import the host performance tracker.
func DiscoveryObserved(v *venv.Venv, generate bool, denied func()) *venv.Venv {
	v = v.WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		denied()
		return vexec.Result{Err: fmt.Errorf("%w %s", errDiscoveryExecute, inv.Name)}
	}).WithHTTP(&http.Client{Transport: deniedTransport{base: vhttp.NewNoNetworkClient().Transport, denied: denied}}).WithSops(vsops.NewMemDecrypter(func(_ map[string]string, path, _ string) ([]byte, error) {
		denied()
		return nil, fmt.Errorf("%w %s", errDiscoveryDecrypt, path)
	}))
	if !generate {
		v = v.WithFS(afero.NewReadOnlyFs(v.FS))
	}
	return v
}

// WorkingDir uses upstream cache identity without downloading or changing state.
//
//nolint:lintroller // The bridge is a separate module and cannot import the host performance tracker.
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
//
//nolint:lintroller // The bridge is a separate module and cannot import the host performance tracker.
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

// IsDestroyOperation skips values of Terraform's string flags before applying upstream
// boolean flag parsing, including last-value precedence.
//
//nolint:lintroller // The bridge is a separate module and cannot import the host performance tracker.
func IsDestroyOperation(command string, arguments []string) bool {
	if command == "destroy" {
		return true
	}
	flags := make([]string, 0, len(arguments))
	for i := 0; i < len(arguments); i++ {
		argument := arguments[i]
		if argument == "--" {
			break
		}
		name, _, inline := strings.Cut(argument, "=")
		if !inline && strings.HasPrefix(name, "-") && destroyValueOption(strings.TrimLeft(name, "-")) {
			i++
			continue
		}
		flags = append(flags, argument)
	}
	return iacargs.HasDestroyFlag(flags)
}

func destroyValueOption(name string) bool {
	switch name {
	case "backend-config", "backup", "chdir", "config", "from-module", "fs-mirror", "generate-config-out", "lock-timeout", "net-mirror", "out", "parallelism", "platform", "plugin-dir", "replace", "state", "state-out", "target", "var", "var-file":
		return true
	default:
		return false
	}
}

// deniedTransport retains the upstream no-network error shape while recording its use.
type deniedTransport struct {
	base   http.RoundTripper
	denied func()
}

//nolint:lintroller // Pure HTTP interface delegation in the standalone compatibility module.
func (t deniedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.denied()
	return t.base.RoundTrip(request)
}
