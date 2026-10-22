// Package build compiles native Go plugins using the shared process runner.
package build

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	"github.com/cloudposse/atmos/pkg/process"
)

type settings struct {
	runner  process.Runner
	streams process.Streams
}

// Option configures build execution without changing the package manifest.
type Option func(*settings)

const outputDirectoryMode = 0o755

// WithRunner selects the subprocess implementation.
func WithRunner(runner process.Runner) Option {
	defer perf.Track(nil, "pluginbuild.WithRunner")()

	return func(options *settings) { options.runner = runner }
}

// WithStreams connects compiler output to the caller's streams.
func WithStreams(streams process.Streams) Option {
	defer perf.Track(nil, "pluginbuild.WithStreams")()

	return func(options *settings) { options.streams = streams }
}

// Run compiles the package into its current-platform executable. A failed build leaves
// the previous executable intact, and a successful build never installs it.
func Run(ctx context.Context, directory string, options ...Option) error {
	defer perf.Track(nil, "pluginbuild.Run")()

	_, err := Compile(ctx, directory, options...)
	return err
}

// Compile builds a plugin and returns its executable path only after atomically
// replacing the previous successful build.
func Compile(ctx context.Context, directory string, options ...Option) (string, error) {
	defer perf.Track(nil, "pluginbuild.Compile")()

	if err := ctx.Err(); err != nil {
		return "", err
	}
	config := settings{runner: process.NewDefaultRunner()}
	for _, option := range options {
		option(&config)
	}
	if config.runner == nil {
		return "", errUtils.ErrPluginInvalidConfig
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	executable, err := validateSource(directory)
	if err != nil {
		return "", err
	}
	if err := compile(ctx, directory, executable, &config); err != nil {
		return "", err
	}
	return filepath.Join(directory, executable), nil
}

func validateSource(directory string) (string, error) {
	definition, err := manifest.Load(directory)
	if err != nil {
		return "", err
	}
	if _, err := manifest.ReadFile(directory, "go.mod"); err != nil {
		return "", errUtils.Build(errUtils.ErrPluginBuild).WithCause(err).
			WithHint("Run this command in a Go plugin source directory containing go.mod and plugin.yaml.").Err()
	}
	registry, err := bootstrap.Descriptors()
	if err != nil {
		return "", err
	}
	if _, err := registry.Validate(directory, definition.Spec.Capabilities); err != nil {
		return "", err
	}
	return manifest.Executable(definition, install.CurrentPlatform()), nil
}

func compile(ctx context.Context, directory, executable string, config *settings) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.MkdirAll(filepath.Dir(executable), outputDirectoryMode); err != nil {
		return err
	}
	output, err := os.CreateTemp(directory, ".atmos-plugin-build-*")
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(filepath.Base(output.Name())) }()
	if err := output.Close(); err != nil {
		return err
	}
	if err := prepareModule(ctx, directory, config); err != nil {
		return err
	}
	result := config.runner.Run(ctx, process.TaskSpec{
		Command: "go", Args: []string{"build", "-o", output.Name(), "."},
		Dir: directory, Streams: config.streams,
	})
	if err := buildResult(&result); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return root.Rename(filepath.Base(output.Name()), executable)
}

func buildResult(result *process.Result) error {
	if result.Success() {
		return nil
	}
	failure := errUtils.Build(errUtils.ErrPluginBuild).WithCause(result.Err)
	if errors.Is(result.Err, exec.ErrNotFound) {
		failure.WithHint("Install Go and make the go executable available on PATH. Plugin consumers do not need Go.")
	}
	if result.ExitCode > 0 {
		failure.WithExitCode(result.ExitCode)
	}
	return failure.Err()
}

// prepareModule resolves a standalone scaffold's dependency graph and checksums.
// A development workspace supplies local modules (including an unreleased SDK)
// without module requirements, so tidy must not resolve those from a registry.
func prepareModule(ctx context.Context, directory string, config *settings) error {
	if _, err := os.Stat(filepath.Join(directory, "go.sum")); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var workspace bytes.Buffer
	result := config.runner.Run(ctx, process.TaskSpec{
		Command: "go", Args: []string{"env", "GOWORK"}, Dir: directory,
		Streams: process.Streams{Stdout: &workspace, Stderr: config.streams.Stderr},
	})
	if err := buildResult(&result); err != nil {
		return err
	}
	if path := strings.TrimSpace(workspace.String()); path != "" && path != "off" {
		return nil
	}
	result = config.runner.Run(ctx, process.TaskSpec{Command: "go", Args: []string{"mod", "tidy"}, Dir: directory, Streams: config.streams})
	return buildResult(&result)
}
