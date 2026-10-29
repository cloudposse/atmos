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

func resolve(ctx context.Context, request model.Request, filesystem afero.Fs) (*model.Resolution, error) {
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
	path, err = canonicalPath(path)
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
			pctx.WithDecodeList(config.DependenciesBlock, config.DependencyBlock, config.TerraformSource), path, nil)
		if err != nil {
			return nil, err
		}
		result := &model.Resolution{Path: path, Files: append(pctx.FilesRead.Paths(), path)}
		if workspace := request.Environment["TF_WORKSPACE"]; workspace != "" && workspace != "default" && len(cfg.TerragruntDependencies) > 0 && cfg.Dependencies != nil && len(cfg.Dependencies.Paths) > 0 {
			return nil, fmt.Errorf("native output dependencies with non-default workspaces require a dependency backend adapter: %s", path)
		}
		if cfg.Terraform != nil && cfg.Terraform.Source != nil {
			result.Folders = []string{localPath(filepath.Dir(path), *cfg.Terraform.Source)}
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
				physical, err := canonicalPath(dependency)
				if err != nil {
					return nil, err
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
					return nil, err
				}
				if info.IsDir() {
					dependency = filepath.Join(dependency, "terragrunt.hcl")
				}
				physical, err := canonicalPath(dependency)
				if err != nil {
					return nil, fmt.Errorf("native adapter dependency must target a unit: %w", err)
				}
				result.Dependencies = append(result.Dependencies, physical)
			}
		}
		return result, nil
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
		return nil, fmt.Errorf("native adapter requires terraform.source")
	}
	if request.Phase != "backend" {
		if err := checkVersions(ctx, request, cfg.TerraformVersionConstraint, cfg.TerragruntVersionConstraint); err != nil {
			return nil, err
		}
	}
	if cfg.TerraformBinary != "" {
		return nil, fmt.Errorf("native adaptation of terraform_binary is not implemented")
	}
	if len(cfg.Terraform.BeforeHooks)+len(cfg.Terraform.AfterHooks)+len(cfg.Terraform.ErrorHooks)+len(cfg.Terraform.ExtraArgs) > 0 {
		return nil, fmt.Errorf("native adaptation of hooks/extra_arguments is not implemented")
	}
	if cfg.PreventDestroy != nil && *cfg.PreventDestroy {
		return nil, fmt.Errorf("native adaptation of prevent_destroy is not implemented")
	}
	if cfg.Errors != nil || cfg.Exclude != nil || len(cfg.FeatureFlags) > 0 || cfg.IamRole != "" || cfg.IamWebIdentityToken != "" {
		return nil, fmt.Errorf("native adaptation of errors, exclude, feature flags, or IAM settings is not implemented")
	}
	result := &model.Resolution{Path: path, Inputs: cfg.Inputs, Source: *cfg.Terraform.Source, Generated: map[string]string{}, Files: append(pctx.FilesRead.Paths(), path)}
	if !plainLocalSource(result.Source) {
		return nil, fmt.Errorf("native adapter only adapts plain local directory sources")
	}
	if !filepath.IsAbs(result.Source) {
		result.Source = filepath.Join(filepath.Dir(path), result.Source)
	}
	if info, err := os.Stat(result.Source); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("native adapter source must be an existing local directory: %s", result.Source)
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
	result.Folders = []string{result.Source}
	result.Files = append(result.Files, request.OriginFiles...)
	result.Folders = append(result.Folders, request.OriginFolders...)
	if cfg.RemoteState != nil {
		result.BackendType = cfg.RemoteState.BackendName
		result.Backend = cfg.RemoteState.BackendConfig
		if result.BackendType != "local" {
			return nil, fmt.Errorf("native adapter only adapts the local backend")
		}
		// Local backend paths are relative to Terragrunt's working directory,
		// including the separate non-default-workspace state directory.
		for key, fallback := range map[string]string{"path": "terraform.tfstate", "workspace_dir": "terraform.tfstate.d"} {
			value, _ := result.Backend[key].(string)
			if value == "" {
				value = fallback
			}
			result.Backend[key] = localPath(result.WorkingDir, value)
		}
	}
	for _, generated := range cfg.GenerateConfigs {
		if generated.Disable {
			continue
		}
		if filepath.Base(generated.Path) != generated.Path || !strings.HasSuffix(generated.Path, ".tf") {
			return nil, fmt.Errorf("unsupported generated path: %s", generated.Path)
		}
		if generated.IfExistsStr != "overwrite" || !generated.DisableSignature {
			return nil, fmt.Errorf("native adapter generation requires overwrite and disable_signature")
		}
		result.Generated[generated.Path] = generated.Contents
	}
	return result, nil
}
