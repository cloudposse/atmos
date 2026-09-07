package asciicast

import (
	"fmt"
	"os/exec"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependencies"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

const (
	aggTool       = "asciinema/agg"
	aggVersion    = "1.9.0"
	ffmpegTool    = "Tyrrrz/FFmpegBin"
	ffmpegVersion = "8.1.2"
)

// renderTools holds the absolute paths to the managed animated renderers.
// Static render targets leave both fields empty.
type renderTools struct {
	agg    string
	ffmpeg string
}

type renderToolRequirements struct {
	agg    bool
	ffmpeg bool
}

type renderToolSpec struct {
	dependency string
	version    string
	binary     string
}

var (
	resolveRenderTools = resolveRenderToolsFromToolchain

	ensureRenderToolDependencies = func(deps map[string]string) error {
		return dependencies.NewInstaller(toolchain.GetAtmosConfig()).EnsureTools(deps)
	}
	findRenderToolBinary = func(spec renderToolSpec) (string, error) {
		installer := toolchain.NewInstaller()
		resolver := installer.GetResolver()
		owner, repo, err := resolver.Resolve(spec.dependency)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", spec.dependency, err)
		}
		path, err := installer.FindBinaryPath(owner, repo, spec.version, spec.binary)
		if err != nil {
			return "", fmt.Errorf("locate %s: %w", spec.dependency, err)
		}
		return path, nil
	}
)

func renderToolRequirementsForTargets(targets []renderTarget) renderToolRequirements {
	var requirements renderToolRequirements
	for _, target := range targets {
		switch target.format {
		case renderFormatGIF:
			requirements.agg = true
		case renderFormatMP4:
			requirements.agg = true
			requirements.ffmpeg = true
		}
	}
	return requirements
}

func renderToolSpecs(requirements renderToolRequirements) []renderToolSpec {
	specs := make([]renderToolSpec, 0, 2)
	if requirements.agg {
		specs = append(specs, renderToolSpec{dependency: aggTool, version: aggVersion, binary: "agg"})
	}
	if requirements.ffmpeg {
		specs = append(specs, renderToolSpec{dependency: ffmpegTool, version: ffmpegVersion, binary: "ffmpeg"})
	}
	return specs
}

func resolveRenderToolsFromToolchain(requirements renderToolRequirements) (renderTools, error) {
	specs := renderToolSpecs(requirements)
	if len(specs) == 0 {
		return renderTools{}, nil
	}

	deps := make(map[string]string, len(specs))
	for _, spec := range specs {
		deps[spec.dependency] = spec.version
	}
	if err := ensureRenderToolDependencies(deps); err != nil {
		return renderTools{}, fmt.Errorf("%w: install managed cast renderers: %w", errUtils.ErrToolInstall, err)
	}

	tools := renderTools{}
	for _, spec := range specs {
		path, err := findRenderToolBinary(spec)
		if err != nil {
			return renderTools{}, fmt.Errorf("%w: %w", errUtils.ErrToolInstall, err)
		}
		if !filepath.IsAbs(path) {
			path, err = filepath.Abs(path)
			if err != nil {
				return renderTools{}, fmt.Errorf("%w: resolve absolute path for %s: %w", errUtils.ErrToolInstall, spec.dependency, err)
			}
		}
		switch spec.binary {
		case "agg":
			tools.agg = path
		case "ffmpeg":
			tools.ffmpeg = path
		}
	}
	return tools, nil
}

func runRenderer(binary string, args ...string) error {
	if binary == "" {
		return fmt.Errorf("%w: managed renderer path is empty", errUtils.ErrToolInstall)
	}
	//nolint:gosec // The command path is resolved from the managed toolchain and arguments are file paths.
	cmd := exec.Command(binary, args...)
	// Route the renderer's own output through the Atmos IO layer instead of wiring
	// os.Stdout/os.Stderr directly, so masking/test capture keep working: any file
	// data the tool prints goes to the data channel, progress/log lines to the UI channel.
	cmd.Stdout = iolib.GetContext().Data()
	cmd.Stderr = iolib.GetContext().UI()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s: %w", errUtils.ErrRenderToolExecFailed, binary, err)
	}
	return nil
}
