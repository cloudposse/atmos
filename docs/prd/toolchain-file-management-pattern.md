# Design Pattern: Toolchain File Management

## Status: Approved

**Last Updated**: 2025-11-09

**Related PRDs**: [Toolchain Implementation](./toolchain-implementation.md) | [Lock File Support](./toolchain-lock-file.md)

---

## Problem

Multiple toolchain commands need to update both `.tool-versions` and `toolchain.lock.yaml`:
- `install` - Add/update tool entries
- `uninstall`/`remove` - Remove tool entries
- `set` - Update default version
- `lock` - Regenerate lock file

Current issues:
- Duplicate code for file updates across commands
- No consistent way to enable/disable file updates
- Hard to add new file types (e.g., future formats)
- Difficult to test file updates in isolation

## Solution: File Manager Registry Pattern

Use a **registry of file managers** where each manager handles one file type. Commands register actions, and the registry coordinates updates.

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     Command Layer                        │
│  (install, uninstall, set, lock commands)               │
└────────────────┬────────────────────────────────────────┘
                 │
                 ↓
┌─────────────────────────────────────────────────────────┐
│                 FileManagerRegistry                      │
│  - Coordinates updates to all enabled file managers     │
│  - Respects configuration (use_tool_versions, etc.)     │
└────────────────┬────────────────────────────────────────┘
                 │
                 ↓
         ┌───────┴───────┐
         ↓               ↓
┌─────────────────┐ ┌─────────────────┐
│ ToolVersionsFile│ │  LockFileManager│
│    Manager      │ │                 │
└─────────────────┘ └─────────────────┘
```

## Implementation

### 1. FileManager Interface

```go
// pkg/toolchain/filemanager/interface.go
package filemanager

import "context"

// FileManager manages a specific toolchain file type.
type FileManager interface {
	// Enabled returns true if this file manager is enabled by configuration.
	Enabled() bool

	// AddTool adds or updates a tool version.
	AddTool(ctx context.Context, tool, version string, opts ...AddOption) error

	// RemoveTool removes a tool version.
	RemoveTool(ctx context.Context, tool, version string) error

	// SetDefault sets a tool version as default.
	SetDefault(ctx context.Context, tool, version string) error

	// GetTools returns all tools managed by this file.
	GetTools(ctx context.Context) (map[string][]string, error)

	// Verify verifies the integrity of the managed file.
	Verify(ctx context.Context) error

	// Name returns the manager name for logging.
	Name() string
}

// AddOption configures tool addition behavior.
type AddOption func(*AddConfig)

type AddConfig struct {
	AsDefault bool
	Platform  string
	Checksum  string
	URL       string
	Size      int64
}

// WithAsDefault adds the tool as the default version.
func WithAsDefault() AddOption {
	return func(c *AddConfig) {
		c.AsDefault = true
	}
}

// WithPlatform specifies the platform for multi-platform lock files.
func WithPlatform(platform string) AddOption {
	return func(c *AddConfig) {
		c.Platform = platform
	}
}

// WithChecksum adds checksum for lock file.
func WithChecksum(checksum string) AddOption {
	return func(c *AddConfig) {
		c.Checksum = checksum
	}
}

// WithURL adds download URL for lock file.
func WithURL(url string) AddOption {
	return func(c *AddConfig) {
		c.URL = url
	}
}

// WithSize adds file size for lock file.
func WithSize(size int64) AddOption {
	return func(c *AddConfig) {
		c.Size = size
	}
}
```

### 2. ToolVersionsFile Manager

```go
// pkg/toolchain/filemanager/toolversions.go
package filemanager

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/toolchain"
)

// ToolVersionsFileManager manages .tool-versions file.
type ToolVersionsFileManager struct {
	config   *schema.AtmosConfiguration
	filePath string
}

// NewToolVersionsFileManager creates a new .tool-versions manager.
func NewToolVersionsFileManager(config *schema.AtmosConfiguration) *ToolVersionsFileManager {
	filePath := config.Toolchain.VersionsFile
	if filePath == "" {
		filePath = toolchain.DefaultToolVersionsFilePath
	}

	return &ToolVersionsFileManager{
		config:   config,
		filePath: filePath,
	}
}

func (m *ToolVersionsFileManager) Enabled() bool {
	// Check configuration flag
	return m.config.Toolchain.UseToolVersions
}

func (m *ToolVersionsFileManager) AddTool(ctx context.Context, tool, version string, opts ...AddOption) error {
	if !m.Enabled() {
		return nil // Skip if disabled
	}

	cfg := &AddConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.AsDefault {
		return toolchain.AddToolToVersionsAsDefault(m.filePath, tool, version)
	}
	return toolchain.AddToolToVersions(m.filePath, tool, version)
}

func (m *ToolVersionsFileManager) RemoveTool(ctx context.Context, tool, version string) error {
	if !m.Enabled() {
		return nil
	}

	return toolchain.RemoveToolFromVersions(m.filePath, tool, version)
}

func (m *ToolVersionsFileManager) SetDefault(ctx context.Context, tool, version string) error {
	if !m.Enabled() {
		return nil
	}

	return toolchain.AddToolToVersionsAsDefault(m.filePath, tool, version)
}

func (m *ToolVersionsFileManager) GetTools(ctx context.Context) (map[string][]string, error) {
	if !m.Enabled() {
		return nil, nil
	}

	tv, err := toolchain.LoadToolVersions(m.filePath)
	if err != nil {
		return nil, err
	}
	return tv.Tools, nil
}

func (m *ToolVersionsFileManager) Verify(ctx context.Context) error {
	if !m.Enabled() {
		return nil
	}

	// .tool-versions doesn't have verification (no checksums)
	return nil
}

func (m *ToolVersionsFileManager) Name() string {
	return "tool-versions"
}
```

### 3. LockFile Manager

```go
// pkg/toolchain/filemanager/lockfile.go
package filemanager

import (
	"context"
	"runtime"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/toolchain/lockfile"
)

// LockFileManager manages toolchain.lock.yaml file.
type LockFileManager struct {
	config   *schema.AtmosConfiguration
	filePath string
}

// NewLockFileManager creates a new lock file manager.
func NewLockFileManager(config *schema.AtmosConfiguration) *LockFileManager {
	filePath := config.Toolchain.LockFile
	if filePath == "" {
		// Default: install_path/toolchain.lock.yaml
		installPath := config.Toolchain.InstallPath
		if installPath == "" {
			installPath = ".tools"
		}
		filePath = filepath.Join(installPath, "toolchain.lock.yaml")
	}

	return &LockFileManager{
		config:   config,
		filePath: filePath,
	}
}

func (m *LockFileManager) Enabled() bool {
	return m.config.Toolchain.UseLockFile
}

func (m *LockFileManager) AddTool(ctx context.Context, tool, version string, opts ...AddOption) error {
	if !m.Enabled() {
		return nil
	}

	cfg := &AddConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Load existing lock file
	lock, err := lockfile.Load(m.filePath)
	if err != nil {
		// Create new if doesn't exist
		lock = lockfile.New()
	}

	// Get or create tool entry
	entry := lock.GetOrCreateTool(tool)
	entry.Version = version

	// Add platform-specific information if provided
	platform := cfg.Platform
	if platform == "" {
		platform = runtime.GOOS + "_" + runtime.GOARCH
	}

	if cfg.URL != "" || cfg.Checksum != "" {
		platformEntry := &lockfile.PlatformEntry{
			URL:      cfg.URL,
			Checksum: cfg.Checksum,
			Size:     cfg.Size,
		}
		entry.Platforms[platform] = platformEntry
	}

	// Save lock file
	return lockfile.Save(m.filePath, lock)
}

func (m *LockFileManager) RemoveTool(ctx context.Context, tool, version string) error {
	if !m.Enabled() {
		return nil
	}

	lock, err := lockfile.Load(m.filePath)
	if err != nil {
		return err
	}

	lock.RemoveTool(tool)
	return lockfile.Save(m.filePath, lock)
}

func (m *LockFileManager) SetDefault(ctx context.Context, tool, version string) error {
	if !m.Enabled() {
		return nil
	}

	// Update tool version in lock file
	return m.AddTool(ctx, tool, version)
}

func (m *LockFileManager) GetTools(ctx context.Context) (map[string][]string, error) {
	if !m.Enabled() {
		return nil, nil
	}

	lock, err := lockfile.Load(m.filePath)
	if err != nil {
		return nil, err
	}

	// Convert lock file format to simple version map
	tools := make(map[string][]string)
	for name, entry := range lock.Tools {
		tools[name] = []string{entry.Version}
	}
	return tools, nil
}

func (m *LockFileManager) Verify(ctx context.Context) error {
	if !m.Enabled() {
		return nil
	}

	return lockfile.Verify(m.filePath)
}

func (m *LockFileManager) Name() string {
	return "lockfile"
}
```

### 4. Registry Coordinator

```go
// pkg/toolchain/filemanager/registry.go
package filemanager

import (
	"context"
	"fmt"

	log "github.com/charmbracelet/log"
)

// Registry coordinates updates across multiple file managers.
type Registry struct {
	managers []FileManager
}

// NewRegistry creates a new file manager registry.
func NewRegistry(managers ...FileManager) *Registry {
	return &Registry{
		managers: managers,
	}
}

// AddTool adds a tool to all enabled managers.
func (r *Registry) AddTool(ctx context.Context, tool, version string, opts ...AddOption) error {
	var errs []error

	for _, mgr := range r.managers {
		if !mgr.Enabled() {
			continue
		}

		if err := mgr.AddTool(ctx, tool, version, opts...); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", mgr.Name(), err))
		} else {
			log.Debug("Updated file", "manager", mgr.Name(), "tool", tool, "version", version)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to update some files: %v", errs)
	}

	return nil
}

// RemoveTool removes a tool from all enabled managers.
func (r *Registry) RemoveTool(ctx context.Context, tool, version string) error {
	var errs []error

	for _, mgr := range r.managers {
		if !mgr.Enabled() {
			continue
		}

		if err := mgr.RemoveTool(ctx, tool, version); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", mgr.Name(), err))
		} else {
			log.Debug("Removed from file", "manager", mgr.Name(), "tool", tool, "version", version)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to update some files: %v", errs)
	}

	return nil
}

// SetDefault sets a tool version as default in all enabled managers.
func (r *Registry) SetDefault(ctx context.Context, tool, version string) error {
	var errs []error

	for _, mgr := range r.managers {
		if !mgr.Enabled() {
			continue
		}

		if err := mgr.SetDefault(ctx, tool, version); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", mgr.Name(), err))
		} else {
			log.Debug("Set default in file", "manager", mgr.Name(), "tool", tool, "version", version)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to update some files: %v", errs)
	}

	return nil
}

// VerifyAll verifies all enabled managers.
func (r *Registry) VerifyAll(ctx context.Context) error {
	var errs []error

	for _, mgr := range r.managers {
		if !mgr.Enabled() {
			continue
		}

		if err := mgr.Verify(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", mgr.Name(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("verification failed: %v", errs)
	}

	return nil
}
```

### 5. Usage in Commands

```go
// toolchain/install.go
package toolchain

import (
	"context"

	"github.com/cloudposse/atmos/pkg/toolchain/filemanager"
)

// Install installs a tool and updates all configured files.
func Install(tool, version string) error {
	ctx := context.Background()

	// Get Atmos configuration
	atmosConfig := GetAtmosConfig()

	// Create file manager registry
	registry := filemanager.NewRegistry(
		filemanager.NewToolVersionsFileManager(atmosConfig),
		filemanager.NewLockFileManager(atmosConfig),
	)

	// Resolve tool to owner/repo format
	owner, repo, err := resolveToolName(tool)
	if err != nil {
		return err
	}
	fullName := owner + "/" + repo

	// Download and install binary
	downloadURL, checksum, size, err := downloadTool(owner, repo, version)
	if err != nil {
		return err
	}

	// Update all files with full metadata
	err = registry.AddTool(ctx, fullName, version,
		filemanager.WithURL(downloadURL),
		filemanager.WithChecksum(checksum),
		filemanager.WithSize(size),
	)
	if err != nil {
		return fmt.Errorf("failed to update files: %w", err)
	}

	fmt.Printf("✓ Installed %s@%s\n", fullName, version)
	return nil
}
```

```go
// toolchain/remove.go
package toolchain

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/toolchain/filemanager"
)

// Remove removes a tool version and updates all configured files.
func Remove(tool, version string) error {
	ctx := context.Background()

	// Get Atmos configuration
	atmosConfig := GetAtmosConfig()

	// Create file manager registry
	registry := filemanager.NewRegistry(
		filemanager.NewToolVersionsFileManager(atmosConfig),
		filemanager.NewLockFileManager(atmosConfig),
	)

	// Resolve tool name
	owner, repo, err := resolveToolName(tool)
	if err != nil {
		return err
	}
	fullName := owner + "/" + repo

	// Remove from binary installation
	if err := removeBinary(owner, repo, version); err != nil {
		return err
	}

	// Update all files
	err = registry.RemoveTool(ctx, fullName, version)
	if err != nil {
		return fmt.Errorf("failed to update files: %w", err)
	}

	fmt.Printf("✓ Removed %s@%s\n", fullName, version)
	return nil
}
```

```go
// toolchain/set.go
package toolchain

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/toolchain/filemanager"
)

// Set sets a tool version as default and updates all configured files.
func Set(tool, version string) error {
	ctx := context.Background()

	atmosConfig := GetAtmosConfig()

	registry := filemanager.NewRegistry(
		filemanager.NewToolVersionsFileManager(atmosConfig),
		filemanager.NewLockFileManager(atmosConfig),
	)

	owner, repo, err := resolveToolName(tool)
	if err != nil {
		return err
	}
	fullName := owner + "/" + repo

	// Set as default in all files
	err = registry.SetDefault(ctx, fullName, version)
	if err != nil {
		return fmt.Errorf("failed to update files: %w", err)
	}

	fmt.Printf("✓ Set %s@%s as default\n", fullName, version)
	return nil
}
```

## Benefits

### 1. **DRY Principle**
- File update logic in one place per file type
- Commands just call registry methods
- No duplicate code

### 2. **Configuration-Driven**
- `use_tool_versions: true/false` - Enable/disable .tool-versions
- `use_lock_file: true/false` - Enable/disable lock file
- Managers automatically skip if disabled

### 3. **Testable**
- Mock `FileManager` interface for unit tests
- Test each manager independently
- Test registry coordination separately

### 4. **Extensible**
- Add new file types by implementing `FileManager`
- No changes to existing commands
- Registry automatically coordinates all managers

### 5. **Type-Safe**
- Options pattern for flexible metadata
- Interface ensures consistent behavior
- Compile-time checks

## Example: Adding a New File Type

Want to add support for `.tools.json` format?

```go
// pkg/toolchain/filemanager/jsonfile.go
package filemanager

type JSONFileManager struct {
	config   *schema.AtmosConfiguration
	filePath string
}

func NewJSONFileManager(config *schema.AtmosConfiguration) *JSONFileManager {
	return &JSONFileManager{
		config:   config,
		filePath: config.Toolchain.JSONFile,
	}
}

func (m *JSONFileManager) Enabled() bool {
	return m.config.Toolchain.UseJSONFile
}

// Implement other FileManager interface methods...
```

Then in commands:
```go
registry := filemanager.NewRegistry(
	filemanager.NewToolVersionsFileManager(atmosConfig),
	filemanager.NewLockFileManager(atmosConfig),
	filemanager.NewJSONFileManager(atmosConfig),  // Just add it!
)
```

## Testing Strategy

### Unit Tests (Manager Level)

```go
func TestToolVersionsFileManager_AddTool(t *testing.T) {
	// Create temp file
	tmpFile := filepath.Join(t.TempDir(), ".tool-versions")

	mgr := &ToolVersionsFileManager{
		config: &schema.AtmosConfiguration{
			Toolchain: schema.Toolchain{
				UseToolVersions: true,
				VersionsFile:    tmpFile,
			},
		},
		filePath: tmpFile,
	}

	// Test add
	err := mgr.AddTool(context.Background(), "terraform", "1.13.4")
	assert.NoError(t, err)

	// Verify file contents
	content, _ := os.ReadFile(tmpFile)
	assert.Contains(t, string(content), "terraform 1.13.4")
}
```

### Integration Tests (Registry Level)

```go
func TestRegistry_AddTool(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseToolVersions: true,
			UseLockFile:     true,
			VersionsFile:    filepath.Join(tmpDir, ".tool-versions"),
			LockFile:        filepath.Join(tmpDir, "toolchain.lock.yaml"),
		},
	}

	registry := filemanager.NewRegistry(
		filemanager.NewToolVersionsFileManager(atmosConfig),
		filemanager.NewLockFileManager(atmosConfig),
	)

	// Add tool
	err := registry.AddTool(context.Background(), "hashicorp/terraform", "1.13.4",
		filemanager.WithChecksum("sha256:abc123"),
		filemanager.WithURL("https://example.com/terraform.zip"),
	)
	assert.NoError(t, err)

	// Both files should be updated
	toolVersions, _ := os.ReadFile(filepath.Join(tmpDir, ".tool-versions"))
	assert.Contains(t, string(toolVersions), "terraform 1.13.4")

	lockFile, _ := os.ReadFile(filepath.Join(tmpDir, "toolchain.lock.yaml"))
	assert.Contains(t, string(lockFile), "sha256:abc123")
}
```

## Migration Path

### Current State (No Pattern)
```go
// toolchain/install.go
AddToolToVersions(path, tool, version)

// toolchain/remove.go
RemoveToolFromVersions(path, tool, version)
// No lock file update!
```

### Step 1: Create Interfaces
- Define `FileManager` interface
- Implement `ToolVersionsFileManager`
- No behavior changes yet

### Step 2: Add Registry
- Create `Registry` type
- Update `install` to use registry
- Verify backward compatibility

### Step 3: Update All Commands
- Update `remove`, `set`, `uninstall`
- All commands use registry pattern
- Consistent behavior

### Step 4: Add Lock File
- Implement `LockFileManager`
- Add to registry
- Feature complete!

## Conclusion

This pattern provides:
- ✅ **Consistency** - All commands update files the same way
- ✅ **Flexibility** - Easy to enable/disable file types
- ✅ **Maintainability** - Single source of truth per file type
- ✅ **Testability** - Mock interfaces, test isolation
- ✅ **Extensibility** - Add new file types without changing commands

The **Go way**: Interfaces, composition, and dependency injection.
