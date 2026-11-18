// Package engine evaluates Terragrunt configuration outside Atmos core.
package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/spf13/afero"
	"github.com/zclconf/go-cty/cty"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// Resolve evaluates HCL using the pinned Terragrunt parser.
//
// The public value API isolates invocation fields; this SDK-only plugin has no host tracker dependency.
//
//nolint:gocritic,lintroller // Preserve the public value API and standalone plugin boundary.
func Resolve(ctx context.Context, request model.Request) (*model.Resolution, error) {
	return resolve(ctx, &request, nil)
}

// ErrPreventDestroy rejects a destroy operation on a unit that sets prevent_destroy.
var ErrPreventDestroy = errors.New("unit is protected by prevent_destroy")

// resolve preserves the structured L6 error boundary around invocation-local evaluation.
func resolve(ctx context.Context, request *model.Request, filesystem afero.Fs) (*model.Resolution, error) {
	result, err := resolveRequest(ctx, request, filesystem)
	if err != nil {
		return nil, problemFor(err, request.Path)
	}
	return result, nil
}

func resolveRequest(ctx context.Context, request *model.Request, filesystem afero.Fs) (*model.Resolution, error) {
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
	evaluator := unitEvaluator{logger: logger, environment: environment, pctx: requestParsingContext(request, path), request: request, path: path}
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
	if err := e.checkDestroyGuard(ctx); err != nil {
		return nil, err
	}
	cfg, err := e.parseUnit(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkUnitPolicies(ctx, e.request, cfg, e.path); err != nil {
		return nil, err
	}
	execution, applyFiles, err := e.unitExecution(ctx, cfg)
	if err != nil {
		return nil, err
	}
	result, err := e.unitResolution(cfg)
	if err != nil {
		return nil, err
	}
	result.Execution = execution
	result.Files = append(result.Files, applyFiles...)
	if err := resolveBackend(cfg, result); err != nil {
		return nil, err
	}
	if e.request.Phase != phaseBackend {
		result.Overlay, result.OverlayModes, err = atmosbridge.UnitOverlay(e.logger, e.environment, filepath.Dir(e.path), cfg.Terraform)
		if err != nil {
			return nil, err
		}
	}
	if err := generatedFiles(ctx, e.logger, cfg, result); err != nil {
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
// Like upstream Terragrunt, a symlinked terragrunt.hcl stays a unit where the link lives.
func requestPath(path string) (string, error) {
	path, err := filepath.Abs(strings.TrimPrefix(path, "terragrunt://"))
	if err != nil {
		return "", err
	}
	return logicalPath(path)
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

// requestParsingContext evaluates the unit at path as the request's command;
// discovery and generation evaluate as plan.
func requestParsingContext(request *model.Request, path string) *config.ParsingContext {
	command := request.Command
	if command == "" || command == phaseDiscover || command == "generate" {
		command = "plan"
	}
	pctx := parsingContext(path, request.Tofu, command, request.Arguments)
	if configured := request.Environment[downloadDirEnv]; configured != "" {
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
func (e *unitEvaluator) checkDestroyGuard(ctx context.Context) error {
	if e.request.Phase == phaseBackend || (e.request.Command != "plan" && e.request.Command != "apply" && e.request.Command != "destroy") {
		return nil
	}
	guard, err := config.PartialParseConfigFile(ctx, e.logger, atmosbridge.Discovery(e.environment, false),
		e.pctx.WithDecodeList(config.TerragruntFlags, config.TerraformExtraArgs), e.path, nil)
	if err != nil {
		return failedDestroyGuard(e.request, e.environment.FS, e.path, err)
	}
	return checkPreventDestroy(e.request, guard)
}

// failedDestroyGuard preserves syntax-only decisions for known destroy requests.
func failedDestroyGuard(request *model.Request, fs afero.Fs, path string, parseErr error) error {
	args, err := effectivePolicyArguments(request, &config.TerragruntConfig{})
	if err != nil {
		return err
	}
	if atmosbridge.IsDestroyOperation(request.Command, args) {
		return staticDestroyGuard(fs, path, parseErr)
	}
	// A full parse may evaluate locals unavailable to the read-only guard.
	// Its policy check still inspects every expanded argument and environment.
	return nil
}

// staticDestroyGuard decides prevent_destroy from the unit's own syntax when the evaluated guard
// parse failed, for example because a local calls run_cmd and guard parsing never executes
// commands. It evaluates no function and reads no other file. A literal prevent_destroy decides the
// verdict, and a unit that sets none without an include block is unprotected, so the destroy
// continues to the ordinary parse. Anything the syntax cannot settle (an expression, an include, an
// unreadable or invalid file) keeps the original error, so the guard still fails closed.
func staticDestroyGuard(fs afero.Fs, path string, parseErr error) error {
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		return parseErr
	}
	file, diagnostics := hclsyntax.ParseConfig(data, path, hcl.InitialPos)
	body, ok := file.Body.(*hclsyntax.Body)
	if diagnostics.HasErrors() || !ok {
		return parseErr
	}
	for _, block := range body.Blocks {
		if block.Type == "include" {
			return parseErr
		}
	}
	return literalPreventDestroy(body, parseErr)
}

// literalPreventDestroy fails closed when the attribute cannot be decided without evaluation.
func literalPreventDestroy(body *hclsyntax.Body, parseErr error) error {
	attribute, found := body.Attributes["prevent_destroy"]
	if !found {
		return nil
	}
	value, diagnostics := attribute.Expr.Value(nil)
	if diagnostics.HasErrors() || value.IsNull() || !value.IsKnown() || value.Type() != cty.Bool {
		return parseErr
	}
	if value.True() {
		return ErrPreventDestroy
	}
	return nil
}

// unitExecution returns hooks and extra arguments as data for the host to run;
// evaluation never executes them. Backend identity is read from a partial parse
// that does not decode them.
func (e *unitEvaluator) unitExecution(ctx context.Context, cfg *config.TerragruntConfig) (*model.Execution, []string, error) {
	if e.request.Phase == phaseBackend {
		return nil, nil, nil
	}
	execution, err := executionFromConfig(cfg.Terraform)
	if err != nil {
		return nil, nil, err
	}
	reparse := func(apply *config.ParsingContext) (*config.TerragruntConfig, error) {
		return config.ParseConfigFile(ctx, e.logger, e.environment, apply, e.path, nil)
	}
	return e.applyTimeExecution(execution, reparse)
}

func (e *unitEvaluator) parseUnit(ctx context.Context) (*config.TerragruntConfig, error) {
	environment := e.environment
	if e.request.Phase == phaseBackend {
		environment = environment.WithFS(withoutInputsFS{Fs: environment.FS, inputs: newInputReferences(e.path, environment.FS)})
		cfg, err := config.PartialParseConfigFile(ctx, e.logger, environment, e.pctx.WithDecodeList(config.TerraformSource, config.RemoteStateBlock), e.path, nil)
		if err != nil {
			return nil, err
		}
		// Reject unverifiable backend generation before the full download_dir probe.
		if err := checkBackendGeneration(environment.FS, backendConfigurationPaths(cfg, e.path)); err != nil {
			return nil, err
		}
		if e.request.Environment[downloadDirEnv] == "" && mentionsDownloadDir(environment.FS, backendConfigurationPaths(cfg, e.path)) {
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

// mentionsDownloadDir reports whether any parsed configuration file names download_dir. The probe
// in downloadDir parses the whole unit, so a unit that never sets it skips the probe and its
// inputs stay unevaluated during backend reads.
func mentionsDownloadDir(filesystem afero.Fs, paths []string) bool {
	for _, path := range paths {
		data, err := afero.ReadFile(filesystem, path)
		if err != nil {
			continue
		}
		parser := hclparse.NewParser()
		var file *hcl.File
		var diagnostics hcl.Diagnostics
		if strings.HasSuffix(path, ".json") {
			file, diagnostics = parser.ParseJSON(data, path)
		} else {
			file, diagnostics = parser.ParseHCL(data, path)
		}
		if diagnostics.HasErrors() {
			continue
		}
		content, _, _ := file.Body.PartialContent(&hcl.BodySchema{Attributes: []hcl.AttributeSchema{{Name: "download_dir"}}})
		if content.Attributes["download_dir"] != nil {
			return true
		}
	}
	return false
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

// unitResolution preserves local directory identities and remote getter URIs.
func (e *unitEvaluator) unitResolution(cfg *config.TerragruntConfig) (*model.Resolution, error) {
	result := &model.Resolution{Path: e.path, Inputs: cfg.Inputs, Source: *cfg.Terraform.Source, Generated: map[string]string{}, Files: append(e.pctx.FilesRead.Paths(), e.path)}
	if plainLocalSource(result.Source) {
		result.Source = localPath(filepath.Dir(e.path), result.Source)
		if problem := localSourceProblem(e.environment.FS, e.path, *cfg.Terraform.Source, result.Source); problem != nil {
			return nil, problem
		}
		result.Folders = []string{result.Source}
	} else if err := validateRemoteSource(result.Source); err != nil {
		return nil, err
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
	result.Files = append(result.Files, e.request.OriginFiles...)
	result.Folders = append(result.Folders, e.request.OriginFolders...)
	return result, nil
}
