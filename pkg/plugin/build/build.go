// Package build compiles native Go plugins using the shared process runner.
package build

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	"github.com/cloudposse/atmos/pkg/plugin/platform"
	"github.com/cloudposse/atmos/pkg/process"
)

type settings struct {
	runner       process.Runner
	streams      process.Streams
	platform     string
	allPlatforms bool
	environment  []string
	prepared     bool
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

// WithPlatform selects an explicitly declared OS/architecture[/variant] build target.
func WithPlatform(value string) Option {
	defer perf.Track(nil, "pluginbuild.WithPlatform")()

	return func(options *settings) { options.platform = value }
}

// WithAllPlatforms builds every declared platform with bounded concurrency.
func WithAllPlatforms(value bool) Option {
	defer perf.Track(nil, "pluginbuild.WithAllPlatforms")()

	return func(options *settings) { options.allPlatforms = value }
}

// Compile builds one plugin and returns its executable after atomic replacement.
// Use CompileAll when requesting multiple declared targets.
func Compile(ctx context.Context, directory string, options ...Option) (string, error) {
	defer perf.Track(nil, "pluginbuild.Compile")()

	paths, err := CompileAll(ctx, directory, options...)
	if err != nil {
		return "", err
	}
	return paths[len(paths)-1], nil
}

// CompileAll builds the selected targets without executing them. All targets and
// destinations are validated first. On failure it returns completed paths; each
// completed binary remains available and the failed target's old binary survives.
func CompileAll(ctx context.Context, directory string, options ...Option) ([]string, error) {
	defer perf.Track(nil, "pluginbuild.CompileAll")()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	config, err := buildSettings(options)
	if err != nil {
		return nil, err
	}
	directory, err = filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	definition, err := validateSource(directory)
	if err != nil {
		return nil, err
	}
	targets, err := buildTargets(definition, &config)
	if err != nil {
		return nil, err
	}
	if err := validateOutputPaths(directory, targets); err != nil {
		return nil, err
	}
	return compileTargets(ctx, directory, targets, &config)
}

func buildSettings(options []Option) (settings, error) {
	config := settings{runner: process.NewDefaultRunner()}
	for _, option := range options {
		option(&config)
	}
	if config.runner == nil || config.platform != "" && config.allPlatforms {
		return config, errUtils.ErrPluginInvalidConfig
	}
	return config, nil
}

type buildTarget struct {
	platform   platform.Target
	executable string
}

func buildTargets(definition *manifest.Config, config *settings) ([]buildTarget, error) {
	keys, err := buildPlatformKeys(definition, config)
	if err != nil {
		return nil, err
	}
	targets := make([]buildTarget, 0, len(keys))
	outputs := make(map[string]bool)
	for _, key := range keys {
		target, err := resolveBuildTarget(definition, key, config.platform != "" || config.allPlatforms)
		if err != nil {
			return nil, err
		}
		normalized := strings.ToLower(filepath.ToSlash(filepath.Clean(target.executable)))
		if outputConflict(outputs, normalized) {
			return nil, errUtils.ErrPluginInvalidConfig
		}
		outputs[normalized] = true
		targets = append(targets, target)
	}
	return targets, nil
}

func buildPlatformKeys(definition *manifest.Config, config *settings) ([]string, error) {
	if config.allPlatforms {
		keys := slices.Sorted(maps.Keys(definition.Spec.Platforms))
		if len(keys) == 0 {
			return nil, errUtils.ErrPluginInvalidConfig
		}
		return keys, nil
	}
	if config.platform != "" {
		return []string{config.platform}, nil
	}
	current := platform.Current()
	if len(definition.Spec.Platforms) == 0 {
		return []string{current}, nil
	}
	selected, err := platform.Select(definition.Spec.Platforms, current)
	if err != nil {
		return nil, err
	}
	return []string{selected}, nil
}

func resolveBuildTarget(definition *manifest.Config, key string, explicit bool) (buildTarget, error) {
	parsed, err := platform.Parse(key)
	if err != nil {
		return buildTarget{}, err
	}
	executable := definition.Spec.Runtime.Executable
	if selected, found := definition.Spec.Platforms[key]; found {
		executable = selected.Executable
	} else if explicit {
		return buildTarget{}, errUtils.ErrPluginUnsupportedPlatform
	}
	if !manifest.LocalPath(executable) {
		return buildTarget{}, errUtils.ErrPluginInvalidConfig
	}
	return buildTarget{platform: parsed, executable: executable}, nil
}

func outputConflict(outputs map[string]bool, candidate string) bool {
	if outputs[candidate] {
		return true
	}
	for existing := range outputs {
		if strings.HasPrefix(existing, candidate+"/") || strings.HasPrefix(candidate, existing+"/") {
			return true
		}
	}
	return false
}

// validateOutputPaths rejects aliases before any compiler can overwrite another
// target's output. Opening the source root permits a symlink to the project itself.
func validateOutputPaths(directory string, targets []buildTarget) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, target := range targets {
		if err := validateOutputPath(root, target.executable); err != nil {
			return err
		}
	}
	return nil
}

func validateOutputPath(root *os.Root, executable string) error {
	relative := ""
	for _, component := range strings.Split(filepath.ToSlash(filepath.Clean(executable)), "/") {
		relative = filepath.Join(relative, component)
		info, err := root.Lstat(relative)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errUtils.Build(errUtils.ErrPluginInvalidConfig).
				WithHintf("Build output %q contains a symbolic link at %q. Use distinct output paths without symbolic links.", executable, relative).Err()
		}
	}
	return nil
}

func validateSource(directory string) (*manifest.Config, error) {
	definition, err := manifest.Load(directory)
	if err != nil {
		return nil, err
	}
	if _, err := manifest.ReadFile(directory, "go.mod"); err != nil {
		return nil, errUtils.Build(errUtils.ErrPluginBuild).WithCause(err).
			WithHint("Run this command in a Go plugin source directory containing go.mod and plugin.yaml.").Err()
	}
	registry, err := bootstrap.Descriptors()
	if err != nil {
		return nil, err
	}
	if _, err := registry.Validate(directory, definition.Spec.Capabilities); err != nil {
		return nil, err
	}
	return definition, nil
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
	if !config.prepared {
		if err := prepareModule(ctx, directory, config); err != nil {
			return err
		}
		config.prepared = true
	}
	result := config.runner.Run(ctx, process.TaskSpec{
		Command: "go", Args: []string{"build", "-o", output.Name(), "."},
		Dir: directory, Streams: config.streams, Env: config.environment,
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
		Command: "go", Args: []string{"env", "GOWORK"}, Dir: directory, Env: config.environment,
		Streams: process.Streams{Stdout: &workspace, Stderr: config.streams.Stderr},
	})
	if err := buildResult(&result); err != nil {
		return err
	}
	if path := strings.TrimSpace(workspace.String()); path != "" && path != "off" {
		return nil
	}
	result = config.runner.Run(ctx, process.TaskSpec{Command: "go", Args: []string{"mod", "tidy"}, Dir: directory, Env: config.environment, Streams: config.streams})
	return buildResult(&result)
}
