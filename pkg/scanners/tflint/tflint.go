package tflint

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	gitlib "github.com/go-git/go-git/v5"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/scanners"
	"github.com/cloudposse/atmos/pkg/scanners/sarif"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	Name    = "tflint"
	Command = "tflint"

	OutputFormatMarkdown = "markdown"
	OutputFormatRich     = "rich"
)

// DefaultArgs returns tflint's default CLI arguments. TFLint 0.47+ dropped support for
// positional [FILE or DIR...] arguments ("Command line arguments support was dropped in
// v0.47. Use --chdir or --filter instead."), so $ATMOS_COMPONENT_PATH must be passed via
// --chdir rather than positionally.
func DefaultArgs() []string {
	defer perf.Track(nil, "tflint.DefaultArgs")()

	return []string{
		"--format=sarif",
		"--chdir=$ATMOS_COMPONENT_PATH",
	}
}

type Options struct {
	Args          []string
	Env           map[string]string
	BaseEnv       []string
	OnFailure     string
	AtmosConfig   *schema.AtmosConfiguration
	Info          *schema.ConfigAndStacksInfo
	ToolchainPATH string
	// MaxFindings caps how many individual findings appear in the table per
	// component (0 uses sarif's own default of 10). See cmd/terraform/lint.go's
	// --max-findings flag.
	MaxFindings int
	// OutputFormat controls Atmos's presentation of findings. TFLint still
	// always receives --format=sarif so artifacts and CI reports stay stable.
	OutputFormat string
}

func Run(ctx context.Context, opts *Options) (*scanners.Output, *scanners.Context, error) {
	defer perf.Track(nil, "scanners.tflint.Run")()

	if opts == nil {
		opts = &Options{}
	}
	args := opts.Args
	if len(args) == 0 {
		args = DefaultArgs()
	}
	args = ResolveArgs(args, opts.AtmosConfig, opts.Info)

	scan := &scanners.Context{
		Name:          Name,
		Command:       Command,
		Args:          append([]string(nil), args...),
		Env:           opts.Env,
		BaseEnv:       opts.BaseEnv,
		OnFailure:     opts.OnFailure,
		CaptureStdout: true,
		AtmosConfig:   opts.AtmosConfig,
		Info:          opts.Info,
		ToolchainPATH: opts.ToolchainPATH,
		ResultHandler: sarif.NewResultHandler(sarif.HandlerOptions{
			Kind:           Name,
			OutputPath:     sarif.DefaultOutputFile,
			MaxFindings:    opts.MaxFindings,
			TerminalFormat: opts.OutputFormat,
		}),
	}

	out, err := scanners.Run(ctx, scan)
	return out, scan, err
}

// ResolveArgs adds TFLint's configuration file when the caller did not
// explicitly provide one. Config discovery uses the closest standard location:
// component directory, Terraform components base path, then repository root.
// The project-level components.terraform.lint.config setting is a final fallback.
func ResolveArgs(args []string, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) []string {
	defer perf.Track(atmosConfig, "tflint.ResolveArgs")()

	resolved := append([]string(nil), args...)
	if hasConfigArg(resolved) {
		return resolved
	}

	if configPath := ConfigPath(atmosConfig, info); configPath != "" {
		return append(resolved, "--config="+configPath)
	}
	return resolved
}

// ConfigPath finds the applicable TFLint configuration. The closest config wins:
// the component directory, then the Terraform components base path, then the
// repository root. This lets shared components override project-wide rules.
func ConfigPath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(atmosConfig, "tflint.ConfigPath")()

	if atmosConfig == nil || info == nil {
		return ""
	}

	for _, directory := range configDirectories(atmosConfig, info) {
		configPath := filepath.Join(directory, ".tflint.hcl")
		if fileExists(configPath) {
			return configPath
		}
	}

	global := strings.TrimSpace(atmosConfig.Components.Terraform.Lint.Config)
	if global == "" {
		return ""
	}
	if !filepath.IsAbs(global) {
		global = filepath.Join(atmosConfig.BasePathAbsolute, global)
	}
	return filepath.Clean(global)
}

func configDirectories(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) []string {
	directories := []string{
		scanners.ComponentPath(&scanners.Context{AtmosConfig: atmosConfig, Info: info}),
		atmosConfig.TerraformDirAbsolutePath,
		repositoryRoot(atmosConfig.BasePathAbsolute),
	}

	seen := make(map[string]struct{}, len(directories))
	result := make([]string, 0, len(directories))
	for _, directory := range directories {
		directory = filepath.Clean(directory)
		if directory == "." || directory == "" {
			continue
		}
		if _, ok := seen[directory]; ok {
			continue
		}
		seen[directory] = struct{}{}
		result = append(result, directory)
	}
	return result
}

// repositoryRoot resolves the Git worktree from the Atmos base path. Projects
// without a Git worktree use the Atmos base path as their repository root.
func repositoryRoot(basePath string) string {
	if basePath == "" {
		return ""
	}
	repo, err := gitlib.PlainOpenWithOptions(basePath, &gitlib.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	})
	if err != nil {
		return basePath
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return basePath
	}
	root, err := filepath.Abs(worktree.Filesystem.Root())
	if err != nil {
		return basePath
	}
	return root
}

func hasConfigArg(args []string) bool {
	for _, arg := range args {
		if arg == "--config" || strings.HasPrefix(arg, "--config=") {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
