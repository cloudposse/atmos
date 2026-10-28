package conformance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginbuild "github.com/cloudposse/atmos/pkg/plugin/build"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginplatform "github.com/cloudposse/atmos/pkg/plugin/platform"
	"github.com/cloudposse/atmos/pkg/process"
)

const (
	coverageDirectoryMode = 0o755
	coverageFileMode      = 0o600
)

// GoSourceRunner builds and tests a standalone Go module through the shared process runner.
// Runner is optional and defaults to the normal subprocess implementation.
type GoSourceRunner struct{ Runner process.Runner }

// Run performs explicitly requested source tasks. Coverage includes the Go module's
// unit tests, independently of plugin.tests.yaml authored protocol/integration steps.
func (source GoSourceRunner) Run(ctx context.Context, request *SourceRequest) error {
	defer perf.Track(nil, "conformance.GoSourceRunner.Run")()

	if request == nil {
		return errUtils.ErrPluginInvalidConfig
	}
	runner := source.Runner
	if runner == nil {
		runner = process.NewDefaultRunner()
	}
	if request.Build {
		if _, err := pluginbuild.Compile(ctx, request.Directory, pluginbuild.WithPlatform(request.Platform), pluginbuild.WithStreams(request.Streams), pluginbuild.WithRunner(runner)); err != nil {
			return err
		}
	}
	if request.CoverProfile == "" {
		return nil
	}
	if !manifest.LocalPath(request.CoverProfile) {
		return errUtils.ErrPluginInvalidConfig
	}
	if _, err := manifest.ReadFile(request.Directory, "go.mod"); err != nil {
		return fmt.Errorf("%w: Go coverage requires go.mod in the plugin directory: %w", errUtils.ErrPluginInvalidConfig, err)
	}
	return collectGoCoverage(ctx, runner, request)
}

func collectGoCoverage(ctx context.Context, runner process.Runner, request *SourceRequest) error {
	platform := request.Platform
	if platform == "" {
		platform = pluginplatform.Current()
	}
	target, err := pluginplatform.Parse(platform)
	if err != nil {
		return err
	}
	// The compiler environment follows the actual tested target, never inherited GOOS/GOARCH.
	environment := target.Environment(os.Environ())
	temporary, err := os.CreateTemp(request.Directory, ".atmos-plugin-coverage-*.out")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if err := temporary.Close(); err != nil {
		return err
	}
	result := runner.Run(ctx, process.TaskSpec{
		Command: "go", Args: []string{"test", "-count=1", "-covermode=atomic", "-coverprofile=" + temporary.Name(), "./..."},
		Dir: request.Directory, Env: environment, Streams: request.Streams,
	})
	if !result.Success() {
		return errors.Join(errUtils.ErrTestsFailed, result.Err)
	}
	// Parse and report the profile before making it available as a successful result.
	result = runner.Run(ctx, process.TaskSpec{
		Command: "go", Args: []string{"tool", "cover", "-func=" + temporary.Name()},
		Dir: request.Directory, Env: environment, Streams: request.Streams,
	})
	if !result.Success() {
		return errors.Join(errUtils.ErrTestsFailed, result.Err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return saveCoverage(request.Directory, request.CoverProfile, temporary.Name())
}

func saveCoverage(directory, destination, source string) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.MkdirAll(filepath.Dir(destination), coverageDirectoryMode); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := root.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, coverageFileMode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	return errors.Join(copyErr, output.Close())
}
