// Package engine evaluates Terragrunt configuration outside Atmos core.
package engine

import (
	"context"
	"errors"
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

// ErrPreventDestroy rejects a destroy operation on a unit that sets prevent_destroy.
var ErrPreventDestroy = errors.New("unit is protected by prevent_destroy")

func resolve(ctx context.Context, request model.Request, filesystem afero.Fs) (*model.Resolution, error) {
	if request.Phase == "inventory" {
		return discoverProject(ctx, request)
	}
	filesystem, err := backendGenerationOverlay(ctx, request, filesystem)
	if err != nil {
		return nil, err
	}
	path, err := requestPath(request.Path)
	if err != nil {
		return nil, err
	}
	ctx = config.WithCaches(ctx)
	logger, err := evaluationLogger(request.LogLevel)
	if err != nil {
		return nil, err
	}
	environment, err := evaluationEnvironment(request, filesystem, path)
	if err != nil {
		return nil, err
	}
	pctx := requestParsingContext(request, path)
	if filepath.Base(path) == stackFile {
		return resolveStack(ctx, logger, environment, pctx, request, path)
	}
	if discovering(request) {
		return resolveUnitInventory(ctx, logger, environment, pctx, request, path)
	}
	if err := checkDestroyGuard(ctx, logger, environment, pctx, request, path); err != nil {
		return nil, err
	}
	cfg, err := parseUnit(ctx, logger, environment, pctx, request, path)
	if err != nil {
		return nil, err
	}
	if err := checkUnitPolicies(ctx, request, cfg); err != nil {
		return nil, err
	}
	execution, applyFiles, err := unitExecution(ctx, logger, environment, pctx, request, cfg, path)
	if err != nil {
		return nil, err
	}
	result, err := unitResolution(logger, environment, pctx, request, cfg, path)
	if err != nil {
		return nil, err
	}
	result.Execution = execution
	result.Files = append(result.Files, applyFiles...)
	if err := resolveBackend(cfg, result); err != nil {
		return nil, err
	}
	if request.Phase != "backend" {
		result.Overlay, err = atmosbridge.UnitOverlay(logger, environment, filepath.Dir(path), cfg.Terraform)
		if err != nil {
			return nil, err
		}
	}
	if err := generatedFiles(ctx, logger, cfg, result); err != nil {
		return nil, err
	}
	return result, nil
}

const (
	stackFile = "terragrunt.stack.hcl"
	unitFile  = "terragrunt.hcl"
	// Nested-stack recursion for generation and discovery stops at maxStackDepth.
	maxStackDepth = 32
)

// backendGenerationOverlay generates the owning stack into an invocation-local
// copy-on-write overlay before a backend read of a stack-generated unit. The
// returned filesystem is the one the unit is then parsed from.
func backendGenerationOverlay(ctx context.Context, request model.Request, filesystem afero.Fs) (afero.Fs, error) {
	if request.Phase != "backend" || request.GenerationRoot == "" {
		return filesystem, nil
	}
	filesystem = afero.NewCopyOnWriteFs(afero.NewReadOnlyFs(afero.NewOsFs()), afero.NewMemMapFs())
	generation := request
	generation.Path, generation.Phase, generation.Command = request.GenerationRoot, "discover", "generate"
	if _, err := resolve(ctx, generation, filesystem); err != nil {
		return nil, err
	}
	return filesystem, nil
}

// requestPath uses one physical path identity for parsing and CLI execution. On
// macOS, mixing /var and /private/var breaks upstream diamond dependency reads.
func requestPath(path string) (string, error) {
	path, err := filepath.Abs(strings.TrimPrefix(path, "terragrunt://"))
	if err != nil {
		return "", err
	}
	return canonicalPath(path)
}

func evaluationLogger(level string) (log.Logger, error) {
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)
	logger := log.New(log.WithOutput(io.Discard), log.WithFormatter(formatter))
	if level != "" {
		if err := logger.SetLevel(level); err != nil {
			return nil, err
		}
	}
	return logger, nil
}

func discovering(request model.Request) bool {
	return request.Phase == "discover" || request.Command == "discover"
}

func generating(request model.Request) bool {
	return request.Command == "generate" && request.Phase != "backend"
}

// evaluationEnvironment uses the local maintained facade, which exposes the
// pinned upstream virtual environment and scoped stack generation without
// modifying upstream source.
func evaluationEnvironment(request model.Request, filesystem afero.Fs, path string) (*atmosbridge.Venv, error) {
	environment := atmosbridge.Environment(request.Environment, io.Discard, io.Discard)
	if filesystem != nil {
		environment = environment.WithFS(filesystem)
	}
	if _, err := environment.FS.Stat(path); err != nil {
		return nil, err
	}
	if discovering(request) || request.Phase == "backend" || generating(request) {
		environment = atmosbridge.Discovery(environment, generating(request))
	}
	return environment, nil
}

// requestParsingContext evaluates the unit at path as the request's command;
// discovery and generation evaluate as plan.
func requestParsingContext(request model.Request, path string) *config.ParsingContext {
	command := request.Command
	if command == "" || command == "discover" || command == "generate" {
		command = "plan"
	}
	pctx := parsingContext(path, request.Tofu, command, request.Arguments)
	if configured := request.Environment["TG_DOWNLOAD_DIR"]; configured != "" {
		pctx.DownloadDir = localPath(filepath.Dir(path), configured)
	}
	return pctx
}

// parsingContext configures Terragrunt evaluation of the unit at path as the named command.
func parsingContext(path, tofu, command string, arguments []string) *config.ParsingContext {
	pctx := config.NewParsingContext().WithFileReadTracking()
	pctx.WorkingDir, pctx.RootWorkingDir = filepath.Dir(path), filepath.Dir(path)
	pctx.TerragruntConfigPath, pctx.OriginalTerragruntConfigPath = path, path
	pctx.TFPath, pctx.TerraformCommand = tofu, command
	pctx.OriginalTerraformCommand = command
	pctx.TerraformCliArgs.SetCommand(command).AppendArgument(arguments...)
	pctx.DownloadDir = filepath.Join(filepath.Dir(path), ".terragrunt-cache")
	pctx.AutoInit = true
	pctx.MaxFoldersToCheck = options.DefaultMaxFoldersToCheck
	return pctx
}

// checkDestroyGuard rejects destroy before a full parse can evaluate HCL functions
// for a unit that sets prevent_destroy. Backend reads never destroy.
func checkDestroyGuard(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, path string) error {
	if request.Phase == "backend" || !atmosbridge.IsDestroyOperation(request.Command, request.Arguments) {
		return nil
	}
	guard, err := config.PartialParseConfigFile(ctx, logger, atmosbridge.Discovery(environment, false),
		pctx.WithDecodeList(config.TerragruntFlags), path, nil)
	if err != nil {
		return err
	}
	if guard.PreventDestroy != nil && *guard.PreventDestroy {
		return ErrPreventDestroy
	}
	return nil
}

// unitExecution returns hooks and extra arguments as data for the host to run;
// evaluation never executes them. Backend identity is read from a partial parse
// that does not decode them.
func unitExecution(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, cfg *config.TerragruntConfig, path string) (*model.Execution, []string, error) {
	if request.Phase == "backend" {
		return nil, nil, nil
	}
	execution, err := executionFromConfig(cfg.Terraform)
	if err != nil {
		return nil, nil, err
	}
	reparse := func(apply *config.ParsingContext) (*config.TerragruntConfig, error) {
		return config.ParseConfigFile(ctx, logger, environment, apply, path, nil)
	}
	return applyTimeExecution(request, path, execution, environment.FS, pctx.FilesRead.Paths(), reparse)
}

func parseUnit(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, path string) (*config.TerragruntConfig, error) {
	if request.Phase == "backend" {
		return config.PartialParseConfigFile(ctx, logger, environment, pctx.WithDecodeList(config.TerraformSource, config.RemoteStateBlock), path, nil)
	}
	return config.ParseConfigFile(ctx, logger, environment, pctx, path, nil)
}

// unitResolution converts the parsed unit into its module identity. Local sources
// must be existing directories; remote sources keep their getter URI.
func unitResolution(logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, cfg *config.TerragruntConfig, path string) (*model.Resolution, error) {
	result := &model.Resolution{Path: path, Inputs: cfg.Inputs, Source: *cfg.Terraform.Source, Generated: map[string]string{}, Files: append(pctx.FilesRead.Paths(), path)}
	if plainLocalSource(result.Source) {
		result.Source = localPath(filepath.Dir(path), result.Source)
		if info, err := os.Stat(result.Source); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("native adapter source must be an existing local directory: %s", result.Source)
		}
		result.Folders = []string{result.Source}
	} else if err := validateRemoteSource(result.Source); err != nil {
		return nil, err
	}
	download := pctx.DownloadDir
	if cfg.DownloadDir != "" && request.Environment["TG_DOWNLOAD_DIR"] == "" {
		download = localPath(filepath.Dir(path), cfg.DownloadDir)
	}
	if configured := request.Environment["TG_DOWNLOAD_DIR"]; configured != "" {
		download = localPath(filepath.Dir(path), configured)
	}
	workingDir, err := atmosbridge.WorkingDir(logger, environment, result.Source, download, filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	result.WorkingDir = workingDir
	result.Files = append(result.Files, request.OriginFiles...)
	result.Folders = append(result.Folders, request.OriginFolders...)
	return result, nil
}
