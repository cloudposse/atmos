package step

import (
	"context"
	"fmt"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
)

// ScriptToolInstaller binds dependency installation to the invocation's configuration.
func ScriptToolInstaller(config *schema.AtmosConfiguration) script.ToolInstaller {
	defer perf.Track(nil, "step.ScriptToolInstaller")()

	return func(ctx context.Context, tools map[string]string) ([]string, error) {
		if config == nil {
			return nil, fmt.Errorf("%w: tool installation requires Atmos configuration", errUtils.ErrScript)
		}
		// The native toolchain and stack resolvers share configuration caches.
		select {
		case scriptResolutionGate <- struct{}{}:
			defer func() { <-scriptResolutionGate }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		environment, err := dependencies.NewEnvironmentFromDeps(config, tools)
		if err != nil {
			return nil, err
		}
		return environment.ToolchainDirs(), ctx.Err()
	}
}

// ScriptProjectRoot returns the absolute Atmos project base path used to show script paths relative
// to the project in errors, or "" when it is unknown. An already-absolute base path wins; a relative
// one is made absolute against the working directory, like the other runtime paths.
func ScriptProjectRoot(basePathAbsolute, basePath string) string {
	defer perf.Track(nil, "step.ScriptProjectRoot")()

	if basePathAbsolute != "" {
		return basePathAbsolute
	}
	if basePath == "" {
		return ""
	}
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return ""
	}
	return abs
}

// scriptProjectRoot returns the project base path of the step's Atmos configuration, if any.
func scriptProjectRoot(vars *Variables) string {
	if vars == nil || vars.AtmosConfig == nil {
		return ""
	}
	return ScriptProjectRoot(vars.AtmosConfig.BasePathAbsolute, vars.AtmosConfig.BasePath)
}
