// Package engine evaluates Terragrunt configuration outside Atmos core.
package engine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
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
	result, err := resolveRequest(ctx, &request, filesystem)
	if err != nil {
		return nil, problemFor(err, request.Path)
	}
	return result, nil
}

// resolveRequest evaluates one request: it generates any owning stack, parses the path, and
// dispatches to the stack, inventory, or unit evaluation.
func resolveRequest(ctx context.Context, request *model.Request, filesystem afero.Fs) (*model.Resolution, error) {
	if request.Phase == "inventory" {
		return discoverProject(ctx, request)
	}
	filesystem, err := backendGenerationOverlay(ctx, *request, filesystem)
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
	environment, err := evaluationEnvironment(*request, filesystem, path)
	if err != nil {
		return nil, err
	}
	reader := &inventoryReader{ctx: ctx, logger: logger, environment: environment}
	pctx := requestParsingContext(*request, path)
	if request.Phase == "iam" {
		return reader.resolveIAM(pctx, request, path)
	}
	if filepath.Base(path) == stackFile {
		return reader.resolveStack(pctx, request, path)
	}
	if err := reader.checkRequestFeatureOverrides(pctx, request, path); err != nil {
		return nil, err
	}
	if discovering(*request) {
		return reader.resolveUnitInventory(pctx, request, path)
	}
	return resolveUnit(ctx, logger, environment, pctx, *request, path)
}

// resolveUnit evaluates one unit: it guards destroy, parses the unit, applies
// policies, and assembles the resolution with its execution data, backend, overlay,
// and generated files.
func resolveUnit(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, path string) (*model.Resolution, error) {
	if err := checkDestroyGuard(ctx, logger, environment, pctx, request, path); err != nil {
		return nil, err
	}
	cfg, err := parseUnit(ctx, logger, environment, pctx, request, path)
	if err != nil {
		return nil, err
	}
	if err := checkUnitPolicies(ctx, &request, cfg, path); err != nil {
		return nil, err
	}
	return buildUnitResolution(ctx, logger, environment, pctx, request, cfg, path)
}

// buildUnitResolution assembles the resolution of a parsed unit. The execution is
// set before the backend resolves so the backend step sees the final result.
func buildUnitResolution(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, cfg *config.TerragruntConfig, path string) (*model.Resolution, error) {
	execution, applyFiles, err := unitExecution(ctx, logger, environment, pctx, request, cfg, path)
	if err != nil {
		return nil, err
	}
	result, err := (&inventoryReader{ctx: ctx, logger: logger, environment: environment}).unitResolution(pctx, &request, cfg, path)
	if err != nil {
		return nil, err
	}
	result.Execution = execution
	result.Files = append(result.Files, applyFiles...)
	if err := resolveBackend(cfg, result, &request); err != nil {
		return nil, err
	}
	if request.Phase != phaseBackend {
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
// copy-on-write overlay before a backend read or IAM probe of a stack-generated unit. The
// host probes IAM before the provider generates the stack, so on a clean checkout the unit
// exists only in this overlay. The returned filesystem is the one the unit is then parsed from.
func backendGenerationOverlay(ctx context.Context, request model.Request, filesystem afero.Fs) (afero.Fs, error) {
	if (request.Phase != phaseBackend && request.Phase != "iam") || request.GenerationRoot == "" {
		return filesystem, nil
	}
	filesystem = afero.NewCopyOnWriteFs(afero.NewReadOnlyFs(afero.NewOsFs()), afero.NewMemMapFs())
	generation := request
	generation.Path, generation.Phase, generation.Command = request.GenerationRoot, phaseDiscover, "generate"
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
	return request.Phase == phaseDiscover || request.Command == phaseDiscover
}

func generating(request model.Request) bool {
	return request.Command == "generate" && request.Phase != phaseBackend
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
	if discovering(request) || request.Phase == phaseBackend || generating(request) {
		environment = atmosbridge.Discovery(environment, generating(request))
	}
	return environment, nil
}

// requestParsingContext evaluates the unit at path as the request's command;
// discovery and generation evaluate as plan.
func requestParsingContext(request model.Request, path string) *config.ParsingContext {
	command := request.Command
	if command == "" || command == phaseDiscover || command == "generate" {
		command = "plan"
	}
	pctx := parsingContext(path, request.Tofu, command, request.Arguments)
	if configured := request.Environment[downloadDirEnv]; configured != "" {
		pctx.DownloadDir = localPath(filepath.Dir(path), configured)
	}
	// Explicit feature overrides apply to every unit, as Terragrunt's --feature flag does.
	pctx.FeatureFlags = request.Features
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
	if request.Phase == phaseBackend || (request.Command != "plan" && request.Command != "apply" && request.Command != "destroy") {
		return nil
	}
	guard, err := config.PartialParseConfigFile(ctx, logger, atmosbridge.Discovery(environment, false),
		pctx.WithDecodeList(config.TerragruntFlags, config.TerraformExtraArgs), path, nil)
	if err != nil {
		return failedDestroyGuard(&request, environment.FS, path, err)
	}
	return checkPreventDestroy(&request, guard)
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
func unitExecution(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, cfg *config.TerragruntConfig, path string) (*model.Execution, []string, error) {
	if request.Phase == phaseBackend {
		return nil, nil, nil
	}
	execution, err := executionFromConfig(cfg.Terraform)
	if err != nil {
		return nil, nil, err
	}
	finalizeExecution(execution, path)
	reparse := func(apply *config.ParsingContext) (*config.TerragruntConfig, error) {
		return config.ParseConfigFile(ctx, logger, environment, apply, path, nil)
	}
	return applyTimeExecution(&request, path, execution, executionReads{filesystem: environment.FS, paths: pctx.FilesRead.Paths()}, reparse)
}

func parseUnit(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, path string) (*config.TerragruntConfig, error) {
	if request.Phase == phaseBackend {
		environment = environment.WithFS(withoutInputsFS{Fs: environment.FS, inputs: newInputReferences(path)})
		cfg, err := config.PartialParseConfigFile(ctx, logger, environment, pctx.WithDecodeList(config.TerraformSource, config.RemoteStateBlock), path, nil)
		if err != nil {
			return nil, err
		}
		// Reject unverifiable backend generation before the full download_dir probe.
		if err := checkBackendGeneration(environment.FS, append(pctx.FilesRead.Paths(), path)); err != nil {
			return nil, err
		}
		if request.Environment[downloadDirEnv] == "" && mentionsDownloadDir(environment.FS, append(pctx.FilesRead.Paths(), path)) {
			cfg.DownloadDir, err = downloadDir(ctx, logger, environment, pctx)
		}
		return cfg, err
	}
	return config.ParseConfigFile(ctx, logger, environment, pctx, path, nil)
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
		if content, err := afero.ReadFile(filesystem, path); err == nil && bytes.Contains(content, []byte("download_dir")) {
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
func (f withoutInputsFS) Open(name string) (afero.File, error) {
	return f.OpenFile(name, os.O_RDONLY, 0)
}

// OpenFile returns the stripped content for read-only opens of HCL files.
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
	memory := afero.NewMemMapFs()
	if err := afero.WriteFile(memory, name, f.inputs.project(name, data), probeFileMode); err != nil {
		return nil, err
	}
	return memory.Open(name)
}

const inputsAttribute = "inputs"

// inputReferences tracks input keys used by the probe, including references in
// retained input values. A whole-map or dynamic reference requires all inputs.
// The parser may read included files concurrently.
type inputReferences struct {
	mu   sync.Mutex
	root string
	keys map[string]bool
	all  bool
}

func newInputReferences(root string) *inputReferences {
	return &inputReferences{root: root, keys: map[string]bool{}}
}

// project preserves syntax errors for the upstream parser to diagnose.
func (r *inputReferences) project(name string, data []byte) []byte {
	parsed, diags := hclsyntax.ParseConfig(data, name, hcl.InitialPos)
	if diags.HasErrors() {
		return data
	}
	body := parsed.Body.(*hclsyntax.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectBody(body)
	inputs := body.Attributes[inputsAttribute]
	if inputs == nil {
		return data
	}
	file, _ := hclwrite.ParseConfig(data, name, hcl.InitialPos)
	if name == r.root || (!r.all && len(r.keys) == 0) {
		file.Body().RemoveAttribute(inputsAttribute)
		return file.Bytes()
	}
	if r.all {
		return data
	}
	object, ok := inputs.Expr.(*hclsyntax.ObjectConsExpr)
	if !ok {
		// Computed maps cannot be safely pruned without evaluating their expression.
		r.collect(inputs.Expr)
		return data
	}
	r.expand(object)
	if r.all {
		return data
	}
	file.Body().SetAttributeRaw(inputsAttribute, r.objectTokens(object, data, name))
	return file.Bytes()
}

func (r *inputReferences) collectBody(body *hclsyntax.Body) {
	for name, attr := range body.Attributes {
		if name != inputsAttribute {
			r.collect(attr.Expr)
		}
	}
	for _, block := range body.Blocks {
		r.collect(block.Body)
	}
}

func (r *inputReferences) expand(object *hclsyntax.ObjectConsExpr) {
	// A selected value may reference another input key. Reach a fixed point before
	// pruning so source order does not determine which values survive.
	for {
		count := len(r.keys)
		for _, item := range object.Items {
			key, diags := item.KeyExpr.Value(nil)
			if diags.HasErrors() || !key.IsKnown() || key.IsNull() || key.Type() != cty.String {
				// Preserve computed keys rather than silently selecting a fallback path.
				r.all = true
				return
			}
			if r.keys[key.AsString()] {
				r.collect(item.ValueExpr)
			}
		}
		if r.all {
			return
		}
		if count == len(r.keys) {
			return
		}
	}
}

func (r *inputReferences) objectTokens(object *hclsyntax.ObjectConsExpr, data []byte, name string) hclwrite.Tokens {
	var attributes []hclwrite.ObjectAttrTokens
	for _, item := range object.Items {
		key, _ := item.KeyExpr.Value(nil)
		if !r.keys[key.AsString()] {
			continue
		}
		// Reparse the value as an attribute to preserve arbitrary HCL expressions.
		value := item.ValueExpr.Range().SliceBytes(data)
		expression, _ := hclwrite.ParseConfig(append(append([]byte("value = "), value...), '\n'), name, hcl.InitialPos)
		attributes = append(attributes, hclwrite.ObjectAttrTokens{
			Name:  hclwrite.TokensForValue(key),
			Value: expression.Body().GetAttribute("value").Expr().BuildTokens(nil),
		})
	}
	return hclwrite.TokensForObject(attributes)
}

func (r *inputReferences) collect(node hclsyntax.Node) {
	_ = hclsyntax.VisitAll(node, func(node hclsyntax.Node) hcl.Diagnostics {
		switch expression := node.(type) {
		case *hclsyntax.ScopeTraversalExpr:
			r.traversal(expression.Traversal)
		case *hclsyntax.RelativeTraversalExpr:
			r.traversal(expression.Traversal)
		}
		return nil
	})
}

func (r *inputReferences) traversal(traversal hcl.Traversal) {
	for i, step := range traversal {
		name, _ := traversalName(step)
		if name != inputsAttribute {
			continue
		}
		if i+1 == len(traversal) {
			r.all = true
			return
		}
		key, known := traversalName(traversal[i+1])
		if known {
			r.keys[key] = true
		} else {
			r.all = true
		}
	}
}

func traversalName(step hcl.Traverser) (string, bool) {
	switch step := step.(type) {
	case hcl.TraverseRoot:
		return step.Name, true
	case hcl.TraverseAttr:
		return step.Name, true
	case hcl.TraverseIndex:
		if step.Key.IsKnown() && !step.Key.IsNull() && step.Key.Type() == cty.String {
			return step.Key.AsString(), true
		}
	}
	return "", false
}

// unitResolution converts the parsed unit into its module identity. Local sources
// must be existing directories; remote sources keep the identity they were written with.
func (r *inventoryReader) unitResolution(pctx *config.ParsingContext, request *model.Request, cfg *config.TerragruntConfig, path string) (*model.Resolution, error) {
	result := &model.Resolution{Path: path, Inputs: cfg.Inputs, Source: *cfg.Terraform.Source, Generated: map[string]string{}, Files: append(pctx.FilesRead.Paths(), path)}
	if err := unitSettings(r.ctx, cfg, request, path, result); err != nil {
		return nil, err
	}
	// A remote source keeps the identity it was written with, exactly as Terragrunt names its working
	// copy; resolving a registry source below only changes where the module is fetched from.
	identity := result.Source
	if local, problem := resolveLocalModuleSource(r.environment.FS, path, result.Source); problem != nil {
		return nil, problem
	} else if local != nil {
		result.Source = local.source
		identity, result.Folders = result.Source, []string{local.root}
	} else if !PreventsRun(result.Exclude, OperationCommand(request.Command, request.Arguments)) {
		// An operation the exclude block prevents never runs, so its registry source is not resolved.
		if err := prepareRemoteSource(r.ctx, r.logger, request, cfg, result); err != nil {
			return nil, err
		}
	}
	download := pctx.DownloadDir
	if cfg.DownloadDir != "" && request.Environment[downloadDirEnv] == "" {
		download = localPath(filepath.Dir(path), cfg.DownloadDir)
	}
	if configured := request.Environment[downloadDirEnv]; configured != "" {
		download = localPath(filepath.Dir(path), configured)
	}
	workingDir, err := atmosbridge.WorkingDir(r.logger, r.environment, identity, download, filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	result.WorkingDir = workingDir
	result.Files = append(result.Files, request.OriginFiles...)
	result.Folders = append(result.Folders, request.OriginFolders...)
	return result, nil
}
