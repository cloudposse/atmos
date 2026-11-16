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
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/spf13/afero"
	"github.com/zclconf/go-cty/cty"

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
	pctx := parsingContext(request, path)
	if filepath.Base(path) == stackFile {
		return resolveStack(ctx, logger, environment, pctx, request, path)
	}
	if discovering(request) {
		return resolveUnitInventory(ctx, logger, environment, pctx, request, path)
	}
	cfg, err := parseUnit(ctx, logger, environment, pctx, request, path)
	if err != nil {
		return nil, err
	}
	if err := checkUnitPolicies(ctx, request, cfg); err != nil {
		return nil, err
	}
	result, err := unitResolution(logger, environment, pctx, request, cfg, path)
	if err != nil {
		return nil, err
	}
	if err := resolveBackend(result, cfg); err != nil {
		return nil, err
	}
	if request.Phase != "backend" {
		result.Overlay, result.OverlayModes, err = atmosbridge.UnitOverlay(logger, environment, filepath.Dir(path), cfg.Terraform)
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

func parsingContext(request model.Request, path string) *config.ParsingContext {
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
	if configured := request.Environment["TG_DOWNLOAD_DIR"]; configured != "" {
		pctx.DownloadDir = localPath(filepath.Dir(path), configured)
	}
	pctx.AutoInit = true
	pctx.MaxFoldersToCheck = options.DefaultMaxFoldersToCheck
	return pctx
}

func parseUnit(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, path string) (*config.TerragruntConfig, error) {
	if request.Phase == "backend" {
		cfg, err := config.PartialParseConfigFile(ctx, logger, environment, pctx.WithDecodeList(config.TerraformSource, config.RemoteStateBlock), path, nil)
		if err != nil {
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
// dependency), so backend reads still never read outputs, and it reads every
// configuration file without its top-level inputs attribute so unrelated input
// expressions, such as command calls, are never evaluated.
func withDownloadDir(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, cfg *config.TerragruntConfig, path string) (*config.TerragruntConfig, error) {
	if request.Environment["TG_DOWNLOAD_DIR"] != "" {
		return cfg, nil
	}
	placeholder := cty.DynamicVal
	probe := pctx.WithSkipOutputsResolution()
	probe.DecodedDependencies = &placeholder
	// Fresh caches keep the stripped files apart from the partial parse's cached ones.
	full, err := config.ParseConfigFile(config.WithCaches(ctx), logger, environment.WithFS(withoutInputsFS{Fs: environment.FS}), probe, path, nil)
	if err != nil {
		return nil, err
	}
	cfg.DownloadDir = full.DownloadDir
	return cfg, nil
}

// withoutInputsFS serves HCL configuration files without their top-level inputs
// attribute. Reading download_dir needs the locals, includes and functions that
// attribute might use, but never the inputs themselves.
type withoutInputsFS struct {
	afero.Fs
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
	if info, err := file.Stat(); err != nil || info.IsDir() {
		return file, nil
	}
	data, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil {
		return nil, err
	}
	memory := afero.NewMemMapFs()
	if err := afero.WriteFile(memory, name, withoutInputs(name, data), 0o644); err != nil {
		return nil, err
	}
	return memory.Open(name)
}

// withoutInputs removes the top-level inputs attribute. Content that does not
// parse is returned unchanged so upstream reports its own syntax errors.
func withoutInputs(name string, data []byte) []byte {
	file, diags := hclwrite.ParseConfig(data, name, hcl.InitialPos)
	if diags.HasErrors() {
		return data
	}
	file.Body().RemoveAttribute("inputs")
	return file.Bytes()
}

// unitResolution converts the parsed unit into its local module identity.
func unitResolution(logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext, request model.Request, cfg *config.TerragruntConfig, path string) (*model.Resolution, error) {
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
	result.Folders = []string{result.Source}
	result.Files = append(result.Files, request.OriginFiles...)
	result.Folders = append(result.Folders, request.OriginFolders...)
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
		return fmt.Errorf("native adapter only adapts the local backend")
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
			return fmt.Errorf("unsupported generated path: %s", generated.Path)
		}
		if generated.IfExistsStr != "overwrite" || !generated.DisableSignature {
			return fmt.Errorf("native adapter generation requires overwrite and disable_signature")
		}
		result.Generated[generated.Path] = generated.Contents
	}
	return nil
}
