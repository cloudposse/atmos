package dependencies

import (
	"fmt"
	"os"
	execPkg "os/exec"
	"path/filepath"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// ToolchainEnvironment holds resolved executable paths and a subprocess PATH
// for a set of toolchain dependencies. Create one per command invocation using
// ForComponent, ForSections, or ForWorkflow, then use Resolve() and EnvVars()
// to configure subprocess execution.
type ToolchainEnvironment struct {
	// path is the full PATH string with toolchain bin dirs prepended.
	path string

	// dirs holds only the toolchain bin directories (without the system PATH).
	dirs []string

	// resolved maps bare command names (e.g. "tofu") to absolute paths.
	resolved map[string]string
}

// ForComponent creates a ToolchainEnvironment by resolving dependencies from
// component stack configuration. The componentType is "terraform", "helmfile",
// or "packer". Pass nil for stackSection/componentSection when no component
// context is available (e.g. version commands).
func ForComponent(
	atmosConfig *schema.AtmosConfiguration,
	componentType string,
	stackSection map[string]any,
	componentSection map[string]any,
) (*ToolchainEnvironment, error) {
	defer perf.Track(atmosConfig, "dependencies.ForComponent")()

	resolver := NewResolver(atmosConfig)
	deps, err := resolver.ResolveComponentDependencies(componentType, stackSection, componentSection)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve component dependencies: %w", err)
	}

	return newEnvironment(atmosConfig, deps)
}

// ForSections creates a ToolchainEnvironment from pre-loaded configuration
// sections (e.g. from DescribeComponent). Dependencies are extracted from
// the sections using ExtractDependenciesFromConfig.
func ForSections(atmosConfig *schema.AtmosConfiguration, sections map[string]any) (*ToolchainEnvironment, error) {
	defer perf.Track(atmosConfig, "dependencies.ForSections")()

	deps := ExtractDependenciesFromConfig(sections)
	return newEnvironment(atmosConfig, deps)
}

// ForWorkflow creates a ToolchainEnvironment for workflow execution.
// Merges .tool-versions with workflow-specific dependencies.
func ForWorkflow(atmosConfig *schema.AtmosConfiguration, workflowDef *schema.WorkflowDefinition, opts ...envOption) (*ToolchainEnvironment, error) {
	defer perf.Track(atmosConfig, "dependencies.ForWorkflow")()

	// Load project-wide tools from .tool-versions.
	toolVersionsDeps, err := LoadToolVersionsDependencies(atmosConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load .tool-versions: %w", err)
	}

	// Get workflow-specific dependencies.
	resolver := NewResolver(atmosConfig)
	workflowDeps, err := resolver.ResolveWorkflowDependencies(workflowDef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve workflow dependencies: %w", err)
	}

	// Merge: .tool-versions as base, workflow deps override.
	deps, err := MergeDependencies(toolVersionsDeps, workflowDeps)
	if err != nil {
		return nil, fmt.Errorf("failed to merge dependencies: %w", err)
	}

	return newEnvironment(atmosConfig, deps, opts...)
}

// Resolve returns the absolute path for a command name. If the command was
// installed via the toolchain, returns the toolchain path. Otherwise falls
// back to exec.LookPath, then returns the original name unchanged.
func (e *ToolchainEnvironment) Resolve(command string) string {
	defer perf.Track(nil, "dependencies.ToolchainEnvironment.Resolve")()

	if filepath.IsAbs(command) {
		return command
	}

	// Strip extension for cross-platform matching (e.g. "tofu.exe" → "tofu").
	key := strings.TrimSuffix(command, filepath.Ext(command))
	if p, ok := e.resolved[key]; ok {
		return p
	}

	if p, err := execPkg.LookPath(command); err == nil {
		return p
	}

	return command
}

// EnvVars returns a slice containing "PATH=<toolchain-augmented-path>" ready
// to append to a subprocess environment. Returns nil if no toolchain
// dependencies were resolved (no-op — does not override the system PATH).
func (e *ToolchainEnvironment) EnvVars() []string {
	defer perf.Track(nil, "dependencies.ToolchainEnvironment.EnvVars")()

	if e.path == "" {
		return nil
	}
	return []string{fmt.Sprintf("PATH=%s", e.path)}
}

// PATH returns the augmented PATH string with toolchain bin dirs prepended,
// or an empty string if no toolchain dependencies were resolved.
func (e *ToolchainEnvironment) PATH() string {
	defer perf.Track(nil, "dependencies.ToolchainEnvironment.PATH")()

	return e.path
}

// ToolchainDirs returns only the toolchain bin directories without the system PATH.
// Use this to prepend toolchain paths to an existing PATH without losing other entries.
func (e *ToolchainEnvironment) ToolchainDirs() []string {
	defer perf.Track(nil, "dependencies.ToolchainEnvironment.ToolchainDirs")()

	return e.dirs
}

// PrependToPath prepends toolchain bin dirs to the given PATH string.
// If there are no toolchain dirs, returns basePATH unchanged.
func (e *ToolchainEnvironment) PrependToPath(basePATH string) string {
	defer perf.Track(nil, "dependencies.ToolchainEnvironment.PrependToPath")()

	if len(e.dirs) == 0 {
		return basePATH
	}
	prefix := strings.Join(e.dirs, string(os.PathListSeparator))
	if basePATH == "" {
		return prefix
	}
	return prefix + string(os.PathListSeparator) + basePATH
}

// envConfig holds injectable dependencies for newEnvironment.
// Production code uses defaults; tests inject mocks via envOption.
type envConfig struct {
	ensureTools    func(deps map[string]string) error
	resolveFunc    func(tool string) (owner, repo string, err error)
	findBinaryPath func(owner, repo, version string, binaryName ...string) (string, error)
	buildPATH      func(atmosConfig *schema.AtmosConfiguration, deps map[string]string) (string, error)
}

// envOption configures newEnvironment for testing.
type envOption func(*envConfig)

// withEnsureTools overrides the tool installation function.
func withEnsureTools(fn func(deps map[string]string) error) envOption {
	return func(c *envConfig) {
		c.ensureTools = fn
	}
}

// withResolveFunc overrides the tool name → owner/repo resolution.
func withResolveFunc(fn func(tool string) (owner, repo string, err error)) envOption {
	return func(c *envConfig) {
		c.resolveFunc = fn
	}
}

// withFindBinaryPath overrides the binary path lookup.
func withFindBinaryPath(fn func(owner, repo, version string, binaryName ...string) (string, error)) envOption {
	return func(c *envConfig) {
		c.findBinaryPath = fn
	}
}

// withBuildPATH overrides the PATH construction.
func withBuildPATH(fn func(atmosConfig *schema.AtmosConfiguration, deps map[string]string) (string, error)) envOption {
	return func(c *envConfig) {
		c.buildPATH = fn
	}
}

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_tool_provisioner_test.go -package=dependencies . ToolProvisioner

// ToolProvisioner abstracts tool installation and resolution for testing.
// Production code uses the concrete Installer; tests inject a mock generated by mockgen.
type ToolProvisioner interface {
	// EnsureTools installs any missing tools for the given dependency map.
	EnsureTools(deps map[string]string) error
	// ResolveToolName maps a tool name to its owner/repo pair.
	ResolveToolName(tool string) (owner, repo string, err error)
	// FindBinaryPath locates the installed binary for a given owner/repo/version.
	FindBinaryPath(owner, repo, version string, binaryName ...string) (string, error)
	// BuildPATH constructs the augmented PATH containing toolchain bin dirs.
	BuildPATH(atmosConfig *schema.AtmosConfiguration, deps map[string]string) (string, error)
}

// withProvisioner configures newEnvironment to use a ToolProvisioner implementation.
func withProvisioner(p ToolProvisioner) envOption {
	return func(c *envConfig) {
		c.ensureTools = p.EnsureTools
		c.resolveFunc = p.ResolveToolName
		c.findBinaryPath = p.FindBinaryPath
		c.buildPATH = p.BuildPATH
	}
}

// newEnvironment is the shared constructor that installs missing tools,
// resolves all executable paths, and builds the augmented PATH.
func newEnvironment(atmosConfig *schema.AtmosConfiguration, deps map[string]string, opts ...envOption) (*ToolchainEnvironment, error) {
	defer perf.Track(atmosConfig, "dependencies.newEnvironment")()

	env := &ToolchainEnvironment{
		resolved: make(map[string]string),
	}

	if len(deps) == 0 {
		return env, nil
	}

	// Build config with defaults, then apply options.
	cfg := &envConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Initialize toolchain with atmosConfig so it uses the configured install path.
	toolchain.SetAtmosConfig(atmosConfig)

	// Install missing tools.
	ensureTools := cfg.ensureTools
	if ensureTools == nil {
		installer := NewInstaller(atmosConfig)
		ensureTools = installer.EnsureTools
	}
	if err := ensureTools(deps); err != nil {
		return nil, fmt.Errorf("failed to install dependencies: %w", err)
	}

	// Resolve every dependency to an absolute binary path.
	resolveBinaryPaths(env, cfg, deps)

	// Build augmented PATH.
	buildPATH := cfg.buildPATH
	if buildPATH == nil {
		buildPATH = BuildToolchainPATH
	}
	toolchainPATH, err := buildPATH(atmosConfig, deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build toolchain PATH: %w", err)
	}
	if toolchainPATH != getPathFromEnv() {
		env.path = toolchainPATH
	}

	// Extract unique toolchain bin dirs from resolved paths for PrependToPath().
	seen := make(map[string]bool)
	for _, p := range env.resolved {
		dir := filepath.Dir(p)
		if !seen[dir] {
			seen[dir] = true
			env.dirs = append(env.dirs, dir)
		}
	}

	return env, nil
}

// resolveBinaryPaths resolves each dependency to an absolute binary path
// and populates env.resolved. Errors are logged and skipped.
func resolveBinaryPaths(env *ToolchainEnvironment, cfg *envConfig, deps map[string]string) {
	resolveFunc := cfg.resolveFunc
	findBinaryPath := cfg.findBinaryPath
	if resolveFunc == nil || findBinaryPath == nil {
		tcInstaller := toolchain.NewInstaller()
		if resolveFunc == nil {
			resolver := tcInstaller.GetResolver()
			resolveFunc = resolver.Resolve
		}
		if findBinaryPath == nil {
			findBinaryPath = tcInstaller.FindBinaryPath
		}
	}

	for tool, version := range deps {
		owner, repo, err := resolveFunc(tool)
		if err != nil {
			log.Debug("Could not resolve tool for toolchain environment", "tool", tool, "error", err)
			continue
		}

		binaryPath, err := findBinaryPath(owner, repo, version)
		if err != nil {
			log.Debug("Could not find binary path for toolchain environment", "tool", tool, "version", version, "error", err)
			continue
		}

		// Store by both the requested tool name and the binary basename so
		// aliases/symlinks can still be resolved downstream.
		env.resolved[tool] = binaryPath
		base := filepath.Base(binaryPath)
		key := strings.TrimSuffix(base, filepath.Ext(base))
		env.resolved[key] = binaryPath
	}
}
