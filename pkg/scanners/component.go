package scanners

import (
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

func ComponentPath(ctx *Context) string {
	defer perf.Track(nil, "scanners.ComponentPath")()

	if ctx == nil || ctx.AtmosConfig == nil || ctx.Info == nil {
		wd, _ := os.Getwd()
		return wd
	}

	if path, exists, err := component.BuildAndResolveWorkdirPath(ctx.AtmosConfig, ctx.Info, cfg.TerraformComponentType); err == nil && exists && path != "" {
		return path
	}

	base := ctx.AtmosConfig.TerraformDirAbsolutePath
	if base == "" {
		wd, _ := os.Getwd()
		return wd
	}
	finalComponent := ctx.Info.FinalComponent
	if finalComponent == "" {
		finalComponent = ctx.Info.ComponentFromArg
	}
	if finalComponent == "" {
		finalComponent = ctx.Info.Component
	}
	return filepath.Join(base, ctx.Info.ComponentFolderPrefix, finalComponent)
}
