package conformance

import (
	"context"
	"fmt"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginplatform "github.com/cloudposse/atmos/pkg/plugin/platform"
	"github.com/cloudposse/atmos/pkg/process"
)

// SourceRequest describes optional source preparation before native conformance.
type SourceRequest struct {
	Directory    string
	Platform     string
	Build        bool
	CoverProfile string
	Streams      process.Streams
}

// SourceRunner separates authoring-runtime operations from protocol conformance.
// Go is the default; alternate runtimes can supply their own implementation.
//
//go:generate go run go.uber.org/mock/mockgen -source=source.go -destination=mock_source_test.go -package=conformance
type SourceRunner interface {
	Run(context.Context, *SourceRequest) error
}

func validateSourceOptions(options *Options) error {
	if options.Platform != "" {
		if err := validateTestPlatform(options.Platform, pluginplatform.Current()); err != nil {
			return err
		}
	}
	if options.CoverProfile != "" && !manifest.LocalPath(options.CoverProfile) {
		return fmt.Errorf("%w: --coverprofile must be a package-relative path", errUtils.ErrPluginInvalidConfig)
	}
	return nil
}

func prepareSource(ctx context.Context, directory string, definition *manifest.Config, options *Options, target *suite) error {
	platform := options.Platform
	if platform != "" {
		selected, found := definition.Spec.Platforms[platform]
		if !found {
			return fmt.Errorf("%w: test platform %s is not declared by plugin.yaml", errUtils.ErrPluginUnsupportedPlatform, platform)
		}
		target.executable = filepath.Join(directory, selected.Executable)
	} else if len(definition.Spec.Platforms) != 0 {
		selected, err := pluginplatform.Select(definition.Spec.Platforms, pluginplatform.Current())
		if err != nil {
			return err
		}
		platform = selected
	}
	if !options.Build && options.CoverProfile == "" {
		return nil
	}
	runner := options.SourceRunner
	if runner == nil {
		runner = GoSourceRunner{}
	}
	return runner.Run(ctx, &SourceRequest{
		Directory: directory, Platform: platform, Build: options.Build, CoverProfile: options.CoverProfile,
		Streams: process.Streams{Stdout: options.Stdout, Stderr: options.Stderr},
	})
}

func validateTestPlatform(requested, host string) error {
	if _, err := pluginplatform.Select(map[string]bool{requested: true}, host); err != nil {
		return fmt.Errorf("%w: cannot execute requested test platform %s on host %s", err, requested, host)
	}
	return nil
}
