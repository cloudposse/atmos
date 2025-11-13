# Testability Refactoring Strategy for Atmos

## Goal
Establish patterns that make future development easier by enabling comprehensive unit testing without requiring real infrastructure (Docker, AWS, filesystem, TTY, etc.).

## Core Principles

### 1. Dependency Injection over Global State
**Current Problem:**
```go
// Hard to test - directly calls global packages
func getDevcontainerName(args []string) (string, error) {
    atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
    // ...
    selectedName, err := promptForDevcontainer("Select:", devcontainers)
    // ...
    fmt.Fprintf(os.Stderr, "Selected: %s\n", selectedName)
}
```

**Solution:**
```go
// Testable - dependencies are injected
func getDevcontainerName(
    args []string,
    configLoader ConfigLoader,
    prompter Prompter,
    output io.Writer,
) (string, error) {
    atmosConfig, err := configLoader.Load()
    // ...
    selectedName, err := prompter.Select("Select:", devcontainers)
    // ...
    fmt.Fprintf(output, "Selected: %s\n", selectedName)
}
```

### 2. Interface-Driven Design
Define interfaces for all external dependencies:

```go
// ConfigLoader abstracts configuration loading
type ConfigLoader interface {
    Load() (*schema.AtmosConfiguration, error)
}

// Prompter abstracts interactive prompts
type Prompter interface {
    Select(message string, options []string) (string, error)
}

// DevcontainerLister abstracts devcontainer discovery
type DevcontainerLister interface {
    List(config *schema.AtmosConfiguration) ([]string, error)
}
```

### 3. Options Pattern for Complex Dependencies
Use functional options to avoid parameter explosion:

```go
type HelperOptions struct {
    ConfigLoader ConfigLoader
    Prompter     Prompter
    Output       io.Writer
}

type HelperOption func(*HelperOptions)

func WithConfigLoader(loader ConfigLoader) HelperOption {
    return func(o *HelperOptions) { o.ConfigLoader = loader }
}

func WithPrompter(p Prompter) HelperOption {
    return func(o *HelperOptions) { o.Prompter = p }
}

// Usage in tests
opts := NewHelperOptions(
    WithConfigLoader(mockLoader),
    WithPrompter(mockPrompter),
)
```

### 4. Separate Construction from Execution
**Current Problem:**
```go
// Command RunE directly creates and uses dependencies
RunE: func(cmd *cobra.Command, args []string) error {
    name, err := getDevcontainerName(args) // Hard-coded dependencies inside
    if err != nil {
        return err
    }
    // ... use name
}
```

**Solution:**
```go
// Separate construction (in init or command setup)
type AttachCommand struct {
    configLoader ConfigLoader
    prompter     Prompter
    // ... other dependencies
}

// Execution uses injected dependencies
func (c *AttachCommand) Run(cmd *cobra.Command, args []string) error {
    name, err := getDevcontainerName(args, c.configLoader, c.prompter, os.Stderr)
    // ...
}
```

## Proposed Refactoring

### Phase 1: Extract Interfaces (Low Risk)

Create `cmd/devcontainer/interfaces.go`:

```go
package devcontainer

import (
    "io"
    "github.com/cloudposse/atmos/pkg/schema"
)

// ConfigLoader abstracts Atmos configuration loading.
type ConfigLoader interface {
    Load() (*schema.AtmosConfiguration, error)
}

// Prompter abstracts interactive user prompts.
type Prompter interface {
    // Select displays a menu and returns the selected item.
    Select(message string, options []string) (string, error)
}

// DevcontainerLister lists available devcontainers from config.
type DevcontainerLister interface {
    List(config *schema.AtmosConfiguration) ([]string, error)
}

// TTYDetector checks if running in interactive terminal.
type TTYDetector interface {
    IsInteractive() bool
}
```

### Phase 2: Create Default Implementations

Create `cmd/devcontainer/implementations.go`:

```go
package devcontainer

import (
    "fmt"
    "os"
    "sort"

    "github.com/charmbracelet/huh"
    "github.com/mattn/go-isatty"

    cfg "github.com/cloudposse/atmos/pkg/config"
    errUtils "github.com/cloudposse/atmos/errors"
    "github.com/cloudposse/atmos/pkg/schema"
)

// DefaultConfigLoader implements ConfigLoader using pkg/config.
type DefaultConfigLoader struct{}

func (d *DefaultConfigLoader) Load() (*schema.AtmosConfiguration, error) {
    config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
    if err != nil {
        return nil, fmt.Errorf("failed to load atmos config: %w", err)
    }
    return &config, nil
}

// HuhPrompter implements Prompter using charmbracelet/huh.
type HuhPrompter struct{}

func (h *HuhPrompter) Select(message string, options []string) (string, error) {
    if len(options) == 0 {
        return "", fmt.Errorf("%w: no options available", errUtils.ErrDevcontainerNotFound)
    }

    var selected string
    form := huh.NewForm(
        huh.NewGroup(
            huh.NewSelect[string]().
                Title(message).
                Options(huh.NewOptions(options...)...).
                Value(&selected),
        ),
    )

    if err := form.Run(); err != nil {
        return "", fmt.Errorf("prompt failed: %w", err)
    }

    return selected, nil
}

// DefaultDevcontainerLister implements DevcontainerLister.
type DefaultDevcontainerLister struct{}

func (d *DefaultDevcontainerLister) List(config *schema.AtmosConfiguration) ([]string, error) {
    if config == nil || config.Components.Devcontainer == nil {
        return nil, fmt.Errorf("%w: no devcontainers configured", errUtils.ErrDevcontainerNotFound)
    }

    var names []string
    for name := range config.Components.Devcontainer {
        names = append(names, name)
    }

    sort.Strings(names)
    return names, nil
}

// StdinTTYDetector implements TTYDetector.
type StdinTTYDetector struct{}

func (s *StdinTTYDetector) IsInteractive() bool {
    return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}
```

### Phase 3: Refactor Helper Functions

Refactor `cmd/devcontainer/helpers.go`:

```go
package devcontainer

import (
    "fmt"
    "io"

    errUtils "github.com/cloudposse/atmos/errors"
)

// NameResolver provides devcontainer name resolution with dependency injection.
type NameResolver struct {
    configLoader ConfigLoader
    lister       DevcontainerLister
    prompter     Prompter
    ttyDetector  TTYDetector
    output       io.Writer
}

// NewNameResolver creates a resolver with default dependencies.
func NewNameResolver() *NameResolver {
    return &NameResolver{
        configLoader: &DefaultConfigLoader{},
        lister:       &DefaultDevcontainerLister{},
        prompter:     &HuhPrompter{},
        ttyDetector:  &StdinTTYDetector{},
        output:       os.Stderr,
    }
}

// NewTestableNameResolver creates a resolver with injectable dependencies (for tests).
func NewTestableNameResolver(
    configLoader ConfigLoader,
    lister DevcontainerLister,
    prompter Prompter,
    ttyDetector TTYDetector,
    output io.Writer,
) *NameResolver {
    return &NameResolver{
        configLoader: configLoader,
        lister:       lister,
        prompter:     prompter,
        ttyDetector:  ttyDetector,
        output:       output,
    }
}

// Resolve gets devcontainer name from args or prompts user.
func (r *NameResolver) Resolve(args []string) (string, error) {
    // If name provided in args, use it.
    if len(args) > 0 && args[0] != "" {
        return args[0], nil
    }

    // Check if running in interactive mode.
    if !r.ttyDetector.IsInteractive() {
        return "", fmt.Errorf("%w: devcontainer name is required in non-interactive mode",
            errUtils.ErrDevcontainerNameEmpty)
    }

    // Load config.
    config, err := r.configLoader.Load()
    if err != nil {
        return "", err // Error already has context from configLoader
    }

    // Get available devcontainers.
    devcontainers, err := r.lister.List(config)
    if err != nil {
        return "", err
    }

    if len(devcontainers) == 0 {
        return "", fmt.Errorf("%w: no devcontainers configured in atmos.yaml",
            errUtils.ErrDevcontainerNotFound)
    }

    // Prompt user.
    selectedName, err := r.prompter.Select("Select a devcontainer:", devcontainers)
    if err != nil {
        return "", err
    }

    // Display selection.
    fmt.Fprintf(r.output, "\nSelected devcontainer: %s\n\n", selectedName)

    return selectedName, nil
}

// LEGACY: Keep old function for backward compatibility, mark as deprecated.
// Deprecated: Use NameResolver.Resolve instead.
func getDevcontainerName(args []string) (string, error) {
    resolver := NewNameResolver()
    return resolver.Resolve(args)
}
```

### Phase 4: Update Commands to Use Resolver

Update `cmd/devcontainer/attach.go`:

```go
package devcontainer

import (
    "github.com/spf13/cobra"
    // ... other imports
)

var (
    attachParser *flags.StandardParser
    nameResolver *NameResolver // Shared resolver
)

var attachCmd = &cobra.Command{
    Use:   "attach [devcontainer-name]",
    Short: "Attach to a running devcontainer",
    Long:  attachLong,
    ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
        return devcontainerNameCompletion(cmd, args, toComplete)
    },
    RunE: func(cmd *cobra.Command, args []string) error {
        v := viper.GetViper()

        // Get devcontainer name (using injectable resolver).
        name, err := nameResolver.Resolve(args)
        if err != nil {
            return err
        }

        // Parse options.
        opts, err := parseAttachOptions(cmd, v, args)
        if err != nil {
            return err
        }

        // Execute attach.
        return devcontainer.Attach(cmd.Context(), atmosConfigPtr, name, devcontainer.AttachOptions{
            Instance: opts.Instance,
            UsePTY:   opts.UsePTY,
        })
    },
}

func init() {
    // Initialize shared resolver with defaults.
    nameResolver = NewNameResolver()

    // ... rest of init
}
```

### Phase 5: Comprehensive Test Suite

Create `cmd/devcontainer/helpers_integration_test.go`:

```go
package devcontainer

import (
    "bytes"
    "errors"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    errUtils "github.com/cloudposse/atmos/errors"
    "github.com/cloudposse/atmos/pkg/schema"
)

// Mock implementations for testing.

type mockConfigLoader struct {
    config *schema.AtmosConfiguration
    err    error
}

func (m *mockConfigLoader) Load() (*schema.AtmosConfiguration, error) {
    return m.config, m.err
}

type mockPrompter struct {
    result string
    err    error
}

func (m *mockPrompter) Select(message string, options []string) (string, error) {
    return m.result, m.err
}

type mockDevcontainerLister struct {
    devcontainers []string
    err           error
}

func (m *mockDevcontainerLister) List(config *schema.AtmosConfiguration) ([]string, error) {
    return m.devcontainers, m.err
}

type mockTTYDetector struct {
    interactive bool
}

func (m *mockTTYDetector) IsInteractive() bool {
    return m.interactive
}

// Test suite covering all branches.

func TestNameResolver_Resolve(t *testing.T) {
    tests := []struct {
        name            string
        args            []string
        configLoader    ConfigLoader
        lister          DevcontainerLister
        prompter        Prompter
        ttyDetector     TTYDetector
        expectedName    string
        expectedError   error
        expectErrorType error
    }{
        {
            name: "name provided in args",
            args: []string{"geodesic"},
            // No need to set other dependencies - short-circuit
            ttyDetector:  &mockTTYDetector{interactive: true},
            expectedName: "geodesic",
        },
        {
            name:            "no args non-interactive mode",
            args:            []string{},
            ttyDetector:     &mockTTYDetector{interactive: false},
            expectErrorType: errUtils.ErrDevcontainerNameEmpty,
        },
        {
            name:         "config loading fails",
            args:         []string{},
            ttyDetector:  &mockTTYDetector{interactive: true},
            configLoader: &mockConfigLoader{err: errors.New("config error")},
            expectedError: errors.New("config error"),
        },
        {
            name:        "no devcontainers available",
            args:        []string{},
            ttyDetector: &mockTTYDetector{interactive: true},
            configLoader: &mockConfigLoader{
                config: &schema.AtmosConfiguration{
                    Components: schema.Components{
                        Devcontainer: map[string]interface{}{},
                    },
                },
            },
            lister:          &mockDevcontainerLister{devcontainers: []string{}},
            expectErrorType: errUtils.ErrDevcontainerNotFound,
        },
        {
            name:        "prompt user successfully",
            args:        []string{},
            ttyDetector: &mockTTYDetector{interactive: true},
            configLoader: &mockConfigLoader{
                config: &schema.AtmosConfiguration{
                    Components: schema.Components{
                        Devcontainer: map[string]interface{}{
                            "geodesic": map[string]interface{}{},
                        },
                    },
                },
            },
            lister:       &mockDevcontainerLister{devcontainers: []string{"geodesic"}},
            prompter:     &mockPrompter{result: "geodesic"},
            expectedName: "geodesic",
        },
        {
            name:        "prompt fails",
            args:        []string{},
            ttyDetector: &mockTTYDetector{interactive: true},
            configLoader: &mockConfigLoader{
                config: &schema.AtmosConfiguration{
                    Components: schema.Components{
                        Devcontainer: map[string]interface{}{
                            "geodesic": map[string]interface{}{},
                        },
                    },
                },
            },
            lister:        &mockDevcontainerLister{devcontainers: []string{"geodesic"}},
            prompter:      &mockPrompter{err: errors.New("user cancelled")},
            expectedError: errors.New("user cancelled"),
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            output := &bytes.Buffer{}

            resolver := NewTestableNameResolver(
                tt.configLoader,
                tt.lister,
                tt.prompter,
                tt.ttyDetector,
                output,
            )

            name, err := resolver.Resolve(tt.args)

            if tt.expectErrorType != nil {
                require.Error(t, err)
                assert.ErrorIs(t, err, tt.expectErrorType)
            } else if tt.expectedError != nil {
                require.Error(t, err)
                assert.Contains(t, err.Error(), tt.expectedError.Error())
            } else {
                require.NoError(t, err)
                assert.Equal(t, tt.expectedName, name)
            }
        })
    }
}
```

## Benefits of This Approach

### 1. **100% Unit Test Coverage Achievable**
- Every branch can be tested without real infrastructure
- Tests run fast (no Docker, no filesystem, no network)
- Deterministic results (no flaky tests)

### 2. **Clear Separation of Concerns**
- Interfaces define contracts
- Implementations are swappable
- Commands are thin orchestrators

### 3. **Easier to Reason About**
- Dependencies are explicit (in constructor/function signature)
- No hidden global state
- Clear data flow

### 4. **Future-Proof**
- Easy to add new implementations (e.g., different prompt library)
- Easy to add new features (just extend interfaces)
- Easy to refactor (change implementation without changing interface)

### 5. **Follows Atmos Patterns**
- Already using this pattern in `pkg/store/` with registry
- Already using this pattern in `pkg/auth/` with providers
- Extends existing architectural patterns

## Migration Strategy

### Step 1: No Breaking Changes
- Add interfaces alongside existing code
- Create default implementations that wrap existing logic
- Keep old functions working (mark as deprecated)

### Step 2: Gradual Adoption
- New code uses new patterns
- Refactor existing code one command at a time
- Run both old and new tests in parallel

### Step 3: Documentation
- Update `CLAUDE.md` with new patterns
- Create examples in `docs/prd/`
- Add ADR (Architecture Decision Record)

### Step 4: Tooling
- Create scaffolding scripts for new commands
- Add linter rules to enforce patterns
- Update code review checklist

## Next Steps

1. **Get approval on approach** - Review this PRD with team
2. **Prototype one command** - Implement for `devcontainer attach`
3. **Measure impact** - Run tests, check coverage
4. **Document learnings** - Update PRD based on experience
5. **Roll out gradually** - Apply to other commands

## Questions to Resolve

1. **Command struct lifetime** - Should commands be long-lived singletons or created per-invocation?
2. **Global resolver** - Should we have one shared `NameResolver` or create per-command?
3. **Error wrapping** - Should interfaces wrap errors or return raw errors?
4. **Context propagation** - Should interfaces accept `context.Context`?
