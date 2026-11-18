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
	"github.com/zclconf/go-cty/cty"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

var (
	errPlainLocalSource = errors.New("native adapter only adapts plain local directory sources")
	errSourceDirectory  = errors.New("native adapter source must be an existing local directory")
	errLocalBackend     = errors.New("native adapter only adapts the local backend")
	errGeneratedPath    = errors.New("unsupported generated path")
	errGenerationPolicy = errors.New("native adapter generation requires overwrite and disable_signature")
)

// Resolve evaluates HCL using the pinned Terragrunt parser.
//
// The public value API isolates invocation fields; this SDK-only plugin has no host tracker dependency.
//
//nolint:gocritic,lintroller // Preserve the public value API and standalone plugin boundary.
func Resolve(ctx context.Context, request model.Request) (*model.Resolution, error) {
	return resolve(ctx, &request, nil)
}

func resolve(ctx context.Context, request *model.Request, filesystem afero.Fs) (*model.Resolution, error) {
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
	evaluator := unitEvaluator{logger: logger, environment: environment, pctx: parsingContext(request, path), request: request, path: path}
	return evaluator.resolve(ctx)
}

// unitEvaluator groups the invocation-local parser dependencies without changing request fields.
type unitEvaluator struct {
	logger      log.Logger
	environment *atmosbridge.Venv
	pctx        *config.ParsingContext
	request     *model.Request
	path        string
}

func (e *unitEvaluator) resolve(ctx context.Context) (*model.Resolution, error) {
	if filepath.Base(e.path) == stackFile {
		return e.resolveStack(ctx)
	}
	if discovering(e.request) {
		return e.resolveUnitInventory(ctx)
	}
	return e.resolveUnit(ctx)
}

func (e *unitEvaluator) resolveUnit(ctx context.Context) (*model.Resolution, error) {
	cfg, err := e.parseUnit(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkUnitPolicies(ctx, e.request, cfg); err != nil {
		return nil, err
	}
	result, err := e.unitResolution(cfg)
	if err != nil {
		return nil, err
	}
	if err := resolveBackend(result, cfg); err != nil {
		return nil, err
	}
	if e.request.Phase != phaseBackend {
		result.Overlay, result.OverlayModes, err = atmosbridge.UnitOverlay(e.logger, e.environment, filepath.Dir(e.path), cfg.Terraform)
		if err != nil {
			return nil, err
		}
	}
	if err := convertGeneration(result, cfg); err != nil {
		return nil, err
	}
	return result, nil
}

const (
	downloadDirEnv = "TG_DOWNLOAD_DIR"
	phaseBackend   = "backend"
	phaseDiscover  = "discover"
	probeFileMode  = 0o644
	stackFile      = "terragrunt.stack.hcl"
	unitFile       = "terragrunt.hcl"
	// Nested-stack recursion for generation and discovery stops at maxStackDepth.
	maxStackDepth = 32
)

// backendGenerationOverlay generates the owning stack into an invocation-local
// copy-on-write overlay before a backend read of a stack-generated unit. The
// returned filesystem is the one the unit is then parsed from.
func backendGenerationOverlay(ctx context.Context, request *model.Request, filesystem afero.Fs) (afero.Fs, error) {
	if request.Phase != phaseBackend || request.GenerationRoot == "" {
		return filesystem, nil
	}
	filesystem = afero.NewCopyOnWriteFs(afero.NewReadOnlyFs(afero.NewOsFs()), afero.NewMemMapFs())
	generation := *request
	generation.Path, generation.Phase, generation.Command = request.GenerationRoot, phaseDiscover, "generate"
	if _, err := resolve(ctx, &generation, filesystem); err != nil {
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

func discovering(request *model.Request) bool {
	return request.Phase == phaseDiscover || request.Command == phaseDiscover
}

func generating(request *model.Request) bool {
	return request.Command == "generate" && request.Phase != phaseBackend
}

// evaluationEnvironment uses the local maintained facade, which exposes the
// pinned upstream virtual environment and scoped stack generation without
// modifying upstream source.
func evaluationEnvironment(request *model.Request, filesystem afero.Fs, path string) (*atmosbridge.Venv, error) {
	environment := atmosbridge.Environment(request.Environment, io.Discard, io.Discard)
	if filesystem != nil {
		environment = environment.WithFS(filesystem)
	}
	if _, err := environment.FS.Stat(path); err != nil {
		return nil, err
	}
	if discovering(request) || request.Phase == phaseBackend || generating(request) {
		environment = atmosbridge.Discovery(environment, generating(request))
	}
	return environment, nil
}

func parsingContext(request *model.Request, path string) *config.ParsingContext {
	command := request.Command
	if command == "" || command == phaseDiscover || command == "generate" {
		command = "plan"
	}
	pctx := config.NewParsingContext().WithFileReadTracking()
	pctx.WorkingDir, pctx.RootWorkingDir = filepath.Dir(path), filepath.Dir(path)
	pctx.TerragruntConfigPath, pctx.OriginalTerragruntConfigPath = path, path
	pctx.TFPath, pctx.TerraformCommand = request.Tofu, command
	pctx.OriginalTerraformCommand = command
	pctx.TerraformCliArgs.SetCommand(command).AppendArgument(request.Arguments...)
	pctx.DownloadDir = filepath.Join(filepath.Dir(path), ".terragrunt-cache")
	if configured := request.Environment[downloadDirEnv]; configured != "" {
		pctx.DownloadDir = localPath(filepath.Dir(path), configured)
	}
	pctx.AutoInit = true
	pctx.MaxFoldersToCheck = options.DefaultMaxFoldersToCheck
	return pctx
}

func (e *unitEvaluator) parseUnit(ctx context.Context) (*config.TerragruntConfig, error) {
	environment := e.environment
	if e.request.Phase == phaseBackend {
		environment = environment.WithFS(withoutInputsFS{Fs: environment.FS, inputs: newInputReferences(e.path, environment.FS)})
		cfg, err := config.PartialParseConfigFile(ctx, e.logger, environment, e.pctx.WithDecodeList(config.TerraformSource, config.RemoteStateBlock), e.path, nil)
		if err != nil {
			return nil, err
		}
		if e.request.Environment[downloadDirEnv] == "" {
			cfg.DownloadDir, err = downloadDir(ctx, e.logger, environment, e.pctx)
		}
		return cfg, err
	}
	return config.ParseConfigFile(ctx, e.logger, environment, e.pctx, e.path, nil)
}

// downloadDir restores download_dir on a partially parsed unit. Partial
// decoding has no section for that attribute, but the working directory (and so
// every relative local-state path) depends on it. The extra parse skips
// dependency output resolution (an unknown placeholder replaces every
// dependency), so backend reads still never read outputs, and it reads every
// configuration file with only the referenced inputs so unrelated input
// expressions, such as command calls, are never evaluated.
func downloadDir(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext) (string, error) {
	placeholder := cty.DynamicVal
	probe := pctx.WithSkipOutputsResolution()
	probe.DecodedDependencies = &placeholder
	// Fresh caches keep the stripped files apart from the partial parse's cached ones.
	full, err := config.ParseConfigFile(config.WithCaches(ctx), logger, environment, probe, pctx.TerragruntConfigPath, nil)
	if err != nil {
		return "", err
	}
	return full.DownloadDir, nil
}

// withoutInputsFS projects inputs to the keys referenced by the configuration.
// References are shared across reads so read_terragrunt_config can supply inputs
// needed by download_dir without evaluating unrelated input expressions.
type withoutInputsFS struct {
	afero.Fs
	inputs *inputReferences
}

// Open reads through OpenFile so every read path strips inputs.
//
//nolint:lintroller // Pure afero delegation in a standalone plugin; no host tracker dependency.
func (f withoutInputsFS) Open(name string) (afero.File, error) {
	return f.OpenFile(name, os.O_RDONLY, 0)
}

// OpenFile returns the stripped content for read-only opens of HCL files.
//
//nolint:lintroller // Standalone plugin code cannot import the host performance tracker.
func (f withoutInputsFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	file, err := f.Fs.OpenFile(name, flag, perm)
	if err != nil || flag != os.O_RDONLY || !strings.HasSuffix(name, ".hcl") {
		return file, err
	}
	return f.projectFile(file, name)
}

// projectFile closes failed reads and preserves directory handles for callers.
func (f withoutInputsFS) projectFile(file afero.File, name string) (afero.File, error) {
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if info.IsDir() {
		return file, nil
	}
	data, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil {
		return nil, err
	}
	projected, err := f.inputs.project(name, data)
	if err != nil {
		return nil, err
	}
	memory := afero.NewMemMapFs()
	if err := afero.WriteFile(memory, name, projected, probeFileMode); err != nil {
		return nil, err
	}
	return memory.Open(name)
}

// unitResolution converts the parsed unit into its local module identity.
func (e *unitEvaluator) unitResolution(cfg *config.TerragruntConfig) (*model.Resolution, error) {
	result := &model.Resolution{Path: e.path, Inputs: cfg.Inputs, Source: *cfg.Terraform.Source, Generated: map[string]string{}, Files: append(e.pctx.FilesRead.Paths(), e.path)}
	if !plainLocalSource(result.Source) {
		return nil, errPlainLocalSource
	}
	if !filepath.IsAbs(result.Source) {
		result.Source = filepath.Join(filepath.Dir(e.path), result.Source)
	}
	if info, err := os.Stat(result.Source); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", errSourceDirectory, result.Source)
	}
	download := e.pctx.DownloadDir
	if cfg.DownloadDir != "" && e.request.Environment[downloadDirEnv] == "" {
		download = localPath(filepath.Dir(e.path), cfg.DownloadDir)
	}
	if configured := e.request.Environment[downloadDirEnv]; configured != "" {
		download = localPath(filepath.Dir(e.path), configured)
	}
	workingDir, err := atmosbridge.WorkingDir(e.logger, e.environment, result.Source, download, filepath.Dir(e.path))
	if err != nil {
		return nil, err
	}
	result.WorkingDir = workingDir
	result.Folders = []string{result.Source}
	result.Files = append(result.Files, e.request.OriginFiles...)
	result.Folders = append(result.Folders, e.request.OriginFolders...)
	return result, nil
}

// resolveBackend adapts only the local backend. Local backend paths are relative
// to Terragrunt's working directory, including the separate non-default-workspace
// state directory.
func resolveBackend(result *model.Resolution, cfg *config.TerragruntConfig) error {
	if cfg.RemoteState == nil {
		return nil
	}
	result.BackendType = cfg.RemoteState.BackendName
	result.Backend = cfg.RemoteState.BackendConfig
	if result.BackendType != "local" {
		return errLocalBackend
	}
	for key, fallback := range map[string]string{"path": "terraform.tfstate", "workspace_dir": "terraform.tfstate.d"} {
		value, _ := result.Backend[key].(string)
		if value == "" {
			value = fallback
		}
		result.Backend[key] = localPath(result.WorkingDir, value)
	}
	return nil
}

// convertGeneration returns literal generated files that Atmos writes itself.
func convertGeneration(result *model.Resolution, cfg *config.TerragruntConfig) error {
	for _, generated := range cfg.GenerateConfigs {
		if generated.Disable {
			continue
		}
		if filepath.Base(generated.Path) != generated.Path || !strings.HasSuffix(generated.Path, ".tf") {
			return fmt.Errorf("%w: %s", errGeneratedPath, generated.Path)
		}
		if generated.IfExistsStr != "overwrite" || !generated.DisableSignature {
			return errGenerationPolicy
		}
		result.Generated[generated.Path] = generated.Contents
	}
	return nil
}
