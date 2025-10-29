# Agent Marketplace - Technical Architecture

**Status:** Design Phase
**Version:** 1.0
**Last Updated:** 2025-10-28

## Package Structure

```
pkg/ai/agents/
├── agent.go                    # Core Agent struct (existing)
├── registry.go                 # Registry for managing agents (existing)
├── builtin.go                  # Built-in agent definitions (existing)
├── prompts/                    # Embedded prompts (existing)
│   ├── embedded.go
│   ├── general.md
│   └── ...
├── marketplace/                # NEW: Marketplace functionality
│   ├── installer.go            # Agent installation logic
│   ├── downloader.go           # Git clone/download operations
│   ├── validator.go            # Agent validation
│   ├── metadata.go             # .agent.yaml parsing
│   ├── local_registry.go       # Local agent registry (~/.atmos/agents/registry.json)
│   ├── updater.go              # Agent update logic
│   └── marketplace_test.go
```

## Core Data Structures

### Agent Metadata (`.agent.yaml`)

```go
// pkg/ai/agents/marketplace/metadata.go

package marketplace

import (
	"time"
)

// AgentMetadata represents the .agent.yaml configuration.
type AgentMetadata struct {
	// Basic info
	Name        string `yaml:"name" validate:"required,lowercase,alphanum_dash"`
	DisplayName string `yaml:"display_name" validate:"required"`
	Version     string `yaml:"version" validate:"required,semver"`
	Author      string `yaml:"author" validate:"required"`
	Description string `yaml:"description" validate:"required"`
	Category    string `yaml:"category" validate:"required,oneof=general analysis refactor security validation optimization"`

	// Atmos compatibility
	Atmos AtmosCompatibility `yaml:"atmos"`

	// Prompt configuration
	Prompt PromptConfig `yaml:"prompt" validate:"required"`

	// Tool access (optional)
	Tools *ToolConfig `yaml:"tools,omitempty"`

	// Capabilities (optional)
	Capabilities []string `yaml:"capabilities,omitempty"`

	// Dependencies (optional)
	Dependencies []string `yaml:"dependencies,omitempty"`

	// Environment variables (optional)
	Env []EnvVar `yaml:"env,omitempty"`

	// Links
	Repository    string `yaml:"repository" validate:"required,url"`
	Documentation string `yaml:"documentation,omitempty"`
}

type AtmosCompatibility struct {
	MinVersion string `yaml:"min_version" validate:"required,semver"`
	MaxVersion string `yaml:"max_version,omitempty"` // empty = no upper limit
}

type PromptConfig struct {
	File string `yaml:"file" validate:"required"` // e.g., "prompt.md"
}

type ToolConfig struct {
	Allowed    []string `yaml:"allowed,omitempty"`
	Restricted []string `yaml:"restricted,omitempty"`
}

type EnvVar struct {
	Name        string `yaml:"name" validate:"required"`
	Required    bool   `yaml:"required"`
	Description string `yaml:"description,omitempty"`
}

// ParseAgentMetadata reads and validates .agent.yaml file.
func ParseAgentMetadata(path string) (*AgentMetadata, error) {
	// Implementation:
	// 1. Read YAML file
	// 2. Unmarshal into AgentMetadata
	// 3. Validate using go-playground/validator
	// 4. Return metadata or error
}
```

### Local Registry

```go
// pkg/ai/agents/marketplace/local_registry.go

package marketplace

import (
	"time"
)

// LocalRegistry manages installed community agents.
type LocalRegistry struct {
	Version string                   `json:"version"`
	Agents  map[string]*InstalledAgent `json:"agents"`
	mu      sync.RWMutex
}

// InstalledAgent represents an installed community agent.
type InstalledAgent struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Source      string    `json:"source"`      // e.g., "github.com/user/repo"
	Version     string    `json:"version"`     // e.g., "1.2.3"
	InstalledAt time.Time `json:"installed_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Path        string    `json:"path"`        // Absolute path to agent directory
	IsBuiltIn   bool      `json:"is_builtin"`  // Always false for community agents
	Enabled     bool      `json:"enabled"`     // Can disable without uninstalling
}

// NewLocalRegistry creates or loads the local agent registry.
func NewLocalRegistry() (*LocalRegistry, error) {
	// Implementation:
	// 1. Check if ~/.atmos/agents/registry.json exists
	// 2. If exists, load and return
	// 3. If not, create new registry with defaults
	// 4. Save to disk
}

// Add registers a newly installed agent.
func (r *LocalRegistry) Add(agent *InstalledAgent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for name conflicts
	if _, exists := r.Agents[agent.Name]; exists {
		return fmt.Errorf("agent %q already installed", agent.Name)
	}

	r.Agents[agent.Name] = agent
	return r.save()
}

// Remove unregisters an agent.
func (r *LocalRegistry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.Agents[name]; !exists {
		return fmt.Errorf("agent %q not found", name)
	}

	delete(r.Agents, name)
	return r.save()
}

// Get retrieves an installed agent by name.
func (r *LocalRegistry) Get(name string) (*InstalledAgent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, exists := r.Agents[name]
	if !exists {
		return nil, fmt.Errorf("agent %q not found", name)
	}

	return agent, nil
}

// List returns all installed community agents.
func (r *LocalRegistry) List() []*InstalledAgent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*InstalledAgent, 0, len(r.Agents))
	for _, agent := range r.Agents {
		agents = append(agents, agent)
	}

	// Sort alphabetically
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Name < agents[j].Name
	})

	return agents
}

// save persists the registry to disk.
func (r *LocalRegistry) save() error {
	// Write to ~/.atmos/agents/registry.json
}
```

## Installation Flow

### High-Level Process

```
User Command: atmos ai agent install github.com/user/agent-name@v1.2.3
                           ↓
┌──────────────────────────────────────────────────────────┐
│ 1. Parse Source URL                                      │
│    - Determine source type (GitHub, URL, local)          │
│    - Extract owner, repo, ref (tag/branch)               │
└──────────────────────────────────────────────────────────┘
                           ↓
┌──────────────────────────────────────────────────────────┐
│ 2. Download Agent                                        │
│    - Clone Git repository to temporary directory         │
│    - Checkout specific version (tag/branch)              │
└──────────────────────────────────────────────────────────┘
                           ↓
┌──────────────────────────────────────────────────────────┐
│ 3. Validate Agent                                        │
│    - Check .agent.yaml exists and is valid               │
│    - Verify Atmos version compatibility                  │
│    - Check prompt.md exists                              │
│    - Validate tool access configuration                  │
└──────────────────────────────────────────────────────────┘
                           ↓
┌──────────────────────────────────────────────────────────┐
│ 4. Security Check                                        │
│    - Display agent metadata (author, repo, tools)        │
│    - Warn if agent requests destructive tool access      │
│    - Prompt user for confirmation                        │
└──────────────────────────────────────────────────────────┘
                           ↓
┌──────────────────────────────────────────────────────────┐
│ 5. Install Agent                                         │
│    - Move to ~/.atmos/agents/github.com/user/repo/       │
│    - Keep .git directory for updates                     │
│    - Register in local registry                          │
└──────────────────────────────────────────────────────────┘
                           ↓
┌──────────────────────────────────────────────────────────┐
│ 6. Success                                               │
│    - Display success message                             │
│    - Show usage instructions                             │
│    - Agent now appears in TUI switcher (Ctrl+A)          │
└──────────────────────────────────────────────────────────┘
```

### Installer Implementation

```go
// pkg/ai/agents/marketplace/installer.go

package marketplace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/ai/agents"
)

// Installer manages agent installation.
type Installer struct {
	downloader      *Downloader
	validator       *Validator
	localRegistry   *LocalRegistry
	agentRegistry   *agents.Registry
	atmosVersion    string
}

// NewInstaller creates a new agent installer.
func NewInstaller(atmosVersion string) (*Installer, error) {
	localRegistry, err := NewLocalRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to load local registry: %w", err)
	}

	agentRegistry := agents.NewRegistry()

	// Register all built-in agents
	for _, agent := range agents.GetBuiltInAgents() {
		if err := agentRegistry.Register(agent); err != nil {
			return nil, fmt.Errorf("failed to register built-in agent: %w", err)
		}
	}

	return &Installer{
		downloader:    NewDownloader(),
		validator:     NewValidator(atmosVersion),
		localRegistry: localRegistry,
		agentRegistry: agentRegistry,
		atmosVersion:  atmosVersion,
	}, nil
}

// Install installs an agent from a source URL.
func (i *Installer) Install(ctx context.Context, source string, opts InstallOptions) error {
	// 1. Parse source
	sourceInfo, err := ParseSource(source)
	if err != nil {
		return fmt.Errorf("invalid source: %w", err)
	}

	// 2. Check if already installed
	if !opts.Force {
		if _, err := i.localRegistry.Get(sourceInfo.Name); err == nil {
			return fmt.Errorf("agent %q already installed (use --force to reinstall)", sourceInfo.Name)
		}
	}

	// 3. Download to temporary directory
	tempDir, err := i.downloader.Download(ctx, sourceInfo)
	if err != nil {
		return fmt.Errorf("failed to download agent: %w", err)
	}
	defer os.RemoveAll(tempDir) // Cleanup on error

	// 4. Parse and validate metadata
	metadata, err := ParseAgentMetadata(filepath.Join(tempDir, ".agent.yaml"))
	if err != nil {
		return fmt.Errorf("invalid agent metadata: %w", err)
	}

	// 5. Validate agent
	if err := i.validator.Validate(tempDir, metadata); err != nil {
		return fmt.Errorf("agent validation failed: %w", err)
	}

	// 6. Security check (interactive prompt)
	if !opts.SkipConfirm {
		if err := i.confirmInstallation(metadata); err != nil {
			return err // User cancelled
		}
	}

	// 7. Install agent (move to final location)
	installPath := i.getInstallPath(sourceInfo)
	if err := os.MkdirAll(filepath.Dir(installPath), 0755); err != nil {
		return fmt.Errorf("failed to create install directory: %w", err)
	}

	if err := os.Rename(tempDir, installPath); err != nil {
		return fmt.Errorf("failed to install agent: %w", err)
	}

	// 8. Register agent
	installedAgent := &InstalledAgent{
		Name:        metadata.Name,
		DisplayName: metadata.DisplayName,
		Source:      sourceInfo.FullPath,
		Version:     metadata.Version,
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
		Path:        installPath,
		IsBuiltIn:   false,
		Enabled:     true,
	}

	if err := i.localRegistry.Add(installedAgent); err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}

	// 9. Load agent into runtime registry
	agent, err := i.loadInstalledAgent(installedAgent, metadata)
	if err != nil {
		return fmt.Errorf("failed to load agent: %w", err)
	}

	if err := i.agentRegistry.Register(agent); err != nil {
		return fmt.Errorf("failed to register agent in runtime: %w", err)
	}

	// 10. Success
	fmt.Printf("✓ Agent %q installed successfully\n", metadata.DisplayName)
	fmt.Printf("  Version: %s\n", metadata.Version)
	fmt.Printf("  Location: %s\n", installPath)
	fmt.Printf("\nUsage: Switch to this agent in the TUI with Ctrl+A\n")

	return nil
}

type InstallOptions struct {
	Force        bool   // Reinstall if already installed
	SkipConfirm  bool   // Skip security confirmation prompt
	CustomName   string // Install with custom name (--as)
}

// getInstallPath returns the installation path for an agent.
func (i *Installer) getInstallPath(source *SourceInfo) string {
	// ~/.atmos/agents/github.com/user/repo/
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".atmos", "agents", source.FullPath)
}

// confirmInstallation prompts user to confirm agent installation.
func (i *Installer) confirmInstallation(metadata *AgentMetadata) error {
	// Display agent info
	fmt.Printf("\nAgent: %s\n", metadata.DisplayName)
	fmt.Printf("Author: %s\n", metadata.Author)
	fmt.Printf("Version: %s\n", metadata.Version)
	fmt.Printf("Repository: %s\n", metadata.Repository)

	// Warn about tool access
	if metadata.Tools != nil && len(metadata.Tools.Allowed) > 0 {
		fmt.Printf("\nTool Access:\n")
		fmt.Printf("  Allowed: %s\n", strings.Join(metadata.Tools.Allowed, ", "))

		// Check for destructive tools
		destructiveTools := []string{"terraform_apply", "terraform_destroy", "helmfile_apply"}
		hasDestructive := false
		for _, tool := range metadata.Tools.Allowed {
			for _, destructive := range destructiveTools {
				if tool == destructive {
					hasDestructive = true
					break
				}
			}
		}

		if hasDestructive {
			fmt.Printf("\n⚠️  WARNING: This agent requests access to destructive operations.\n")
			fmt.Printf("   Review the agent source before using:\n")
			fmt.Printf("   %s\n", metadata.Repository)
		}
	}

	// Prompt for confirmation
	fmt.Printf("\nDo you want to install this agent? [y/N] ")
	var response string
	fmt.Scanln(&response)

	if strings.ToLower(strings.TrimSpace(response)) != "y" {
		return fmt.Errorf("installation cancelled by user")
	}

	return nil
}

// loadInstalledAgent creates an Agent from installed metadata.
func (i *Installer) loadInstalledAgent(installed *InstalledAgent, metadata *AgentMetadata) (*agents.Agent, error) {
	// Read prompt from file
	promptPath := filepath.Join(installed.Path, metadata.Prompt.File)
	promptContent, err := os.ReadFile(promptPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompt file: %w", err)
	}

	agent := &agents.Agent{
		Name:         metadata.Name,
		DisplayName:  metadata.DisplayName,
		Description:  metadata.Description,
		SystemPrompt: string(promptContent),
		Category:     metadata.Category,
		IsBuiltIn:    false,
	}

	// Set tool access
	if metadata.Tools != nil {
		agent.AllowedTools = metadata.Tools.Allowed
		agent.RestrictedTools = metadata.Tools.Restricted
	}

	return agent, nil
}
```

### Source Parsing

```go
// pkg/ai/agents/marketplace/source.go

package marketplace

import (
	"fmt"
	"net/url"
	"strings"
)

// SourceInfo contains parsed source information.
type SourceInfo struct {
	Type     string // "github", "git", "local"
	Owner    string // GitHub owner
	Repo     string // GitHub repo name
	Ref      string // Tag, branch, or commit (optional)
	URL      string // Full URL
	FullPath string // e.g., "github.com/user/repo"
	Name     string // Agent name (derived from repo)
}

// ParseSource parses various source formats.
func ParseSource(source string) (*SourceInfo, error) {
	// Format 1: github.com/user/repo
	// Format 2: github.com/user/repo@v1.2.3
	// Format 3: https://github.com/user/repo.git
	// Format 4: git@github.com:user/repo.git

	// Remove @ref suffix if present
	ref := ""
	if idx := strings.LastIndex(source, "@"); idx > 0 {
		ref = source[idx+1:]
		source = source[:idx]
	}

	// Parse GitHub shorthand: github.com/user/repo
	if strings.HasPrefix(source, "github.com/") {
		parts := strings.Split(strings.TrimPrefix(source, "github.com/"), "/")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid GitHub source format")
		}

		return &SourceInfo{
			Type:     "github",
			Owner:    parts[0],
			Repo:     parts[1],
			Ref:      ref,
			URL:      fmt.Sprintf("https://github.com/%s/%s.git", parts[0], parts[1]),
			FullPath: fmt.Sprintf("github.com/%s/%s", parts[0], parts[1]),
			Name:     parts[1],
		}, nil
	}

	// Parse full Git URL
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "git@") {
		// Use go-git to parse
		// ...
	}

	return nil, fmt.Errorf("unsupported source format: %s", source)
}
```

## Agent Loading at Runtime

### Integration with Existing Registry

```go
// pkg/ai/agents/registry.go (modifications)

// LoadCommunityAgents loads installed community agents into the registry.
func (r *Registry) LoadCommunityAgents() error {
	localRegistry, err := marketplace.NewLocalRegistry()
	if err != nil {
		// No community agents installed, not an error
		return nil
	}

	for _, installed := range localRegistry.List() {
		if !installed.Enabled {
			continue // Skip disabled agents
		}

		// Parse metadata
		metadataPath := filepath.Join(installed.Path, ".agent.yaml")
		metadata, err := marketplace.ParseAgentMetadata(metadataPath)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to load community agent %q: %v", installed.Name, err))
			continue
		}

		// Read prompt
		promptPath := filepath.Join(installed.Path, metadata.Prompt.File)
		promptContent, err := os.ReadFile(promptPath)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to read prompt for agent %q: %v", installed.Name, err))
			continue
		}

		// Create agent
		agent := &Agent{
			Name:         metadata.Name,
			DisplayName:  metadata.DisplayName,
			Description:  metadata.Description,
			SystemPrompt: string(promptContent),
			Category:     metadata.Category,
			IsBuiltIn:    false,
		}

		if metadata.Tools != nil {
			agent.AllowedTools = metadata.Tools.Allowed
			agent.RestrictedTools = metadata.Tools.Restricted
		}

		// Register
		if err := r.Register(agent); err != nil {
			log.Warn(fmt.Sprintf("Failed to register community agent %q: %v", installed.Name, err))
			continue
		}
	}

	return nil
}
```

### TUI Integration

```go
// pkg/ai/tui/chat.go (modifications in NewChatModel)

func NewChatModel(...) (*ChatModel, error) {
	// ... existing code ...

	// Load built-in agents
	for _, agent := range agents.GetBuiltInAgents() {
		if err := agentRegistry.Register(agent); err != nil {
			return nil, err
		}
	}

	// Load community agents
	if err := agentRegistry.LoadCommunityAgents(); err != nil {
		log.Warn(fmt.Sprintf("Failed to load community agents: %v", err))
		// Non-fatal error, continue
	}

	// ... rest of existing code ...
}
```

## Testing Strategy

### Unit Tests

```go
// pkg/ai/agents/marketplace/installer_test.go

func TestParseSource(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *SourceInfo
		wantErr  bool
	}{
		{
			name:  "GitHub shorthand",
			input: "github.com/user/repo",
			expected: &SourceInfo{
				Type:     "github",
				Owner:    "user",
				Repo:     "repo",
				FullPath: "github.com/user/repo",
				Name:     "repo",
			},
		},
		{
			name:  "GitHub shorthand with tag",
			input: "github.com/user/repo@v1.2.3",
			expected: &SourceInfo{
				Type:     "github",
				Owner:    "user",
				Repo:     "repo",
				Ref:      "v1.2.3",
				FullPath: "github.com/user/repo",
				Name:     "repo",
			},
		},
		{
			name:    "Invalid format",
			input:   "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseSource(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
```

### Integration Tests

```bash
# tests/marketplace_test.go

func TestAgentInstallation(t *testing.T) {
	// Prerequisite: Create test agent repo
	testRepo := setupTestAgentRepo(t)
	defer cleanupTestRepo(t, testRepo)

	// Install agent
	installer := marketplace.NewInstaller("1.50.0")
	err := installer.Install(context.Background(), testRepo.URL, marketplace.InstallOptions{
		SkipConfirm: true, // Non-interactive for tests
	})
	require.NoError(t, err)

	// Verify installation
	registry, err := marketplace.NewLocalRegistry()
	require.NoError(t, err)

	agent, err := registry.Get("test-agent")
	require.NoError(t, err)
	assert.Equal(t, "Test Agent", agent.DisplayName)

	// Verify agent appears in runtime registry
	agentRegistry := agents.NewRegistry()
	err = agentRegistry.LoadCommunityAgents()
	require.NoError(t, err)

	loadedAgent, err := agentRegistry.Get("test-agent")
	require.NoError(t, err)
	assert.NotNil(t, loadedAgent)
}
```

## Error Handling

### Error Types

```go
// pkg/ai/agents/marketplace/errors.go

package marketplace

import (
	"errors"
	"fmt"
)

var (
	// Installation errors
	ErrAgentAlreadyInstalled = errors.New("agent already installed")
	ErrInvalidSource         = errors.New("invalid agent source")
	ErrDownloadFailed        = errors.New("agent download failed")

	// Validation errors
	ErrInvalidMetadata       = errors.New("invalid agent metadata")
	ErrIncompatibleVersion   = errors.New("incompatible Atmos version")
	ErrMissingPromptFile     = errors.New("prompt file not found")
	ErrInvalidToolConfig     = errors.New("invalid tool configuration")

	// Registry errors
	ErrAgentNotFound         = errors.New("agent not found")
	ErrRegistryCorrupted     = errors.New("registry file corrupted")
)

// ValidationError provides detailed validation failure information.
type ValidationError struct {
	Field   string
	Message string
	Err     error
}

func (e *ValidationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("validation failed for %s: %s (%v)", e.Field, e.Message, e.Err)
	}
	return fmt.Sprintf("validation failed for %s: %s", e.Field, e.Message)
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}
```

## Performance Considerations

1. **Caching** - Cache parsed metadata to avoid repeated file reads
2. **Lazy Loading** - Load community agents on-demand, not at startup
3. **Concurrent Downloads** - Support installing multiple agents in parallel
4. **Registry Locking** - Use file locking for concurrent access
5. **Prompt Size** - Validate prompt file size (warn if >100KB)

## Security Hardening

1. **Path Traversal** - Sanitize agent names and paths
2. **Git Safety** - Use shallow clones, verify HTTPS
3. **Metadata Validation** - Strict schema validation
4. **Tool Restrictions** - Enforce tool access limits
5. **Prompt Injection** - Sanitize prompt content (escape special chars)
6. **Rate Limiting** - Limit install operations per minute

## Future Enhancements

### Phase 3: Advanced Features

1. **Agent Dependencies** - Install dependent agents automatically
2. **Agent Marketplace API** - Centralized discovery service
3. **Agent Ratings** - Community rating and review system
4. **Private Registries** - Support for private agent repositories
5. **Agent Sandboxing** - Run agents in isolated environments
6. **Auto-Updates** - Automatic agent updates (opt-in)
7. **Agent Analytics** - Track agent usage and performance

## References

- [Go-Git Library](https://github.com/go-git/go-git) - Git operations in Go
- [go-playground/validator](https://github.com/go-playground/validator) - Struct validation
- [Semantic Versioning](https://semver.org/) - Version format
- [YAML v3](https://github.com/go-yaml/yaml) - YAML parsing

## Changelog

- 2025-10-28: Initial architecture document
