// Package engine evaluates Terragrunt configuration outside Atmos core.
package engine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/spf13/afero"
	"github.com/zclconf/go-cty/cty"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// Resolve evaluates HCL using the pinned Terragrunt parser.
func Resolve(ctx context.Context, request model.Request) (*model.Resolution, error) {
	return resolve(ctx, request, nil)
}

// ErrPreventDestroy rejects a destroy operation on a unit that sets prevent_destroy.
var ErrPreventDestroy = errors.New("unit is protected by prevent_destroy")

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
	filesystem, err := backendGenerationOverlay(ctx, request, filesystem)
	if err != nil {
		return nil, err
	}
	path, err := requestPath(request.Path)
	if err != nil {
		return nil, err
	}
	// This fixed progress event is safe to show before configuration values are evaluated.
	if sink := logSinkFrom(ctx); sink != nil {
		sink.Event(log.InfoLevel, "Evaluating unit", filepath.Dir(path))
	}
	ctx = config.WithCaches(ctx)
	logger, err := evaluationLogger(ctx, request.LogLevel)
	if err != nil {
		return nil, err
	}
	environment, err := evaluationEnvironment(ctx, request, filesystem, path)
	if err != nil {
		return nil, err
	}
	pctx := requestParsingContext(request, path)
	redacted := false
	defer func() {
		if sink := logSinkFrom(ctx); sink.collecting() && unredactedSensitivity(environment.FS, append(pctx.FilesRead.Paths(), path), path, redacted) {
			sink.Withhold()
		}
	}()
	if request.Phase == "iam" {
		return resolveIAM(ctx, logger, environment, pctx, request, path)
	}
	if filepath.Base(path) == stackFile {
		return resolveStack(ctx, logger, environment, pctx, request, path)
	}
	if err := checkRequestFeatureOverrides(ctx, logger, environment, pctx, request, path); err != nil {
		return nil, err
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
	var sensitiveInputs [][]string
	if request.Phase != "backend" {
		sensitiveInputs = SensitiveInputs(environment.FS, append(pctx.FilesRead.Paths(), path), cfg.Inputs)
		redactInputs(logSinkFrom(ctx), cfg.Inputs, sensitiveInputs)
		redactInputs(logSinkFrom(ctx), cfg.Locals, sensitiveLocals(environment.FS, path, cfg.Locals))
		redacted = true
	}
	if err := checkUnitPolicies(ctx, request, cfg, path); err != nil {
		return nil, err
	}
	execution, applyFiles, err := unitExecution(ctx, logger, environment, pctx, request, cfg, path)
	if err != nil {
		return nil, err
	}
	result, err := unitResolution(ctx, logger, environment, pctx, request, cfg, path)
	if err != nil {
		return nil, err
	}
	result.Execution = execution
	result.SensitiveInputs = sensitiveInputs
	result.Files = append(result.Files, applyFiles...)
	if err := resolveBackend(cfg, result, &request); err != nil {
		return nil, err
	}
	if request.Phase != "backend" {
		result.Overlay, result.OverlayModes, err = atmosbridge.UnitOverlay(logger, environment, filepath.Dir(path), cfg.Terraform)
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
// copy-on-write overlay before a backend read or IAM probe of a stack-generated unit. The
// host probes IAM before the provider generates the stack, so on a clean checkout the unit
// exists only in this overlay. The returned filesystem is the one the unit is then parsed from.
func backendGenerationOverlay(ctx context.Context, request model.Request, filesystem afero.Fs) (afero.Fs, error) {
	if (request.Phase != "backend" && request.Phase != "iam") || request.GenerationRoot == "" {
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
// Like upstream Terragrunt, a symlinked terragrunt.hcl stays a unit where the link lives.
func requestPath(path string) (string, error) {
	path, err := filepath.Abs(strings.TrimPrefix(path, "terragrunt://"))
	if err != nil {
		return "", err
	}
	return logicalPath(path)
}

func evaluationLogger(ctx context.Context, level string) (log.Logger, error) {
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)
	options := []log.Option{log.WithOutput(io.Discard), log.WithFormatter(formatter)}
	if sink := logSinkFrom(ctx); sink != nil {
		options = append(options, log.WithHooks(sink.Hook()))
		level = sink.TerragruntLevel()
	}
	logger := log.New(options...)
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
func evaluationEnvironment(ctx context.Context, request model.Request, filesystem afero.Fs, path string) (*atmosbridge.Venv, error) {
	environment := atmosbridge.Environment(request.Environment, io.Discard, io.Discard)
	if filesystem != nil {
		environment = environment.WithFS(filesystem)
	}
	if _, err := environment.FS.Stat(path); err != nil {
		return nil, err
	}
	if discovering(request) || request.Phase == "backend" || generating(request) {
		environment = atmosbridge.Discovery(environment, generating(request))
	} else {
		environment = atmosbridge.AWS(ctx, environment)
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
	// Explicit feature overrides apply to every unit, as Terragrunt's --feature flag does.
	pctx.FeatureFlags = request.Features
	return pctx
}

// resolveIAM resolves the role before the full parse evaluates AWS functions. The host
// assumes it and supplies the resulting session on the second call.
func resolveIAM(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, path string) (*model.Resolution, error) {
	flags, err := config.PartialParseConfigFile(ctx, logger, environment,
		pctx.WithDecodeList(config.TerragruntFlags, config.ExcludeBlock), path, nil)
	if err != nil {
		return nil, err
	}
	if PreventsRun(excludeFromConfig(flags.Exclude), OperationCommand(request.Command, request.Arguments)) {
		return &model.Resolution{Path: path}, nil
	}
	iam, warnings, err := iamSettings(flags, &request, path)
	if err != nil {
		return nil, err
	}
	return &model.Resolution{Path: path, IAM: iam, Warnings: warnings}, nil
}

// checkRequestFeatureOverrides rejects feature overrides the unit does not declare. Backend
// reads do not evaluate feature flags.
func checkRequestFeatureOverrides(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, path string) error {
	if request.Phase == "backend" {
		return nil
	}
	return checkFeatureOverrides(path, request.Features, func() (*config.TerragruntConfig, error) {
		// Read the declarations without the overrides, which are what is being checked.
		declarations := pctx.WithDecodeList(config.FeatureFlagsBlock)
		declarations.FeatureFlags = nil
		return config.PartialParseConfigFile(ctx, logger, environment, declarations, path, nil)
	})
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
	finalizeExecution(execution, path)
	reparse := func(apply *config.ParsingContext) (*config.TerragruntConfig, error) {
		parsed, err := config.ParseConfigFile(ctx, logger, environment, apply, path, nil)
		if err != nil {
			return nil, err
		}
		// Apply-time values can differ from the ones planned, so register them before any event is returned.
		sink := logSinkFrom(ctx)
		redactInputs(sink, parsed.Inputs, SensitiveInputs(environment.FS, append(apply.FilesRead.Paths(), path), parsed.Inputs))
		redactInputs(sink, parsed.Locals, sensitiveLocals(environment.FS, path, parsed.Locals))
		return parsed, nil
	}
	return applyTimeExecution(request, path, execution, environment.FS, pctx.FilesRead.Paths(), reparse)
}

func parseUnit(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, path string) (*config.TerragruntConfig, error) {
	if request.Phase == "backend" {
		cfg, err := config.PartialParseConfigFile(ctx, logger, environment, pctx.WithDecodeList(config.TerraformSource, config.RemoteStateBlock), path, nil)
		if err != nil {
			return nil, err
		}
		// Fail closed on unverifiable backend generation before the download_dir probe, which
		// parses the whole unit and would evaluate inputs, hooks and commands.
		if err := checkBackendGeneration(environment.FS, append(pctx.FilesRead.Paths(), path)); err != nil {
			return nil, err
		}
		return withDownloadDir(ctx, logger, environment, pctx, request, cfg, path)
	}
	return config.ParseConfigFile(ctx, logger, environment, pctx, path, nil)
}

// withDownloadDir restores download_dir on a partially parsed unit. Partial
// decoding has no section for that attribute, but the working directory (and so
// every relative local-state path) depends on it. The extra parse skips
// dependency output resolution (an unknown placeholder replaces every
// dependency), so backend reads still never read outputs.
func withDownloadDir(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, cfg *config.TerragruntConfig, path string) (*config.TerragruntConfig, error) {
	if request.Environment["TG_DOWNLOAD_DIR"] != "" || !mentionsDownloadDir(environment.FS, append(pctx.FilesRead.Paths(), path)) {
		return cfg, nil
	}
	placeholder := cty.DynamicVal
	probe := pctx.WithSkipOutputsResolution()
	probe.DecodedDependencies = &placeholder
	full, err := config.ParseConfigFile(ctx, logger, environment, probe, path, nil)
	if err != nil {
		return nil, err
	}
	cfg.DownloadDir = full.DownloadDir
	return cfg, nil
}

// mentionsDownloadDir reports whether any parsed configuration file names download_dir. The probe
// in withDownloadDir parses the whole unit, so a unit that never sets it skips the probe and its
// inputs stay unevaluated during backend reads.
func mentionsDownloadDir(filesystem afero.Fs, paths []string) bool {
	for _, path := range paths {
		if content, err := afero.ReadFile(filesystem, path); err == nil && bytes.Contains(content, []byte("download_dir")) {
			return true
		}
	}
	return false
}

// unitResolution converts the parsed unit into its module identity. Local sources
// must be existing directories; remote sources keep the identity they were written with.
func unitResolution(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, cfg *config.TerragruntConfig, path string) (*model.Resolution, error) {
	result := &model.Resolution{Path: path, Inputs: cfg.Inputs, Source: *cfg.Terraform.Source, Generated: map[string]string{}, Files: append(pctx.FilesRead.Paths(), path)}
	if err := unitSettings(ctx, cfg, request, path, result); err != nil {
		return nil, err
	}
	// A remote source keeps the identity it was written with, exactly as Terragrunt names its working
	// copy; resolving a registry source below only changes where the module is fetched from.
	identity := result.Source
	if resolved, root, local, problem := resolveLocalModuleSource(environment.FS, path, result.Source); problem != nil {
		return nil, problem
	} else if local {
		result.Source = resolved
		identity, result.Folders = result.Source, []string{root}
	} else if !PreventsRun(result.Exclude, OperationCommand(request.Command, request.Arguments)) {
		// An operation the exclude block prevents never runs, so its registry source is not resolved.
		if err := prepareRemoteSource(ctx, logger, &request, cfg, result); err != nil {
			return nil, err
		}
	}
	download := pctx.DownloadDir
	if cfg.DownloadDir != "" && request.Environment["TG_DOWNLOAD_DIR"] == "" {
		download = localPath(filepath.Dir(path), cfg.DownloadDir)
	}
	if configured := request.Environment["TG_DOWNLOAD_DIR"]; configured != "" {
		download = localPath(filepath.Dir(path), configured)
	}
	workingDir, err := atmosbridge.WorkingDir(logger, environment, identity, download, filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	result.WorkingDir = workingDir
	result.Files = append(result.Files, request.OriginFiles...)
	result.Folders = append(result.Folders, request.OriginFolders...)
	return result, nil
}

// unitSettings applies the parity and IAM settings of a fully evaluated unit. Backend reads
// evaluate only the backend identity, so they skip them.
func unitSettings(ctx context.Context, cfg *config.TerragruntConfig, request model.Request, path string, result *model.Resolution) error {
	if request.Phase == "backend" {
		return nil
	}
	if err := applyUnitSettings(ctx, cfg, request, path, result); err != nil {
		return err
	}
	iam, warnings, err := iamSettings(cfg, &request, path)
	if err != nil {
		return err
	}
	result.IAM, result.Warnings = iam, append(result.Warnings, warnings...)
	return nil
}
