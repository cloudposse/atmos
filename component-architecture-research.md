# Atmos Component Architecture Research

## Executive Summary

This document provides a detailed analysis of the current component architecture in Atmos, the command registry pattern implementation, and recommendations for extracting a component interface pattern.

## 1. Current Component Architecture

### 1.1 Component Implementation Approach

**Components in Atmos are currently data-driven, not object-oriented.** They are represented as:

1. **Map-based configuration structures** (`map[string]any`) processed from YAML stack files
2. **Type differentiation by string constants** (`terraform`, `helmfile`, `packer`)
3. **Processing logic scattered across multiple packages** (`internal/exec/`, `pkg/component/`, `pkg/list/`, etc.)

**Key Types:**

```go
// pkg/schema/schema.go
type Components struct {
    Terraform Terraform `yaml:"terraform" json:"terraform"`
    Helmfile  Helmfile  `yaml:"helmfile" json:"helmfile"`
    Packer    Packer    `yaml:"packer" json:"packer"`
}

type BaseComponentConfig struct {
    BaseComponentVars                      AtmosSectionMapType
    BaseComponentSettings                  AtmosSectionMapType
    BaseComponentEnv                       AtmosSectionMapType
    BaseComponentAuth                      AtmosSectionMapType
    BaseComponentProviders                 AtmosSectionMapType
    BaseComponentHooks                     AtmosSectionMapType
    FinalBaseComponentName                 string
    BaseComponentCommand                   string
    BaseComponentBackendType               string
    BaseComponentBackendSection            AtmosSectionMapType
    // ... more fields
}

// Component types defined as constants
const (
    TerraformComponentType = "terraform"
    HelmfileComponentType  = "helmfile"
    PackerComponentType    = "packer"
)
```

### 1.2 Component Discovery & Loading

**File Locations:**
- Component configs: `components/{terraform|helmfile|packer}/`
- Stack configs: `stacks/` (YAML files defining component instances)

**Discovery Process:**

1. **Stack Processing** (`internal/exec/stack_processor_*.go`):
   - Loads YAML stack manifests from `stacks/` directory
   - Processes imports and inheritance hierarchically
   - Merges component configurations from multiple sources
   - Stores result in `ConfigAndStacksInfo.ComponentSection`

2. **Component Path Resolution** (`pkg/utils/component_path_utils.go`):
   ```go
   func GetComponentPath(
       atmosConfig *schema.AtmosConfiguration,
       componentType string,
       folderPrefix string,
       component string,
   ) (string, error)
   ```
   - Resolves physical path to component directory
   - Handles folder prefixes and naming patterns
   - Validates component existence

3. **Type Detection** (via try/catch pattern):
   ```go
   // internal/exec/describe_component.go lines 406-426
   // Try Terraform first
   result, err := tryProcessWithComponentType(&baseParams{componentType: "terraform"})
   if err != nil {
       // Try Helmfile
       result, err = tryProcessWithComponentType(&baseParams{componentType: "helmfile"})
       if err != nil {
           // Try Packer
           result, err = tryProcessWithComponentType(&baseParams{componentType: "packer"})
       }
   }
   ```

### 1.3 Component Execution Flow

**Terraform Execution** (`cmd/terraform_commands.go`, `internal/exec/terraform.go`):

```
1. User runs: atmos terraform plan vpc -s dev
   ↓
2. getTerraformCommands() provides command definitions
   ↓
3. ExecuteTerraformCmd() parses args/flags
   ↓
4. ProcessStacks() loads component config from stack
   ↓
5. Component validation (enabled, locked, abstract checks)
   ↓
6. Generate backend.tf, vars files
   ↓
7. ExecuteShellCommand() runs terraform binary
```

**Helmfile Execution** (`internal/exec/helmfile.go`):

```
1. User runs: atmos helmfile apply myapp -s dev
   ↓
2. ExecuteHelmfileCmd() parses args
   ↓
3. ProcessStacks() loads component config
   ↓
4. Component validation
   ↓
5. Generate varfile for helmfile
   ↓
6. ExecuteShellCommand() runs helmfile binary
```

**Common Pattern:**
- All component types follow similar execution flow
- Differences handled by type-specific functions (not polymorphism)
- Shared utilities in `internal/exec/` and `pkg/utils/`

### 1.4 Key Integration Points

**Components are accessed throughout the codebase:**

1. **CLI Commands** (`cmd/`):
   - `describe component` - Shows component configuration
   - `list components` - Lists all components
   - `validate component` - Validates component config
   - `terraform/helmfile/packer <subcommand>` - Executes component operations

2. **Stack Processing** (`internal/exec/stack_processor_*.go`):
   - Component inheritance processing
   - Component configuration merging
   - Backend configuration generation
   - Template rendering with component context

3. **Template Functions** (`internal/exec/template_funcs_component.go`):
   - `atmos.Component()` - Get component config in templates
   - `terraform.output()` - Access terraform outputs
   - `terraform.state()` - Query terraform state

4. **Affected Components** (`internal/exec/describe_affected.go`):
   - Git diff analysis
   - Component dependency tracking
   - Change impact determination

5. **Component Listing** (`pkg/list/list_components.go`):
   - Filter components by stack
   - Collect components across all stacks
   - Remove abstract/disabled components

## 2. Command Registry Pattern

### 2.1 Overview

**Location:** `cmd/internal/`

**Purpose:** Provide a modular, self-registering architecture for built-in commands while maintaining compatibility with custom commands from `atmos.yaml`.

**Key Files:**
- `cmd/internal/command.go` - CommandProvider interface
- `cmd/internal/registry.go` - Registry implementation
- `docs/prd/command-registry-pattern.md` - Complete design documentation
- `docs/developing-atmos-commands.md` - Developer guide

### 2.2 CommandProvider Interface

```go
// cmd/internal/command.go
type CommandProvider interface {
    // GetCommand returns the cobra.Command for this provider.
    // For commands with subcommands, return parent with subcommands attached.
    GetCommand() *cobra.Command

    // GetName returns unique command name (e.g., "about", "terraform").
    GetName() string

    // GetGroup returns command group for help organization.
    // Examples: "Core Stack Commands", "Stack Introspection"
    GetGroup() string
}
```

**Standard Command Groups:**
| Group | Commands |
|-------|----------|
| Core Stack Commands | terraform, helmfile, workflow, packer |
| Stack Introspection | describe, list, validate |
| Configuration Management | vendor, docs |
| Cloud Integration | aws, atlantis |
| Pro Features | auth, pro |
| Other Commands | about, completion, version, support |

### 2.3 Registry Implementation

**Key Features:**
- Thread-safe via `sync.RWMutex`
- Self-registration via `init()` functions
- Allows re-registration for testing/plugins
- Separate from custom command processing

**Core Functions:**
```go
// Register a command provider
func Register(provider CommandProvider)

// Register all providers with root command
func RegisterAll(root *cobra.Command) error

// Retrieve a provider by name (for testing/diagnostics)
func GetProvider(name string) (CommandProvider, bool)

// List all providers grouped by category
func ListProviders() map[string][]CommandProvider

// Count registered providers
func Count() int

// Reset registry (testing only)
func Reset()
```

### 2.4 Command Registration Flow

```
Application Start
├─ Package imports trigger init() functions
│  ├─ _ "github.com/cloudposse/atmos/cmd/about"
│  ├─ _ "github.com/cloudposse/atmos/cmd/describe"
│  └─ ... other command packages
│
├─ Each init() calls internal.Register()
│  └─ Providers stored in global registry
│
├─ cmd/root.go init() calls internal.RegisterAll()
│  └─ All registered commands added to RootCmd
│
└─ Execute() processes custom commands from atmos.yaml
   └─ Custom commands can extend/override built-in commands
```

### 2.5 Command Patterns in Registry

The registry supports four command patterns:

**Pattern 1: Simple Command**
- Standalone command with no subcommands
- Example: `about`, `version`
- Structure: `cmd/about/about.go`

**Pattern 2: Static Subcommands**
- Parent command + predefined subcommands
- Example: `describe` (component, stacks, affected)
- Structure:
  ```
  cmd/describe/
  ├── describe.go    # Parent + Provider
  ├── component.go   # Subcommand
  ├── stacks.go      # Subcommand
  └── affected.go    # Subcommand
  ```

**Pattern 3: Dynamic Subcommands**
- Parent + subcommands from arrays
- Example: `terraform` (plan, apply, destroy)
- Structure:
  ```
  cmd/terraform/
  ├── terraform.go   # Parent + Provider
  ├── commands.go    # Dynamic generator
  └── generate/      # Nested group
      ├── generate.go
      ├── backend.go
      └── varfile.go
  ```

**Pattern 4: Deeply Nested**
- Multiple nesting levels
- Example: `aws eks update-kubeconfig`
- Structure:
  ```
  cmd/aws/
  ├── aws.go         # Grandparent + Provider
  └── eks/
      ├── eks.go     # Parent
      └── update_kubeconfig.go
  ```

**Key Rule:** Only top-level commands register with registry. Subcommands are attached via `AddCommand()`.

## 3. Component Processing in Commands

### 3.1 `atmos describe component`

**Files:**
- `cmd/describe_component.go` - CLI command
- `internal/exec/describe_component.go` - Business logic
- `pkg/describe/describe_component.go` - Thin wrapper

**Processing Flow:**

```go
// cmd/describe_component.go:71
err = e.NewDescribeComponentExec().ExecuteDescribeComponentCmd(...)

// internal/exec/describe_component.go:429
func ExecuteDescribeComponentWithContext(...) (*DescribeComponentResult, error) {
    // Try each component type
    configAndStacksInfo, err = detectComponentType(atmosConfig, ...)

    // Get merge context for provenance tracking
    mergeContext = GetMergeContextForStack(configAndStacksInfo.StackFile)

    // Apply filtering
    filteredComponentSection = FilterEmptySections(...)

    return &DescribeComponentResult{
        ComponentSection: filteredComponentSection,
        MergeContext:     mergeContext,
        StackFile:        configAndStacksInfo.StackFile,
    }
}
```

**Key Insight:** Component type is detected by trying each type until one succeeds. No interface polymorphism.

### 3.2 `atmos list components`

**Files:**
- `cmd/list_components.go` - CLI command
- `pkg/list/list_components.go` - Business logic

**Processing:**

```go
// pkg/list/list_components.go:78
func FilterAndListComponents(stackFlag string, stacksMap map[string]any) ([]string, error) {
    // Extract components from stack map
    componentsMap := stackMap["components"].(map[string]any)

    // Iterate through component types
    if terraformComponents, ok := componentsMap["terraform"].(map[string]any); ok {
        allComponents = append(allComponents, lo.Keys(terraformComponents)...)
    }
    if helmfileComponents, ok := componentsMap["helmfile"].(map[string]any); ok {
        allComponents = append(allComponents, lo.Keys(helmfileComponents)...)
    }
    if packerComponents, ok := componentsMap["packer"].(map[string]any); ok {
        allComponents = append(allComponents, lo.Keys(packerComponents)...)
    }

    return lo.Uniq(allComponents), nil
}
```

**Key Insight:** Hardcoded type checking (`terraform`, `helmfile`, `packer`). Adding new types requires code changes.

### 3.3 `atmos describe affected`

**Files:**
- `cmd/describe_affected.go` - CLI command
- `internal/exec/describe_affected.go` - Business logic

**Component Processing:**

```go
// internal/exec/describe_affected.go
// Analyzes git changes to determine affected components
// Component type stored in Affected struct:
type Affected struct {
    Component      string  `yaml:"component"`
    ComponentType  string  `yaml:"component_type"`  // "terraform", "helmfile", "packer"
    ComponentPath  string  `yaml:"component_path"`
    Stack          string  `yaml:"stack"`
    // ...
}
```

**Key Insight:** Component type is a string field, not a type-safe abstraction.

### 3.4 Component Execution Commands

**Terraform** (`cmd/terraform.go`, `cmd/terraform_commands.go`):

```go
// Dynamic commands created from array
func getTerraformCommands() []*cobra.Command {
    return []*cobra.Command{
        {Use: "plan", Short: "Show changes required"},
        {Use: "apply", Short: "Apply changes"},
        {Use: "destroy", Short: "Destroy infrastructure"},
        // ... 20+ more commands
    }
}

// Common execution path
func ExecuteTerraformCmd(cmd *cobra.Command, args []string, ...) error {
    info, err := ProcessCommandLineArgs("terraform", cmd, args, ...)
    return ExecuteTerraform(info)  // Type-specific function
}
```

**Helmfile** (`cmd/helmfile.go`, `internal/exec/helmfile.go`):

```go
func ExecuteHelmfileCmd(cmd *cobra.Command, args []string, ...) error {
    info, err := ProcessCommandLineArgs("helmfile", cmd, args, ...)
    return ExecuteHelmfile(info)  // Type-specific function
}
```

**Packer** - Similar pattern to above.

**Key Insight:** Each component type has its own execution function. No shared interface.

## 4. Potential Component Interface

### 4.1 What Could Be Extracted?

Based on the analysis, here are the common operations that could form a `ComponentProvider` interface:

**Option A: Execution-Focused Interface**
```go
type ComponentProvider interface {
    // GetName returns the component type name ("terraform", "helmfile", "packer")
    GetName() string

    // GetBasePath returns the base directory for this component type
    GetBasePath(atmosConfig *schema.AtmosConfiguration) string

    // ValidateConfig validates component configuration
    ValidateConfig(config map[string]any) error

    // Execute runs a command for this component
    Execute(cmd *cobra.Command, args []string, info schema.ConfigAndStacksInfo) error

    // GenerateFiles generates necessary files (backend.tf, varfiles, etc.)
    GenerateFiles(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo) error

    // GetCommands returns available commands for this component type
    GetCommands() []*cobra.Command
}
```

**Option B: Discovery-Focused Interface**
```go
type ComponentProvider interface {
    // GetType returns the component type string
    GetType() string

    // Discover finds all components of this type in the filesystem
    Discover(atmosConfig *schema.AtmosConfiguration) ([]string, error)

    // LoadConfig loads component configuration from stack
    LoadConfig(atmosConfig *schema.AtmosConfiguration, component string, stack string) (map[string]any, error)

    // IsValid checks if a path contains a valid component of this type
    IsValid(path string) bool
}
```

**Option C: Hybrid Interface (Recommended)**
```go
type ComponentProvider interface {
    // Core identity
    GetType() string  // "terraform", "helmfile", "packer"
    GetGroup() string // "Infrastructure", "Kubernetes", "Images"

    // Configuration
    GetBasePath(atmosConfig *schema.AtmosConfiguration) string
    GetCommand(atmosConfig *schema.AtmosConfiguration) string

    // Validation
    ValidateComponent(config map[string]any) error
    CheckLocked(metadata map[string]any) bool
    CheckAbstract(metadata map[string]any) bool
    CheckEnabled(metadata map[string]any) bool

    // Execution
    Execute(ctx ExecutionContext) error
    GenerateArtifacts(ctx ExecutionContext) error

    // Discovery
    GetAvailableCommands() []string
}

type ExecutionContext struct {
    AtmosConfig *schema.AtmosConfiguration
    Command     string
    Subcommand  string
    Component   string
    Stack       string
    Config      map[string]any
    Args        []string
    Flags       map[string]any
}
```

### 4.2 Implementation Strategy

**For Terraform:**
```go
type TerraformComponentProvider struct{}

func (t *TerraformComponentProvider) GetType() string {
    return "terraform"
}

func (t *TerraformComponentProvider) GetGroup() string {
    return "Infrastructure as Code"
}

func (t *TerraformComponentProvider) Execute(ctx ExecutionContext) error {
    // Current logic from internal/exec/terraform.go
    return ExecuteTerraform(ctx.toConfigAndStacksInfo())
}

func (t *TerraformComponentProvider) GetAvailableCommands() []string {
    return []string{"plan", "apply", "destroy", "workspace", "clean", "deploy", "shell"}
}

func init() {
    components.Register(&TerraformComponentProvider{})
}
```

**For Helmfile:**
```go
type HelmfileComponentProvider struct{}

func (h *HelmfileComponentProvider) GetType() string {
    return "helmfile"
}

func (h *HelmfileComponentProvider) Execute(ctx ExecutionContext) error {
    // Current logic from internal/exec/helmfile.go
    return ExecuteHelmfile(ctx.toConfigAndStacksInfo())
}
```

### 4.3 Benefits of Component Interface

1. **Type Safety:** Compile-time checking instead of runtime string comparisons
2. **Extensibility:** Easy to add new component types (CDK, Pulumi, etc.)
3. **Plugin System:** External plugins could register new component types
4. **Consistency:** Enforced contract for all component types
5. **Testing:** Mock component providers for unit tests
6. **Discovery:** Introspection of available component types

### 4.4 Challenges & Considerations

**Challenge 1: Backward Compatibility**
- Current code expects component type as string constant
- Changing would require extensive refactoring
- **Solution:** Interface wraps existing implementation, string constants remain

**Challenge 2: Shared Configuration**
- `ConfigAndStacksInfo` struct is 539 lines, used everywhere
- Tightly coupled to current implementation
- **Solution:** Start with minimal interface, gradually expand

**Challenge 3: Component Type Detection**
- Current try/catch pattern works for YAML-based config
- Interface-based approach needs different discovery mechanism
- **Solution:** Keep detection logic, use interface after type is known

**Challenge 4: Template Functions**
- `atmos.Component()` assumes map-based structure
- Interface would need additional methods for template context
- **Solution:** Interface provides `ToMap()` method for template rendering

## 5. Integration Points for Interface

### 5.1 Files That Would Need Changes

**Core Component Logic:**
- `internal/exec/terraform.go` - Wrap in TerraformComponentProvider
- `internal/exec/helmfile.go` - Wrap in HelmfileComponentProvider
- `internal/exec/packer.go` - Wrap in PackerComponentProvider

**Component Discovery:**
- `pkg/list/list_components.go` - Use interface instead of hardcoded types
- `pkg/utils/component_path_utils.go` - Accept ComponentProvider parameter

**Component Processing:**
- `internal/exec/stack_processor_*.go` - Type detection via registry lookup
- `internal/exec/describe_component.go` - Interface-based execution

**Command Handlers:**
- `cmd/terraform.go` - Get provider from registry, call Execute()
- `cmd/helmfile.go` - Same pattern
- `cmd/list_components.go` - Iterate registered providers

**Template Functions:**
- `internal/exec/template_funcs_component.go` - Use interface methods

**New Files:**
- `pkg/component/provider.go` - ComponentProvider interface
- `pkg/component/registry.go` - Component registry (similar to command registry)
- `pkg/component/terraform.go` - TerraformComponentProvider
- `pkg/component/helmfile.go` - HelmfileComponentProvider
- `pkg/component/packer.go` - PackerComponentProvider

### 5.2 Migration Phases

**Phase 1: Foundation (Low Risk)**
1. Create ComponentProvider interface in `pkg/component/provider.go`
2. Create component registry in `pkg/component/registry.go`
3. Implement basic providers wrapping existing functions
4. Add 100% unit test coverage
5. No behavior changes, pure abstraction

**Phase 2: Integration (Medium Risk)**
1. Update `list components` to use registry
2. Update `describe component` to use registry
3. Add provider-based discovery utilities
4. Maintain backward compatibility with string constants

**Phase 3: Execution (Higher Risk)**
1. Update terraform/helmfile/packer commands to use providers
2. Refactor type detection to use registry
3. Update template functions to use providers
4. Comprehensive integration testing

**Phase 4: Extensions (Future)**
1. Plugin system for external component types
2. Dynamic component type loading
3. Community component providers

## 6. Recommendations

### 6.1 Should We Extract a Component Interface?

**YES, but incrementally:**

**Pros:**
✅ Aligns with command registry pattern (consistency)
✅ Enables plugin system for new component types
✅ Improves type safety and testability
✅ Makes codebase more maintainable long-term
✅ Facilitates future extensibility (CDK, Pulumi, etc.)

**Cons:**
❌ Significant refactoring effort (100+ files touched)
❌ Risk of breaking existing functionality
❌ Current map-based approach works well for YAML configs
❌ Interface overhead may not provide immediate value

### 6.2 Recommended Approach

**Start with Hybrid Interface (Option C):**

1. **Minimal Interface** - Only abstract what's truly polymorphic:
   ```go
   type ComponentProvider interface {
       GetType() string
       GetBasePath(*schema.AtmosConfiguration) string
       Execute(ExecutionContext) error
   }
   ```

2. **Gradual Migration:**
   - Phase 1: Create interface and basic providers (1 week)
   - Phase 2: Migrate `list` and `describe` commands (1 week)
   - Phase 3: Migrate execution commands (2 weeks)
   - Phase 4: Plugin system (future)

3. **Maintain Compatibility:**
   - Keep string constants (`TerraformComponentType` etc.)
   - Keep `ConfigAndStacksInfo` struct unchanged
   - Interface wraps existing implementation

4. **Test-Driven:**
   - 100% test coverage for interface implementation
   - Integration tests for each phase
   - No behavior changes until Phase 3

### 6.3 Alternative: Don't Extract Interface

**If the goal is only code organization:**
- Current architecture works well for YAML-driven configuration
- Map-based approach is flexible and easy to extend
- String-based type checking is simple and debuggable
- Focus effort on command registry migration instead

**Keep current approach if:**
- No plans to support new component types (CDK, Pulumi)
- Plugin system is not a priority
- Refactoring risk outweighs benefits

## 7. Conclusion

The Atmos component architecture is currently **data-driven and map-based**, which works well for YAML configuration processing. Components are differentiated by string constants and processed through type-specific functions.

The **command registry pattern** provides a solid foundation for similar component abstraction. However, extracting a component interface is a **larger effort** than command migration because:

1. Components are deeply integrated throughout the codebase
2. Component types affect execution flow, not just organization
3. Current map-based approach is flexible for YAML-driven config

**Recommendation:**
- **Proceed with component interface extraction** IF plugin system and extensibility are priorities
- **Start with minimal interface** and migrate incrementally
- **Focus on command registry migration first** to establish patterns and confidence
- **Defer component interface** until command registry is stable and benefits are clear

The research shows that while a component interface is **technically feasible** and **architecturally desirable**, it should be approached **cautiously** with **incremental implementation** to minimize risk.

---

## Appendix: Key Files Reference

### Command Registry
- `cmd/internal/command.go` - CommandProvider interface
- `cmd/internal/registry.go` - Registry implementation
- `docs/prd/command-registry-pattern.md` - Complete design doc

### Component Processing
- `internal/exec/component_utils.go` - Component utility functions
- `internal/exec/describe_component.go` - Component description logic
- `internal/exec/stack_processor_*.go` - Stack processing and merging
- `pkg/component/component_processor.go` - Component processing helpers

### Component Types
- `internal/exec/terraform.go` - Terraform execution
- `internal/exec/helmfile.go` - Helmfile execution
- `internal/exec/packer.go` - Packer execution (assumed)

### Component Discovery
- `pkg/list/list_components.go` - Component listing
- `pkg/utils/component_path_utils.go` - Path resolution

### Component Configuration
- `pkg/schema/schema.go` - Component configuration schemas
- `pkg/config/config.go` - Configuration loading

### Template Functions
- `internal/exec/template_funcs_component.go` - Component template functions

### Command Implementations
- `cmd/describe_component.go` - CLI for describe component
- `cmd/list_components.go` - CLI for list components
- `cmd/terraform.go` - Terraform parent command
- `cmd/terraform_commands.go` - Terraform dynamic commands
- `cmd/helmfile.go` - Helmfile parent command
