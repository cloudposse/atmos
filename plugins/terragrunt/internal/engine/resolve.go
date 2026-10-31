// Package engine evaluates Terragrunt configuration outside Atmos core.
package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/spf13/afero"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// Resolve evaluates HCL using the pinned Terragrunt parser.
func Resolve(ctx context.Context, request model.Request) (*model.Resolution, error) {
	return resolve(ctx, request, nil)
}

// resolve evaluates one request and describes any rejection as a model.Problem the user can act on.
func resolve(ctx context.Context, request model.Request, filesystem afero.Fs) (*model.Resolution, error) {
	result, err := resolveUnit(ctx, request, filesystem)
	if err != nil {
		return nil, problemFor(err, request.Path)
	}
	return result, nil
}

func resolveUnit(ctx context.Context, request model.Request, filesystem afero.Fs) (*model.Resolution, error) {
	if request.Phase == "inventory" {
		return discoverProject(ctx, request)
	}
	if request.Phase == "backend" && request.GenerationRoot != "" {
		filesystem = afero.NewCopyOnWriteFs(afero.NewReadOnlyFs(afero.NewOsFs()), afero.NewMemMapFs())
		generation := request
		generation.Path, generation.Phase, generation.Command = request.GenerationRoot, "discover", "generate"
		if _, err := resolve(ctx, generation, filesystem); err != nil {
			return nil, err
		}
	}
	path, err := filepath.Abs(strings.TrimPrefix(request.Path, "terragrunt://"))
	if err != nil {
		return nil, err
	}
	// Use one physical path identity for parsing and CLI execution. On macOS,
	// mixing /var and /private/var breaks upstream diamond dependency reads.
	path, err = logicalPath(path)
	if err != nil {
		return nil, err
	}
	ctx = config.WithCaches(ctx)
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)
	logger := log.New(log.WithOutput(io.Discard), log.WithFormatter(formatter))
	if request.LogLevel != "" {
		if err := logger.SetLevel(request.LogLevel); err != nil {
			return nil, err
		}
	}

	// The local maintained facade exposes the pinned upstream virtual environment
	// and recursive stack generation without modifying upstream source.
	environment := atmosbridge.Environment(request.Environment, io.Discard, io.Discard)
	if filesystem != nil {
		environment = environment.WithFS(filesystem)
	}
	if _, err := environment.FS.Stat(path); err != nil {
		return nil, err
	}
	discover := request.Phase == "discover" || request.Command == "discover"
	if discover || request.Phase == "backend" || request.Command == "generate" {
		environment = atmosbridge.Discovery(environment, request.Command == "generate")
	}
	command := request.Command
	if command == "" || command == "discover" || command == "generate" {
		command = "plan"
	}
	pctx := config.NewParsingContext().WithFileReadTracking()
	pctx.WorkingDir, pctx.RootWorkingDir = filepath.Dir(path), filepath.Dir(path)
	pctx.TerragruntConfigPath, pctx.OriginalTerragruntConfigPath = path, path
	pctx.TFPath, pctx.TerraformCommand = request.Tofu, command
	pctx.OriginalTerraformCommand = command
	pctx.TerraformCliArgs.SetCommand(command).AppendArgument(request.Arguments...)
	pctx.DownloadDir = filepath.Join(filepath.Dir(path), ".terragrunt-cache")
	pctx.AutoInit = true
	pctx.MaxFoldersToCheck = options.DefaultMaxFoldersToCheck
	if filepath.Base(path) == "terragrunt.stack.hcl" {
		if request.Command == "generate" {
			if err := atmosbridge.Generate(ctx, logger, environment, path, request.Tofu); err != nil {
				return nil, err
			}
		}
		values, err := config.ReadValues(ctx, logger, environment, pctx, filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		stack, err := config.ReadStackConfigFile(ctx, logger, environment, pctx, path, values)
		if err != nil {
			return nil, err
		}
		units := map[string]string{}
		unitSources := map[string]string{}
		unitConfigs := map[string]string{}
		for _, unit := range stack.Units {
			if !unit.IsEnabled() {
				continue
			}
			units[unit.Name] = unit.Path
			unitSources[unit.Name] = localPath(filepath.Dir(path), unit.Source)
			unitConfigs[unit.Name] = filepath.Join(unit.GeneratedPath(filepath.Dir(path)), "terragrunt.hcl")
		}
		stacks := map[string]string{}
		stackSources := map[string]string{}
		stackConfigs := map[string]string{}
		for _, child := range stack.Stacks {
			if !child.IsEnabled() {
				continue
			}
			stacks[child.Name] = child.Path
			stackSources[child.Name] = localPath(filepath.Dir(path), child.Source)
			stackConfigs[child.Name] = filepath.Join(child.GeneratedPath(filepath.Dir(path)), "terragrunt.stack.hcl")
		}
		return &model.Resolution{Path: path, Files: append(pctx.FilesRead.Paths(), path), Units: units, Stacks: stacks, UnitConfigs: unitConfigs, StackConfigs: stackConfigs, UnitSources: unitSources, StackSources: stackSources}, nil
	}
	if discover {
		cfg, err := config.PartialParseConfigFile(ctx, logger, environment,
			pctx.WithDecodeList(config.DependenciesBlock, config.DependencyBlock, config.TerraformSource, config.TerraformBlock, config.TerragruntFlags, config.TerragruntVersionConstraints, config.FeatureFlagsBlock, config.ExcludeBlock, config.ErrorsBlock), path, nil)
		if err != nil {
			return nil, err
		}
		// Reject unsupported settings now, so every selected unit is validated before any
		// infrastructure command runs.
		if err := rejectUnsupported(cfg, path); err != nil {
			return nil, err
		}
		result := &model.Resolution{Path: path, Files: append(pctx.FilesRead.Paths(), path)}
		if workspace := request.Environment["TF_WORKSPACE"]; workspace != "" && workspace != "default" && len(cfg.TerragruntDependencies) > 0 && cfg.Dependencies != nil && len(cfg.Dependencies.Paths) > 0 {
			return nil, workspaceDependencyProblem(path, workspace)
		}
		if cfg.Terraform != nil && cfg.Terraform.Source != nil {
			result.Source = *cfg.Terraform.Source
			if plainLocalSource(result.Source) {
				result.Source = localPath(filepath.Dir(path), result.Source)
				result.Folders = []string{result.Source}
			}
		}
		for _, dep := range cfg.TerragruntDependencies {
			if dep.Enabled != nil && !*dep.Enabled {
				continue
			}
			if dep.SkipOutputs != nil && *dep.SkipOutputs {
				continue
			}
			if dep.ConfigPath.IsKnown() && !dep.ConfigPath.IsNull() {
				dependency := localPath(filepath.Dir(path), dep.ConfigPath.AsString())
				if filepath.Ext(dependency) != ".hcl" {
					dependency = filepath.Join(dependency, "terragrunt.hcl")
				}
				physical, err := logicalPath(dependency)
				if err != nil {
					return nil, dependencyProblem(path, dependency, err)
				}
				if _, err := environment.FS.Stat(physical); err != nil {
					return nil, dependencyProblem(path, dependency, err)
				}
				result.OutputDependencies = append(result.OutputDependencies, physical)
			}
		}
		if cfg.Dependencies != nil {
			for _, dependency := range cfg.Dependencies.Paths {
				if !filepath.IsAbs(dependency) {
					dependency = filepath.Join(filepath.Dir(path), dependency)
				}
				info, err := environment.FS.Stat(dependency)
				if err != nil {
					return nil, dependencyProblem(path, dependency, err)
				}
				if info.IsDir() {
					dependency = filepath.Join(dependency, "terragrunt.hcl")
				}
				physical, err := logicalPath(dependency)
				if err != nil {
					return nil, dependencyProblem(path, dependency, err)
				}
				result.Dependencies = append(result.Dependencies, physical)
			}
		}
		return result, nil
	}
	if request.Phase != "backend" && atmosbridge.IsDestroyOperation(request.Command, request.Arguments) {
		guard, err := config.PartialParseConfigFile(ctx, logger, atmosbridge.Discovery(environment, false),
			pctx.WithDecodeList(config.TerragruntFlags), path, nil)
		if err != nil {
			return nil, err
		}
		if guard.PreventDestroy != nil && *guard.PreventDestroy {
			return nil, fmt.Errorf("unit is protected by prevent_destroy")
		}
	}
	var cfg *config.TerragruntConfig
	if request.Phase == "backend" {
		cfg, err = config.PartialParseConfigFile(ctx, logger, environment, pctx.WithDecodeList(config.TerraformSource, config.RemoteStateBlock), path, nil)
	} else {
		cfg, err = config.ParseConfigFile(ctx, logger, environment, pctx, path, nil)
	}
	if err != nil {
		return nil, err
	}
	if cfg.Terraform == nil || cfg.Terraform.Source == nil {
		return nil, missingSourceProblem(path)
	}
	if request.Phase != "backend" {
		if err := checkVersions(ctx, request, cfg.TerraformVersionConstraint, cfg.TerragruntVersionConstraint); err != nil {
			return nil, err
		}
	}
	if err := rejectUnsupported(cfg, path); err != nil {
		return nil, err
	}
	// Hooks and extra arguments are returned as data for the host to run; evaluation never
	// executes them. Backend identity is read from a partial parse that does not decode them.
	var execution *model.Execution
	if request.Phase != "backend" {
		if execution, err = executionFromConfig(cfg.Terraform); err != nil {
			return nil, err
		}
	}
	if cfg.PreventDestroy != nil && *cfg.PreventDestroy && atmosbridge.IsDestroyOperation(request.Command, request.Arguments) {
		return nil, fmt.Errorf("unit is protected by prevent_destroy")
	}
	result := &model.Resolution{Execution: execution, Path: path, Inputs: cfg.Inputs, Source: *cfg.Terraform.Source, Generated: map[string]string{}, Files: append(pctx.FilesRead.Paths(), path)}
	if plainLocalSource(result.Source) {
		result.Source = localPath(filepath.Dir(path), result.Source)
		if info, err := os.Stat(result.Source); err != nil || !info.IsDir() {
			return nil, sourceDirectoryProblem(path, *cfg.Terraform.Source)
		}
		result.Folders = []string{result.Source}
	} else if err := validateRemoteSource(result.Source); err != nil {
		return nil, err
	}
	download := pctx.DownloadDir
	if cfg.DownloadDir != "" {
		download = localPath(filepath.Dir(path), cfg.DownloadDir)
	}
	if configured := request.Environment["TG_DOWNLOAD_DIR"]; configured != "" {
		download = localPath(filepath.Dir(path), configured)
	}
	result.WorkingDir, err = atmosbridge.WorkingDir(logger, environment, result.Source, download, filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	result.Files = append(result.Files, request.OriginFiles...)
	result.Folders = append(result.Folders, request.OriginFolders...)
	if err := resolveBackend(cfg, result); err != nil {
		return nil, err
	}

	if err := generatedFiles(ctx, logger, cfg, result); err != nil {
		return nil, err
	}
	return result, nil
}
