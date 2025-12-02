# File Handler Registry: Extensible Multi-Format Configuration Support

## Executive Summary

This PRD outlines a comprehensive refactoring plan to make Atmos's file import and configuration handling extensible through a **File Handler Registry** pattern. The goal is to support multiple file formats (JSON, YAML, HCL, TypeScript, PKL, and future formats) while maintaining the ability to register Atmos YAML functions (like `!terraform.output`, `!store.get`) across all supported formats.

All formats will ultimately be translated into a common internal representation (`map[string]any`), making them interchangeable for stack configuration.

## Goals

1. **Format Extensibility**: Support multiple configuration formats (JSON, YAML, HCL, TypeScript, PKL) through a pluggable registry.
2. **Function Portability**: Register YAML functions (like `!terraform.output`) across all formats.
3. **Pure Functions**: Refactor into packages with pure, testable functions.
4. **High Test Coverage**: Target >90% unit test coverage for the new packages.
5. **Backward Compatibility**: Existing YAML-based configurations continue to work unchanged.

## Current Architecture Analysis

### Pain Points Identified

1. **Scattered File Handling**: File type handling is spread across:
   - `pkg/filetype/filetype.go` - Format detection and parsing
   - `pkg/filetype/filetype_by_extension.go` - Extension-based parsing
   - `pkg/utils/yaml_include_by_extension.go` - Include tag processing
   - `pkg/config/imports.go` - Import resolution
   - `pkg/config/process_yaml.go` - YAML function preprocessing

2. **YAML Function Fragmentation**: Functions processed in multiple places:
   - Pre-merge: `pkg/config/process_yaml.go` (direct node manipulation)
   - Post-merge: `internal/exec/yaml_func_*.go` (string parsing)
   - Different mechanisms for different stages

3. **Tight Coupling**: Current code assumes YAML everywhere:
   - `yaml.Node` used as the intermediate representation
   - YAML-specific tags (`!env`, `!include`) baked into the codebase
   - Viper configured only for YAML config type

4. **No Format Abstraction**: No common interface for file handlers.

### Current Flow

```
File → Read Bytes → Detect Format → Parse → YAML Node → Process Tags → Decode → map[string]any
```

The goal is to change this to:

```
File → Read Bytes → Registry Lookup → Handler.Parse() → map[string]any → Function Processing → Final Config
```

## Proposed Architecture

### Core Packages

```
pkg/
├── filehandler/                    # Core file handler infrastructure
│   ├── registry.go                 # FileHandlerRegistry implementation
│   ├── handler.go                  # FileHandler interface
│   ├── options.go                  # Functional options
│   ├── errors.go                   # Error definitions
│   └── registry_test.go
│
├── filehandler/yaml/               # YAML handler implementation
│   ├── handler.go                  # YAMLHandler struct
│   ├── parser.go                   # Pure parsing functions
│   ├── encoder.go                  # Pure encoding functions
│   └── handler_test.go
│
├── filehandler/json/               # JSON handler implementation
│   ├── handler.go                  # JSONHandler struct
│   ├── parser.go                   # Pure parsing functions
│   ├── encoder.go                  # Pure encoding functions
│   └── handler_test.go
│
├── filehandler/hcl/                # HCL handler implementation
│   ├── handler.go                  # HCLHandler struct
│   ├── parser.go                   # Pure parsing functions
│   ├── encoder.go                  # Pure encoding functions (HCL2 JSON)
│   └── handler_test.go
│
├── filehandler/typescript/         # TypeScript/Deno handler (future)
│   └── ...
│
├── filehandler/pkl/                # PKL handler (future)
│   └── ...
│
├── configfunc/                     # Configuration functions (YAML functions generalized)
│   ├── registry.go                 # FunctionRegistry implementation
│   ├── function.go                 # ConfigFunction interface
│   ├── context.go                  # FunctionContext for execution
│   ├── errors.go                   # Error definitions
│   └── registry_test.go
│
├── configfunc/builtin/             # Built-in functions
│   ├── env.go                      # !env function
│   ├── exec.go                     # !exec function
│   ├── include.go                  # !include function
│   ├── template.go                 # !template function
│   ├── terraform_output.go         # !terraform.output function
│   ├── terraform_state.go          # !terraform.state function
│   ├── store.go                    # !store.get function
│   └── git_root.go                 # !repo-root function
│
└── configfunc/processor/           # Function processing pipeline
    ├── processor.go                # Main processor implementation
    ├── walker.go                   # Tree walker for function detection
    └── processor_test.go
```

### Key Interfaces

#### FileHandler Interface

```go
// pkg/filehandler/handler.go

package filehandler

import "context"

// FileHandler defines the interface for handling configuration file formats.
// Each format (YAML, JSON, HCL, etc.) implements this interface.
type FileHandler interface {
    // Extensions returns the file extensions this handler supports.
    // Example: [".yaml", ".yml"] for YAML handler.
    Extensions() []string

    // MIMETypes returns the MIME types this handler supports.
    // Example: ["application/yaml", "text/yaml"] for YAML handler.
    MIMETypes() []string

    // Name returns a human-readable name for this handler.
    // Example: "YAML", "JSON", "HCL".
    Name() string

    // Parse reads raw bytes and converts them to the common representation.
    // The result is always map[string]any or []any for arrays.
    // This is a pure function with no side effects.
    Parse(ctx context.Context, data []byte, opts ...ParseOption) (any, error)

    // ParseWithMetadata parses and returns additional metadata (positions, comments).
    // Used when provenance tracking is enabled.
    ParseWithMetadata(ctx context.Context, data []byte, opts ...ParseOption) (any, *Metadata, error)

    // Encode converts the common representation back to this format.
    // Used for outputting configurations in specific formats.
    Encode(ctx context.Context, data any, opts ...EncodeOption) ([]byte, error)

    // SupportsComments returns whether this format supports inline comments.
    // Used for preserving comments during round-trips.
    SupportsComments() bool

    // SupportsFunctions returns whether this format supports Atmos functions.
    // YAML supports !tag syntax, JSON/HCL use string conventions.
    SupportsFunctions() bool
}

// Metadata contains additional information extracted during parsing.
type Metadata struct {
    // Positions maps paths (e.g., "vars.region") to source positions.
    Positions map[string]Position

    // Comments maps paths to associated comments.
    Comments map[string]string

    // SourceFile is the original file path, if available.
    SourceFile string
}

// Position represents a location in a source file.
type Position struct {
    Line   int
    Column int
    Offset int
}

// ParseOption configures parsing behavior.
type ParseOption func(*parseOptions)

// EncodeOption configures encoding behavior.
type EncodeOption func(*encodeOptions)
```

#### FileHandlerRegistry

```go
// pkg/filehandler/registry.go

package filehandler

import (
    "context"
    "sync"
)

// Registry manages file handler registration and lookup.
type Registry struct {
    mu              sync.RWMutex
    handlers        map[string]FileHandler  // extension -> handler
    mimeHandlers    map[string]FileHandler  // mime type -> handler
    defaultHandler  FileHandler
}

// NewRegistry creates a new file handler registry.
func NewRegistry(opts ...RegistryOption) *Registry

// Register adds a handler to the registry for its declared extensions.
func (r *Registry) Register(handler FileHandler) error

// GetByExtension returns the handler for a file extension.
func (r *Registry) GetByExtension(ext string) (FileHandler, bool)

// GetByMIMEType returns the handler for a MIME type.
func (r *Registry) GetByMIMEType(mimeType string) (FileHandler, bool)

// ParseFile parses a file using the appropriate handler based on extension.
func (r *Registry) ParseFile(ctx context.Context, path string, data []byte) (any, error)

// ParseFileWithMetadata parses with metadata using the appropriate handler.
func (r *Registry) ParseFileWithMetadata(ctx context.Context, path string, data []byte) (any, *Metadata, error)

// List returns all registered handlers.
func (r *Registry) List() []FileHandler

// DefaultRegistry returns the global default registry with all built-in handlers.
func DefaultRegistry() *Registry
```

#### ConfigFunction Interface

```go
// pkg/configfunc/function.go

package configfunc

import "context"

// ConfigFunction defines an Atmos configuration function.
// These are the generalized form of YAML functions (!env, !terraform.output, etc.).
type ConfigFunction interface {
    // Name returns the function name (e.g., "env", "terraform.output").
    Name() string

    // Aliases returns alternative names for this function.
    // Example: ["store"] as alias for "store.get".
    Aliases() []string

    // Execute runs the function with the given arguments.
    // The context contains stack information, resolution state, etc.
    Execute(ctx *FunctionContext, args string) (any, error)

    // Phase returns when this function should be processed.
    // PreMerge functions run during initial loading.
    // PostMerge functions run after configuration merging.
    Phase() ExecutionPhase

    // SupportsCycleDetection returns whether this function can participate
    // in dependency cycle detection (e.g., terraform.output).
    SupportsCycleDetection() bool
}

// ExecutionPhase determines when a function is processed.
type ExecutionPhase int

const (
    // PreMerge functions are processed during initial file loading.
    // Examples: !env, !exec, !include, !repo-root.
    PreMerge ExecutionPhase = iota

    // PostMerge functions are processed after all configuration merging.
    // Examples: !terraform.output, !terraform.state, !store.get, !template.
    PostMerge
)

// FunctionContext provides context for function execution.
type FunctionContext struct {
    // Context is the Go context for cancellation and deadlines.
    Context context.Context

    // AtmosConfig is the current Atmos configuration.
    AtmosConfig *schema.AtmosConfiguration

    // CurrentStack is the stack being processed.
    CurrentStack string

    // CurrentComponent is the component being processed.
    CurrentComponent string

    // SourceFile is the file containing this function invocation.
    SourceFile string

    // SourcePosition is the position in the source file.
    SourcePosition Position

    // ResolutionContext tracks dependencies for cycle detection.
    ResolutionContext *ResolutionContext

    // StackInfo contains additional stack processing information.
    StackInfo *schema.ConfigAndStacksInfo

    // Skip lists function names to skip during processing.
    Skip []string
}
```

#### FunctionRegistry

```go
// pkg/configfunc/registry.go

package configfunc

import "sync"

// FunctionRegistry manages configuration function registration.
type FunctionRegistry struct {
    mu        sync.RWMutex
    functions map[string]ConfigFunction  // name -> function
    phases    map[ExecutionPhase][]ConfigFunction
}

// NewFunctionRegistry creates a new function registry.
func NewFunctionRegistry() *FunctionRegistry

// Register adds a function to the registry.
func (r *FunctionRegistry) Register(fn ConfigFunction) error

// Get returns a function by name or alias.
func (r *FunctionRegistry) Get(name string) (ConfigFunction, bool)

// GetByPhase returns all functions for a given execution phase.
func (r *FunctionRegistry) GetByPhase(phase ExecutionPhase) []ConfigFunction

// List returns all registered functions.
func (r *FunctionRegistry) List() []ConfigFunction

// DefaultRegistry returns the global registry with all built-in functions.
func DefaultRegistry() *FunctionRegistry
```

### Function Processing Pipeline

```go
// pkg/configfunc/processor/processor.go

package processor

// Processor handles configuration function processing.
type Processor struct {
    registry      *configfunc.FunctionRegistry
    fileRegistry  *filehandler.Registry
    atmosConfig   *schema.AtmosConfiguration
}

// NewProcessor creates a new function processor.
func NewProcessor(opts ...Option) *Processor

// ProcessPreMerge processes pre-merge functions in a configuration.
// This is called during initial file loading.
func (p *Processor) ProcessPreMerge(ctx context.Context, data any, sourceFile string) (any, error)

// ProcessPostMerge processes post-merge functions in a configuration.
// This is called after configuration merging.
func (p *Processor) ProcessPostMerge(ctx *FunctionContext, data any) (any, error)

// Walk traverses data and identifies all function invocations.
func (p *Processor) Walk(data any) ([]*FunctionInvocation, error)

// FunctionInvocation represents a detected function call in configuration.
type FunctionInvocation struct {
    Name     string
    Args     string
    Path     []string  // Path to this value (e.g., ["vars", "region"])
    Position Position
    Phase    ExecutionPhase
}
```

### Format-Specific Function Syntax

Since different formats have different syntax capabilities, functions are represented differently:

#### YAML (Native Tag Support)
```yaml
vars:
  region: !env AWS_REGION
  vpc_id: !terraform.output vpc/vpc_id
  secret: !store.get ssm/db-password
```

#### JSON (String Convention)
```json
{
  "vars": {
    "region": "${env:AWS_REGION}",
    "vpc_id": "${terraform.output:vpc/vpc_id}",
    "secret": "${store.get:ssm/db-password}"
  }
}
```

#### HCL (Expression/Function Syntax)
```hcl
vars {
  region = atmos_env("AWS_REGION")
  vpc_id = atmos_terraform_output("vpc", "vpc_id")
  secret = atmos_store_get("ssm", "db-password")
}
```

Each handler is responsible for detecting its format's function syntax and normalizing it to a common representation for the processor.

## Implementation Plan

### Phase 1: Foundation (Core Infrastructure)

#### 1.1 Create pkg/filehandler Package

```go
// pkg/filehandler/handler.go - FileHandler interface
// pkg/filehandler/registry.go - Registry implementation
// pkg/filehandler/options.go - Functional options
// pkg/filehandler/errors.go - Error definitions
```

**Tasks:**
1. Define `FileHandler` interface with all methods.
2. Implement `Registry` with thread-safe registration.
3. Add functional options for parsing/encoding configuration.
4. Define static errors following Atmos error patterns.
5. Write comprehensive unit tests (>90% coverage).

#### 1.2 Create pkg/configfunc Package

```go
// pkg/configfunc/function.go - ConfigFunction interface
// pkg/configfunc/registry.go - FunctionRegistry implementation
// pkg/configfunc/context.go - FunctionContext struct
// pkg/configfunc/errors.go - Error definitions
```

**Tasks:**
1. Define `ConfigFunction` interface with phase support.
2. Implement `FunctionRegistry` with name/alias lookup.
3. Create `FunctionContext` with all execution context.
4. Define static errors.
5. Write comprehensive unit tests (>90% coverage).

### Phase 2: YAML Handler Migration

#### 2.1 Create pkg/filehandler/yaml Package

Migrate existing YAML functionality to the new architecture:

```go
// pkg/filehandler/yaml/handler.go
type YAMLHandler struct{}

func (h *YAMLHandler) Extensions() []string {
    return []string{".yaml", ".yml"}
}

func (h *YAMLHandler) Parse(ctx context.Context, data []byte, opts ...ParseOption) (any, error) {
    // Use pure parsing functions from parser.go
    return parser.Parse(data, opts...)
}
```

**Tasks:**
1. Extract pure parsing functions from `pkg/utils/yaml_utils.go`.
2. Implement `YAMLHandler` struct.
3. Migrate `processCustomTags` to use function registry.
4. Add YAML-specific options (indent, preserve comments).
5. Ensure backward compatibility with existing behavior.
6. Write unit tests for all functions.

#### 2.2 Migrate Existing YAML Functions to configfunc/builtin

```go
// pkg/configfunc/builtin/env.go
type EnvFunction struct{}

func (f *EnvFunction) Name() string { return "env" }
func (f *EnvFunction) Phase() ExecutionPhase { return PreMerge }

func (f *EnvFunction) Execute(ctx *FunctionContext, args string) (any, error) {
    // Migrate logic from pkg/utils/yaml_func_env.go
}
```

**Migrate these functions:**
- `!env` → `pkg/configfunc/builtin/env.go`
- `!exec` → `pkg/configfunc/builtin/exec.go`
- `!include` → `pkg/configfunc/builtin/include.go`
- `!include.raw` → `pkg/configfunc/builtin/include_raw.go`
- `!repo-root` → `pkg/configfunc/builtin/git_root.go`
- `!template` → `pkg/configfunc/builtin/template.go`
- `!terraform.output` → `pkg/configfunc/builtin/terraform_output.go`
- `!terraform.state` → `pkg/configfunc/builtin/terraform_state.go`
- `!store.get` → `pkg/configfunc/builtin/store.go`

### Phase 3: JSON Handler

#### 3.1 Create pkg/filehandler/json Package

```go
// pkg/filehandler/json/handler.go
type JSONHandler struct{}

func (h *JSONHandler) Extensions() []string {
    return []string{".json"}
}

func (h *JSONHandler) Parse(ctx context.Context, data []byte, opts ...ParseOption) (any, error) {
    return parser.Parse(data, opts...)
}
```

**Tasks:**
1. Implement JSON parsing with function detection.
2. Support `${function:args}` syntax for functions.
3. Handle JSON5 comments if enabled via options.
4. Write comprehensive unit tests.

#### 3.2 JSON Function Syntax Support

```go
// pkg/filehandler/json/functions.go

// DetectFunctions finds ${...} patterns in JSON strings.
func DetectFunctions(data any) ([]*FunctionInvocation, error)

// TransformFunctions converts ${...} patterns to standardized invocations.
func TransformFunctions(value string) *FunctionInvocation
```

### Phase 4: HCL Handler

#### 4.1 Create pkg/filehandler/hcl Package

```go
// pkg/filehandler/hcl/handler.go
type HCLHandler struct{}

func (h *HCLHandler) Extensions() []string {
    return []string{".hcl", ".tf", ".tfvars"}
}

func (h *HCLHandler) Parse(ctx context.Context, data []byte, opts ...ParseOption) (any, error) {
    // Use HCL2 parser, convert to map[string]any
    return parser.Parse(data, opts...)
}
```

**Tasks:**
1. Migrate existing HCL parsing from `pkg/filetype/filetype.go`.
2. Support HCL2 with proper block handling.
3. Implement `atmos_*` function detection in HCL expressions.
4. Handle Terraform-style variable interpolation.
5. Write comprehensive unit tests.

### Phase 5: Integration

#### 5.1 Update Import Processing

```go
// pkg/config/imports.go

func processConfigImportsWithFS(source *schema.AtmosConfiguration, dst *viper.Viper, fs filesystem.FileSystem) error {
    registry := filehandler.DefaultRegistry()
    processor := processor.NewProcessor(
        processor.WithFunctionRegistry(configfunc.DefaultRegistry()),
        processor.WithFileHandlerRegistry(registry),
        processor.WithAtmosConfig(source),
    )

    for _, resolvedPath := range resolvedPaths {
        data, err := fs.ReadFile(resolvedPath.filePath)
        if err != nil {
            continue
        }

        // Parse using appropriate handler
        parsed, err := registry.ParseFile(ctx, resolvedPath.filePath, data)
        if err != nil {
            continue
        }

        // Process pre-merge functions
        processed, err := processor.ProcessPreMerge(ctx, parsed, resolvedPath.filePath)
        if err != nil {
            continue
        }

        // Merge into destination
        mergeConfigData(processed, dst)
    }
    return nil
}
```

#### 5.2 Update Stack Processing

Update `internal/exec/stack_processor_*.go` to use the new infrastructure:

```go
// internal/exec/stack_processor_process_stacks.go

func ProcessStackConfig(atmosConfig *schema.AtmosConfiguration, ...) (map[string]any, error) {
    registry := filehandler.DefaultRegistry()
    funcRegistry := configfunc.DefaultRegistry()
    processor := processor.NewProcessor(...)

    // Parse stack manifest using registry
    handler, _ := registry.GetByExtension(filepath.Ext(stackManifestPath))
    data, err := handler.Parse(ctx, fileContent)

    // Process pre-merge functions
    data, err = processor.ProcessPreMerge(ctx, data, stackManifestPath)

    // ... merging logic ...

    // Process post-merge functions
    funcCtx := &FunctionContext{
        AtmosConfig:    atmosConfig,
        CurrentStack:   stackName,
        StackInfo:      stackInfo,
    }
    result, err = processor.ProcessPostMerge(funcCtx, mergedData)

    return result, nil
}
```

#### 5.3 Update YAML Processor Interface

Replace `merge.YAMLFunctionProcessor` with the new interface:

```go
// pkg/merge/yaml_processor.go - DEPRECATED, use pkg/configfunc

// For backward compatibility, wrap the new processor
type legacyProcessorAdapter struct {
    processor *processor.Processor
    funcCtx   *configfunc.FunctionContext
}

func (a *legacyProcessorAdapter) ProcessYAMLFunctionString(value string) (any, error) {
    // Detect function in string
    inv := detectFunctionInString(value)
    if inv == nil {
        return value, nil
    }

    // Get function from registry
    fn, ok := a.processor.FunctionRegistry().Get(inv.Name)
    if !ok {
        return value, nil
    }

    // Execute function
    return fn.Execute(a.funcCtx, inv.Args)
}
```

### Phase 6: Future Format Support

#### 6.1 TypeScript/Deno Handler (Future)

```go
// pkg/filehandler/typescript/handler.go

type TypeScriptHandler struct {
    denoPath string
}

func (h *TypeScriptHandler) Extensions() []string {
    return []string{".ts", ".tsx"}
}

func (h *TypeScriptHandler) Parse(ctx context.Context, data []byte, opts ...ParseOption) (any, error) {
    // Execute TypeScript file with Deno runtime
    // Expect default export of configuration object
    // Parse JSON output to map[string]any
}
```

#### 6.2 PKL Handler (Future)

```go
// pkg/filehandler/pkl/handler.go

type PKLHandler struct {
    pklPath string
}

func (h *PKLHandler) Extensions() []string {
    return []string{".pkl"}
}

func (h *PKLHandler) Parse(ctx context.Context, data []byte, opts ...ParseOption) (any, error) {
    // Execute PKL file with pkl CLI
    // Parse JSON output to map[string]any
}
```

## Migration Strategy

### Backward Compatibility

1. **Existing YAML configs continue to work unchanged.**
2. **New file handlers are opt-in** - users must explicitly use new formats.
3. **Function syntax is format-specific** but semantically equivalent.
4. **Gradual migration** - old code paths remain until fully deprecated.

### Deprecation Plan

1. **Phase 1**: New infrastructure alongside existing code.
2. **Phase 2**: Migrate existing code to use new infrastructure internally.
3. **Phase 3**: Deprecate old functions with warnings.
4. **Phase 4**: Remove deprecated code (major version bump).

## Testing Strategy

### Unit Test Requirements

Each package must have:
- **>90% code coverage** for new packages.
- **Table-driven tests** for parsing/encoding.
- **Mock-based tests** for external dependencies.
- **Error case coverage** for all error paths.

### Integration Tests

- **Format round-trip tests**: Parse → Encode → Parse produces same result.
- **Function equivalence tests**: Same function produces same result across formats.
- **Migration tests**: Existing configs produce identical output with new code.

### Benchmark Tests

- **Parse performance**: Compare new handlers vs old code.
- **Function execution**: Benchmark individual function performance.
- **Registry lookup**: Ensure O(1) handler lookup performance.

## Configuration Schema Updates

Update `pkg/datafetcher/schema/` to support new formats:

```yaml
# atmos.yaml
settings:
  file_handlers:
    enabled_extensions:
      - .yaml
      - .yml
      - .json
      - .hcl
    default_format: yaml
    json:
      allow_comments: false  # JSON5 comment support
      function_syntax: "${function:args}"
    hcl:
      function_prefix: "atmos_"
```

## Success Metrics

1. **Test Coverage**: >90% for all new packages.
2. **Performance**: No regression in parse time (±5%).
3. **Backward Compatibility**: All existing tests pass.
4. **Documentation**: All interfaces documented with examples.
5. **Format Support**: JSON and HCL handlers fully functional.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Breaking existing configs | High | Comprehensive integration tests, gradual rollout |
| Performance regression | Medium | Benchmark tests, lazy loading, caching |
| Complex format interactions | Medium | Clear format-specific documentation |
| Incomplete function support | Medium | Phased rollout, format capability flags |

## Timeline Estimate

- **Phase 1 (Foundation)**: 2-3 days
- **Phase 2 (YAML Migration)**: 3-4 days
- **Phase 3 (JSON Handler)**: 2-3 days
- **Phase 4 (HCL Handler)**: 2-3 days
- **Phase 5 (Integration)**: 3-4 days
- **Phase 6 (Future Formats)**: As needed

**Total**: ~2-3 weeks for core functionality.

## Appendix: Package Dependencies

```
pkg/filehandler/
├── handler.go          (no deps)
├── registry.go         (sync)
├── options.go          (no deps)
└── errors.go           (errors)

pkg/filehandler/yaml/
├── handler.go          (filehandler, yaml.v3)
├── parser.go           (yaml.v3)
└── encoder.go          (yaml.v3)

pkg/filehandler/json/
├── handler.go          (filehandler, encoding/json)
├── parser.go           (encoding/json)
└── encoder.go          (encoding/json)

pkg/filehandler/hcl/
├── handler.go          (filehandler, hcl/v2)
├── parser.go           (hcl/v2, go-cty)
└── encoder.go          (hcl/v2)

pkg/configfunc/
├── function.go         (context)
├── registry.go         (sync)
├── context.go          (schema)
└── errors.go           (errors)

pkg/configfunc/builtin/
├── env.go              (configfunc, os)
├── exec.go             (configfunc, os/exec)
├── include.go          (configfunc, filehandler)
├── template.go         (configfunc, text/template)
├── terraform_output.go (configfunc, internal/exec)
├── terraform_state.go  (configfunc, internal/exec)
└── store.go            (configfunc, pkg/store)

pkg/configfunc/processor/
├── processor.go        (configfunc, filehandler)
└── walker.go           (no deps)
```

## Appendix: Example Usage

### Registering a Custom Handler

```go
// Custom TOML handler example
type TOMLHandler struct{}

func (h *TOMLHandler) Extensions() []string { return []string{".toml"} }
func (h *TOMLHandler) Name() string { return "TOML" }
func (h *TOMLHandler) Parse(ctx context.Context, data []byte, opts ...ParseOption) (any, error) {
    var result map[string]any
    if err := toml.Unmarshal(data, &result); err != nil {
        return nil, err
    }
    return result, nil
}

// Register with registry
registry := filehandler.DefaultRegistry()
registry.Register(&TOMLHandler{})
```

### Registering a Custom Function

```go
// Custom function example
type VaultFunction struct{}

func (f *VaultFunction) Name() string { return "vault" }
func (f *VaultFunction) Phase() ExecutionPhase { return PostMerge }
func (f *VaultFunction) Execute(ctx *FunctionContext, args string) (any, error) {
    // Parse: "secret/data/myapp key"
    parts := strings.SplitN(args, " ", 2)
    path, key := parts[0], parts[1]

    // Fetch from Vault
    client := vault.NewClient(...)
    secret, err := client.Logical().Read(path)
    if err != nil {
        return nil, err
    }

    return secret.Data[key], nil
}

// Register with registry
funcRegistry := configfunc.DefaultRegistry()
funcRegistry.Register(&VaultFunction{})
```

### Using Different Formats

```yaml
# stacks/prod.yaml
import:
  - base.yaml
  - networking.json  # JSON import
  - security.hcl     # HCL import

vars:
  region: !env AWS_REGION
```

```json
// stacks/networking.json
{
  "vars": {
    "vpc_cidr": "10.0.0.0/16",
    "subnets": "${terraform.output:vpc/subnet_ids}"
  }
}
```

```hcl
# stacks/security.hcl
vars {
  admin_role = atmos_store_get("ssm", "/admin/role-arn")
  encryption_key = atmos_env("KMS_KEY_ID")
}
```

All three formats merge seamlessly into a single stack configuration.
