package manager

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Permission for directories created to hold the lock file.
	lockDirPerm os.FileMode = 0o755
	// Permission for the lock file. The lock is a non-sensitive project file
	// committed to version control, so it stays world-readable like other
	// lockfiles.
	lockFilePerm os.FileMode = 0o644
)

// LockFile is the on-disk versions.lock.yaml format.
type LockFile struct {
	Version int                             `yaml:"version" json:"version"`
	Tracks  map[string]map[string]LockEntry `yaml:"tracks" json:"tracks"`
}

// LockEntry is one resolved version in the lock file.
type LockEntry struct {
	Version    string `yaml:"version" json:"version"`
	Ecosystem  string `yaml:"ecosystem,omitempty" json:"ecosystem,omitempty"`
	Datasource string `yaml:"datasource,omitempty" json:"datasource,omitempty"`
	Provider   string `yaml:"provider,omitempty" json:"provider,omitempty"`
	Package    string `yaml:"package,omitempty" json:"package,omitempty"`
	Digest     string `yaml:"digest,omitempty" json:"digest,omitempty"`
	ResolvedAt string `yaml:"resolved_at,omitempty" json:"resolved_at,omitempty"`
	// ReleasedAt is the upstream release timestamp when the datasource
	// provides one; used by update cooldown checks.
	ReleasedAt string `yaml:"released_at,omitempty" json:"released_at,omitempty"`
}

// LockFilePath returns the absolute lock file path.
func LockFilePath(atmosConfig *schema.AtmosConfiguration) string {
	defer perf.Track(atmosConfig, "manager.LockFilePath")()

	lockFile := DefaultLockFile
	basePath := ""
	if atmosConfig != nil {
		if atmosConfig.Version.LockFile != "" {
			lockFile = atmosConfig.Version.LockFile
		}
		basePath = atmosConfig.BasePath
		if basePath == "" {
			basePath = atmosConfig.CliConfigPath
		}
	}
	if filepath.IsAbs(lockFile) {
		return lockFile
	}
	if basePath == "" {
		wd, err := os.Getwd()
		if err == nil {
			basePath = wd
		}
	}
	return filepath.Join(basePath, lockFile)
}

// LoadLock reads the lock file. A missing lock file returns an empty lock.
func LoadLock(atmosConfig *schema.AtmosConfiguration) (*LockFile, error) {
	defer perf.Track(atmosConfig, "manager.LoadLock")()

	lockPath := LockFilePath(atmosConfig)
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyLock(), nil
		}
		return nil, err
	}
	var lock LockFile
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, err
	}
	if lock.Version == 0 {
		lock.Version = lockVersion
	}
	if lock.Tracks == nil {
		lock.Tracks = map[string]map[string]LockEntry{}
	}
	return &lock, nil
}

// SaveLock writes the lock file.
func SaveLock(atmosConfig *schema.AtmosConfiguration, lock *LockFile) error {
	defer perf.Track(atmosConfig, "manager.SaveLock")()

	if lock == nil {
		lock = emptyLock()
	}
	if lock.Version == 0 {
		lock.Version = lockVersion
	}
	if lock.Tracks == nil {
		lock.Tracks = map[string]map[string]LockEntry{}
	}
	lockPath := LockFilePath(atmosConfig)
	if err := os.MkdirAll(filepath.Dir(lockPath), lockDirPerm); err != nil {
		return err
	}
	data, err := yaml.Marshal(lock)
	if err != nil {
		return err
	}
	return os.WriteFile(lockPath, data, lockFilePerm) // #nosec G306 -- the lock file is a non-sensitive, committed project file.
}

// ResolveLocked resolves a version name from the lock file.
func ResolveLocked(atmosConfig *schema.AtmosConfiguration, track, name string) (string, error) {
	defer perf.Track(atmosConfig, "manager.ResolveLocked")()

	track = EffectiveTrack(atmosConfig, track)
	lock, err := LoadLock(atmosConfig)
	if err != nil {
		return "", err
	}
	entries := lock.Tracks[track]
	if entries == nil {
		return "", fmt.Errorf("%w: track %s", ErrVersionNotLocked, track)
	}
	entry, ok := entries[name]
	if !ok || entry.Version == "" {
		return "", fmt.Errorf("%w: %s in track %s", ErrVersionNotLocked, name, track)
	}
	return entry.Version, nil
}

// VersionMap returns a map usable as template context at .version. Each value
// is a VersionRef whose String() form honors the entry's pin policy, so
// `{{ .version.name }}` renders the digest for pinned entries while
// `.Version` and `.Digest` stay individually addressable.
func VersionMap(atmosConfig *schema.AtmosConfiguration, track string) (map[string]VersionRef, error) {
	defer perf.Track(atmosConfig, "manager.VersionMap")()

	track = EffectiveTrack(atmosConfig, track)
	lock, err := LoadLock(atmosConfig)
	if err != nil {
		return nil, err
	}
	// Pin preferences come from configuration; tolerate lock entries whose
	// track is no longer configured (they resolve with pin "none").
	configured, err := EffectiveEntries(atmosConfig, track)
	if err != nil && !errors.Is(err, ErrTrackNotFound) {
		return nil, err
	}
	lockedEntries := lock.Tracks[track]
	result := map[string]VersionRef{}
	for name := range lockedEntries {
		entry := lockedEntries[name]
		ref := VersionRef{Version: entry.Version, Digest: entry.Digest}
		if configuredEntry, ok := configured[name]; ok {
			ref.Pin = normalizePin(configuredEntry.Update.Pin)
		}
		result[name] = ref
	}
	return result, nil
}

// LockTrack resolves and writes all entries in a track.
func LockTrack(atmosConfig *schema.AtmosConfiguration, track, group string) (*LockFile, error) {
	defer perf.Track(atmosConfig, "manager.LockTrack")()

	track = EffectiveTrack(atmosConfig, track)
	entries, err := EffectiveEntries(atmosConfig, track)
	if err != nil {
		return nil, err
	}
	lock, err := LoadLock(atmosConfig)
	if err != nil {
		return nil, err
	}
	if lock.Tracks[track] == nil {
		lock.Tracks[track] = map[string]LockEntry{}
	}
	for _, name := range sortedNames(entries) {
		entry := entries[name]
		if group != "" && entry.Group != group {
			continue
		}
		candidate, err := ResolveEntry(atmosConfig, &entry, pinEnabled(&entry))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name, err)
		}
		lockEntry := LockEntry{
			Version:    candidate.Version,
			Ecosystem:  entry.Ecosystem,
			Datasource: entry.Datasource,
			Provider:   entry.Provider,
			Package:    entry.Package,
			Digest:     candidate.Digest,
			ResolvedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if candidate.ReleasedAt != nil {
			lockEntry.ReleasedAt = candidate.ReleasedAt.UTC().Format(time.RFC3339)
		}
		lock.Tracks[track][entry.Name] = lockEntry
	}
	return lock, SaveLock(atmosConfig, lock)
}

func emptyLock() *LockFile {
	return &LockFile{
		Version: lockVersion,
		Tracks:  map[string]map[string]LockEntry{},
	}
}
