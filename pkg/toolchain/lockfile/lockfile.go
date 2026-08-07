package lockfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// DirPermissions is the default permission for directories.
	dirPermissions = 0o755
	// FilePermissions is the default permission for files.
	filePermissions = 0o644
)

// Error definitions for the lockfile package.
var (
	// ErrInvalidLockFile indicates the lock file is malformed or missing required fields.
	ErrInvalidLockFile = errors.New("invalid lock file")

	// ErrLockFileNil indicates the lock file is nil.
	ErrLockFileNil = errors.New("lock file is nil")

	// ErrToolMissingVersion indicates a tool entry has no version entries.
	ErrToolMissingVersion = errors.New("tool missing version")

	// ErrToolNoPlatforms indicates a version entry has no platform entries.
	ErrToolNoPlatforms = errors.New("tool has no platform entries")

	// ErrPlatformNoURL indicates a platform entry is missing the URL field.
	ErrPlatformNoURL = errors.New("platform entry missing URL")

	// ErrPlatformNoChecksum indicates a platform entry is missing the checksum field.
	ErrPlatformNoChecksum = errors.New("platform entry missing checksum")

	// ErrToolEntryNil indicates a tool entry in the lock file is nil.
	ErrToolEntryNil = errors.New("tool entry is nil")

	// ErrVersionEntryNil indicates a version entry in the lock file is nil.
	ErrVersionEntryNil = errors.New("version entry is nil")

	// ErrPlatformEntryNil indicates a platform entry in the lock file is nil.
	ErrPlatformEntryNil = errors.New("platform entry is nil")
)

// LockFile represents the toolchain.lock.yaml structure.
type LockFile struct {
	Version  int              `yaml:"version"`
	Tools    map[string]*Tool `yaml:"tools"`
	Metadata LockFileMetadata `yaml:"metadata"`
}

// Tool represents a locked tool, keyed by owner/repo. Versions holds one entry per pinned
// version of this tool (keyed by version string) -- a .tool-versions line can legitimately pin
// more than one version of the same tool (e.g. "yq 4.45.1 4.50.1"), so each version's checksum
// data must be tracked independently rather than overwriting a single flat entry.
type Tool struct {
	Versions map[string]*VersionEntry `yaml:"versions"`
}

// VersionEntry represents a single locked version of a tool.
type VersionEntry struct {
	Source      string                    `yaml:"source,omitempty"`
	Platforms   map[string]*PlatformEntry `yaml:"platforms"`
	BinaryName  string                    `yaml:"binary_name,omitempty"`
	InstalledAt string                    `yaml:"installed_at"` // RFC3339
}

// PlatformEntry represents platform-specific download information.
type PlatformEntry struct {
	URL               string   `yaml:"url"`
	Checksum          string   `yaml:"checksum"`
	Size              int64    `yaml:"size"`
	ChecksumAlgorithm string   `yaml:"checksum_algorithm,omitempty"`
	Verification      []string `yaml:"verification,omitempty"`
}

// LockFileMetadata contains metadata about the lock file.
type LockFileMetadata struct {
	GeneratedAt     string `yaml:"generated_at"` // RFC3339
	AtmosVersion    string `yaml:"atmos_version"`
	Platform        string `yaml:"platform"` // GOOS_GOARCH
	LockFileVersion int    `yaml:"lock_file_version"`
}

// New creates a new empty lock file.
func New() *LockFile {
	defer perf.Track(nil, "lockfile.New")()

	return &LockFile{
		Version: 1,
		Tools:   make(map[string]*Tool),
		Metadata: LockFileMetadata{
			GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
			AtmosVersion:    atmosVersion(),
			Platform:        runtime.GOOS + "_" + runtime.GOARCH,
			LockFileVersion: currentLockFileVersion,
		},
	}
}

// currentLockFileVersion bumped from 1 to 2 with the move from a single flat Version/Platforms
// pair per tool to a nested per-version Versions map, so a tool pinned to multiple versions
// (e.g. "yq 4.45.1 4.50.1") can have each version's checksum data recorded independently instead
// of the second locked version silently overwriting the first's.
const currentLockFileVersion = 2

// Load loads a lock file from disk.
func Load(filePath string) (*LockFile, error) {
	defer perf.Track(nil, "lockfile.Load")()

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read lock file: %w", err)
	}

	var lockFile LockFile
	if err := yaml.Unmarshal(data, &lockFile); err != nil {
		return nil, fmt.Errorf("failed to parse lock file: %w", err)
	}

	// Validate
	if lockFile.Version == 0 {
		return nil, fmt.Errorf("%w: missing version", ErrInvalidLockFile)
	}

	return &lockFile, nil
}

// Save writes the lock file to disk, updating metadata fields and creating parent directories if needed.
// The function updates GeneratedAt, AtmosVersion, and Platform fields in the lockfile metadata to reflect
// the current environment. It creates the parent directory structure if it doesn't exist (with 0o755 permissions)
// and writes the lockfile with 0o644 permissions. Returns an error if the lockfile is nil, marshaling fails,
// directory creation fails, or file writing fails.
func Save(filePath string, lockFile *LockFile) error {
	defer perf.Track(nil, "lockfile.Save")()

	if lockFile == nil {
		return ErrLockFileNil
	}

	// Update metadata
	lockFile.Metadata.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	lockFile.Metadata.AtmosVersion = atmosVersion()
	lockFile.Metadata.Platform = runtime.GOOS + "_" + runtime.GOARCH

	data, err := yaml.Marshal(lockFile)
	if err != nil {
		return fmt.Errorf("failed to marshal lock file: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Add header comment
	header := `# Generated by Atmos Toolchain
# This file locks tool versions and checksums for reproducible installations
# Do not edit manually - run 'atmos toolchain lock' to update

`
	content := header + string(data)

	if err := os.WriteFile(filePath, []byte(content), filePermissions); err != nil {
		return fmt.Errorf("failed to write lock file: %w", err)
	}

	return nil
}

// atmosVersion reads build metadata without importing pkg/version. Keeping the
// lock-format package below the toolchain installer avoids an import cycle and
// lets every writer use this canonical model.
func atmosVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "unknown"
}

// GetOrCreateTool gets or creates a tool entry in the lock file.
func (lf *LockFile) GetOrCreateTool(tool string) *Tool {
	defer perf.Track(nil, "lockfile.LockFile.GetOrCreateTool")()

	if lf.Tools == nil {
		lf.Tools = make(map[string]*Tool)
	}

	if entry, exists := lf.Tools[tool]; exists {
		if entry.Versions == nil {
			entry.Versions = make(map[string]*VersionEntry)
		}
		return entry
	}

	entry := &Tool{Versions: make(map[string]*VersionEntry)}
	lf.Tools[tool] = entry
	return entry
}

// GetOrCreateVersion gets or creates the entry for a specific version of this tool. Locking or
// installing a second version of an already-locked tool must not disturb any other version's
// recorded data -- each version's checksum/platform data lives in its own entry.
func (t *Tool) GetOrCreateVersion(version string) *VersionEntry {
	defer perf.Track(nil, "lockfile.Tool.GetOrCreateVersion")()

	if t.Versions == nil {
		t.Versions = make(map[string]*VersionEntry)
	}

	if entry, exists := t.Versions[version]; exists {
		if entry.Platforms == nil {
			entry.Platforms = make(map[string]*PlatformEntry)
		}
		return entry
	}

	entry := &VersionEntry{
		Platforms:   make(map[string]*PlatformEntry),
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}
	t.Versions[version] = entry
	return entry
}

// RemoveTool removes a tool (all of its locked versions) from the lock file.
func (lf *LockFile) RemoveTool(tool string) {
	defer perf.Track(nil, "lockfile.LockFile.RemoveTool")()

	delete(lf.Tools, tool)
}

// RemoveVersion removes a single locked version of this tool.
func (t *Tool) RemoveVersion(version string) {
	defer perf.Track(nil, "lockfile.Tool.RemoveVersion")()

	delete(t.Versions, version)
}

// Verify verifies the integrity and validity of the lock file at the specified path.
// It loads the lockfile from disk and performs comprehensive validation including:
// version field checks, metadata validation, and per-tool validation (version presence,
// platform entries existence, and platform-specific URL/checksum requirements).
// Returns nil if the lockfile is valid, or an error describing the validation failure
// (e.g., missing required fields, nil entries, empty values).
func Verify(filePath string) error {
	defer perf.Track(nil, "lockfile.Verify")()

	lockFile, err := Load(filePath)
	if err != nil {
		return err
	}

	// Basic validation.
	if lockFile.Version == 0 {
		return fmt.Errorf("%w: version is 0", ErrInvalidLockFile)
	}

	if lockFile.Metadata.LockFileVersion == 0 {
		return fmt.Errorf("%w: metadata version is 0", ErrInvalidLockFile)
	}

	// Validate each tool entry.
	for toolName, tool := range lockFile.Tools {
		if err := validateToolEntry(toolName, tool); err != nil {
			return err
		}
	}

	return nil
}

// validateToolEntry validates a single tool entry in the lock file.
func validateToolEntry(toolName string, tool *Tool) error {
	// Guard against nil entries.
	if tool == nil {
		return fmt.Errorf("%w: %s", ErrToolEntryNil, toolName)
	}

	if len(tool.Versions) == 0 {
		return fmt.Errorf("%w: %s", ErrToolMissingVersion, toolName)
	}

	for version, entry := range tool.Versions {
		if err := validateVersionEntry(toolName, version, entry); err != nil {
			return err
		}
	}

	return nil
}

// validateVersionEntry validates a single locked version of a tool.
func validateVersionEntry(toolName, version string, entry *VersionEntry) error {
	if entry == nil {
		return fmt.Errorf("%w: %s@%s", ErrVersionEntryNil, toolName, version)
	}

	if len(entry.Platforms) == 0 {
		return fmt.Errorf("%w: %s@%s", ErrToolNoPlatforms, toolName, version)
	}

	for platform, platformEntry := range entry.Platforms {
		if err := validatePlatformEntry(toolName, platform, platformEntry); err != nil {
			return err
		}
	}

	return nil
}

// validatePlatformEntry validates a single platform entry for a tool.
func validatePlatformEntry(toolName, platform string, entry *PlatformEntry) error {
	// Guard against nil platform entries.
	if entry == nil {
		return fmt.Errorf("%w: %s/%s", ErrPlatformEntryNil, toolName, platform)
	}
	if entry.URL == "" {
		return fmt.Errorf("%w: %s/%s", ErrPlatformNoURL, toolName, platform)
	}
	if entry.Checksum == "" {
		return fmt.Errorf("%w: %s/%s", ErrPlatformNoChecksum, toolName, platform)
	}
	return nil
}
