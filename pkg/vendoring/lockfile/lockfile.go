// Package lockfile stores the immutable identities and installed-file inventory
// produced by Atmos vendoring.
//
//nolint:cyclop,err113,gocognit,gocritic,gosec,lintroller,nestif,nilerr,revive // Lock transactions keep ownership, verification, and atomicity in one auditable unit.
package lockfile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendor"
)

const (
	// DefaultFileName is the default committed vendor lock file.
	DefaultFileName = "vendor.lock.yaml"
	lockVersion     = 1
	fileMode        = 0o644
	directoryMode   = 0o755
)

// LockFile is the versioned vendor.lock.yaml format.
type LockFile struct {
	Version   int                 `yaml:"version" json:"version"`
	Artifacts map[string]Artifact `yaml:"artifacts" json:"artifacts"`
}

// Artifact is one independently materialized source, target, or mixin.
type Artifact struct {
	Component string `yaml:"component,omitempty" json:"component,omitempty"`
	Kind      string `yaml:"kind" json:"kind"`
	Target    string `yaml:"target" json:"target"`
	Source    Source `yaml:"source" json:"source"`
	Files     []File `yaml:"files" json:"files"`
	Order     int    `yaml:"order" json:"order"`
}

// Source identifies the immutable artifact that was installed.
type Source struct {
	Declared     string `yaml:"declared" json:"declared"`
	Resolved     string `yaml:"resolved,omitempty" json:"resolved,omitempty"`
	Digest       string `yaml:"digest,omitempty" json:"digest,omitempty"`
	ETag         string `yaml:"etag,omitempty" json:"etag,omitempty"`
	LastModified string `yaml:"last_modified,omitempty" json:"last_modified,omitempty"`
}

// File records one lock-owned output path. SHA256 is the file bytes, or the
// symlink target text for a symlink.
type File struct {
	Path   string `yaml:"path" json:"path"`
	Type   string `yaml:"type" json:"type"`
	Mode   uint32 `yaml:"mode" json:"mode"`
	SHA256 string `yaml:"sha256" json:"sha256"`
}

// Drift reports a missing or changed lock-owned file.
type Drift struct {
	Artifact string
	Path     string
	Reason   string
}

// CleanReport summarizes a vendor clean operation.
type CleanReport struct {
	Removed   []string
	Conflicts []Drift
}

// Path returns the absolute vendor lock path for the given Atmos configuration.
func Path(config *schema.AtmosConfiguration) string {
	lockPath := DefaultFileName
	basePath := ""
	if config != nil {
		if config.Vendor.LockFile != "" {
			lockPath = config.Vendor.LockFile
		}
		basePath = config.BasePath
		if basePath == "" {
			basePath = config.CliConfigPath
		}
	}
	if filepath.IsAbs(lockPath) {
		return lockPath
	}
	if basePath == "" {
		basePath, _ = os.Getwd()
	}
	return filepath.Join(basePath, lockPath)
}

// New returns an empty lock file.
func New() *LockFile {
	return &LockFile{Version: lockVersion, Artifacts: map[string]Artifact{}}
}

// Load returns an empty lock when no lock file has been created yet.
func Load(config *schema.AtmosConfiguration) (*LockFile, error) {
	data, err := os.ReadFile(Path(config))
	if os.IsNotExist(err) {
		return New(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read vendor lock: %w", err)
	}
	lock := New()
	if err := yaml.Unmarshal(data, lock); err != nil {
		return nil, fmt.Errorf("parse vendor lock: %w", err)
	}
	if lock.Version != lockVersion {
		return nil, fmt.Errorf("unsupported vendor lock version %d", lock.Version)
	}
	if lock.Artifacts == nil {
		lock.Artifacts = map[string]Artifact{}
	}
	return lock, nil
}

// Save atomically replaces the committed lock with deterministic YAML.
func Save(config *schema.AtmosConfiguration, lock *LockFile) error {
	if lock == nil {
		lock = New()
	}
	if lock.Version == 0 {
		lock.Version = lockVersion
	}
	if lock.Artifacts == nil {
		lock.Artifacts = map[string]Artifact{}
	}
	for id, artifact := range lock.Artifacts {
		target, targetErr := projectRelativeTarget(config, artifact.Target)
		if targetErr != nil {
			return fmt.Errorf("normalize lock target for artifact %q: %w", id, targetErr)
		}
		artifact.Target = target
		artifact.Source.Declared = RedactSource(artifact.Source.Declared)
		artifact.Source.Resolved = RedactSource(artifact.Source.Resolved)
		sort.Slice(artifact.Files, func(i, j int) bool { return artifact.Files[i].Path < artifact.Files[j].Path })
		lock.Artifacts[id] = artifact
	}
	data, err := yaml.Marshal(lock)
	if err != nil {
		return fmt.Errorf("marshal vendor lock: %w", err)
	}
	lockPath := Path(config)
	if err := os.MkdirAll(filepath.Dir(lockPath), directoryMode); err != nil {
		return fmt.Errorf("create vendor lock directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(lockPath), ".vendor.lock-*")
	if err != nil {
		return fmt.Errorf("create temporary vendor lock: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary vendor lock: %w", err)
	}
	if err := tmp.Chmod(fileMode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set vendor lock permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary vendor lock: %w", err)
	}
	if err := os.Rename(tmpName, lockPath); err != nil {
		return fmt.Errorf("replace vendor lock: %w", err)
	}
	return nil
}

// Inventory records every regular file and symlink below root. Paths are always
// relative and slash-normalized so the lock is portable across operating systems.
func Inventory(root string) ([]File, error) {
	return inventory(root, false)
}

// VendorInventory records the files that the standard directory vendoring
// copy writes. Git metadata is deliberately excluded because the installer
// never copies it to the materialized target.
func VendorInventory(root string) ([]File, error) {
	return inventory(root, true)
}

// VendorInventoryWithPatterns records the exact files selected by the shared
// vendor copy policy. It allows component.yaml sources to own only copied
// files, rather than taking a broad snapshot of a component directory that
// may also contain local configuration or mixin output.
func VendorInventoryWithPatterns(root string, includedPaths, excludedPaths []string) ([]File, error) {
	files := []File{}
	skip := vendor.CreateSkipFunc(root, includedPaths, excludedPaths)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		shouldSkip, err := skip(info, path, "")
		if err != nil {
			return err
		}
		if shouldSkip {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		file := File{Path: filepath.ToSlash(rel), Mode: uint32(info.Mode().Perm())}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			file.Type, file.SHA256 = "symlink", hashString(target)
		} else {
			digest, err := hashFile(path)
			if err != nil {
				return err
			}
			file.Type, file.SHA256 = "file", digest
		}
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("inventory patterned vendor source %q: %w", root, err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func inventory(root string, skipGit bool) ([]File, error) {
	files := []File{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root || entry.IsDir() {
			if skipGit && entry.IsDir() && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		file := File{Path: filepath.ToSlash(rel), Mode: uint32(info.Mode().Perm())}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			file.Type, file.SHA256 = "symlink", hashString(target)
		} else {
			digest, err := hashFile(path)
			if err != nil {
				return err
			}
			file.Type, file.SHA256 = "file", digest
		}
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("inventory vendor target %q: %w", root, err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// FilesDigest returns the deterministic SHA-256 identity for an installation
// manifest. It is used only when a source has no stronger immutable identity.
func FilesDigest(files []File) string {
	copyFiles := append([]File(nil), files...)
	sort.Slice(copyFiles, func(i, j int) bool { return copyFiles[i].Path < copyFiles[j].Path })
	hash := sha256.New()
	for _, file := range copyFiles {
		_, _ = fmt.Fprintf(hash, "%s\x00%s\x00%o\x00%s\n", file.Path, file.Type, file.Mode, file.SHA256)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

// ArtifactID returns a stable key for the writer that owns a target. The
// declared source is deliberately not part of the key: a new declared ref for
// the same target replaces the old receipt and can safely prune its stale files.
func ArtifactID(kind, target string, writers ...string) string {
	identity := strings.Join(append([]string{kind, filepath.Clean(target)}, writers...), "\x00")
	hash := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(hash[:])
}

// IsMaterialized reports whether a declared source has a complete, unchanged
// installation receipt at target. It never treats cache metadata as evidence
// of integrity.
func IsMaterialized(config *schema.AtmosConfiguration, id, declared, target string) (bool, error) {
	lock, err := Load(config)
	if err != nil {
		return false, err
	}
	lockTarget, err := projectRelativeTarget(config, target)
	if err != nil {
		return false, err
	}
	artifact, found := lock.Artifacts[id]
	if !found || artifact.Target != lockTarget || artifact.Source.Declared != RedactSource(declared) {
		return false, nil
	}
	for _, file := range artifact.Files {
		path, pathErr := lockedPath(config, artifact.Target, file.Path)
		if pathErr != nil {
			return false, pathErr
		}
		info, statErr := os.Lstat(path)
		if statErr != nil || !matches(file, path, info) {
			return false, nil
		}
	}
	return true, nil
}

// Replace atomically updates one artifact receipt after its files have been
// materialized. It safely prunes stale, unchanged files from the previous
// receipt while retaining files claimed by another artifact.
func Replace(config *schema.AtmosConfiguration, id string, artifact Artifact) error {
	target, err := projectRelativeTarget(config, artifact.Target)
	if err != nil {
		return fmt.Errorf("normalize artifact target: %w", err)
	}
	artifact.Target = target
	lock, err := Load(config)
	if err != nil {
		return err
	}
	previous, hadPrevious := lock.Artifacts[id]
	if hadPrevious {
		artifact.Order = previous.Order
	} else {
		artifact.Order = nextOrder(lock)
	}
	newPaths := map[string]struct{}{}
	for _, file := range artifact.Files {
		path, pathErr := lockedPath(config, artifact.Target, file.Path)
		if pathErr != nil {
			return pathErr
		}
		newPaths[path] = struct{}{}
	}
	if hadPrevious {
		for _, file := range previous.Files {
			path, pathErr := lockedPath(config, previous.Target, file.Path)
			if pathErr != nil {
				return pathErr
			}
			if _, retained := newPaths[path]; retained || ownedByAnother(config, lock, id, path) {
				continue
			}
			info, statErr := os.Lstat(path)
			if os.IsNotExist(statErr) {
				continue
			}
			if statErr != nil {
				return fmt.Errorf("inspect stale lock-owned file %q: %w", path, statErr)
			}
			if !matches(file, path, info) {
				return fmt.Errorf("stale lock-owned file %q was modified", path)
			}
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove stale lock-owned file %q: %w", path, err)
			}
			previousRoot, rootErr := lockTargetRoot(config, previous.Target)
			if rootErr != nil {
				return rootErr
			}
			removeEmptyParents(filepath.Dir(path), previousRoot)
		}
	}
	lock.Artifacts[id] = artifact
	return Save(config, lock)
}

// Verify compares lock-owned files with the current target tree and rejects
// lock targets that escape the configured project root.
func Verify(config *schema.AtmosConfiguration, lock *LockFile) ([]Drift, error) {
	if lock == nil {
		return nil, nil
	}
	var drifts []Drift
	for id, artifact := range lock.Artifacts {
		for _, file := range artifact.Files {
			path, pathErr := lockedPath(config, artifact.Target, file.Path)
			if pathErr != nil {
				return nil, pathErr
			}
			info, err := os.Lstat(path)
			if os.IsNotExist(err) {
				drifts = append(drifts, Drift{Artifact: id, Path: path, Reason: "missing"})
				continue
			}
			if err != nil {
				drifts = append(drifts, Drift{Artifact: id, Path: path, Reason: err.Error()})
				continue
			}
			var digest string
			if file.Type == "symlink" && info.Mode()&os.ModeSymlink != 0 {
				target, readErr := os.Readlink(path)
				if readErr == nil {
					digest = hashString(target)
				}
			} else if info.Mode().IsRegular() {
				digest, _ = hashFile(path)
			}
			if digest == "" || digest != file.SHA256 {
				drifts = append(drifts, Drift{Artifact: id, Path: path, Reason: "checksum mismatch"})
			}
		}
	}
	return drifts, nil
}

// Clean removes files owned by selected lock artifacts. A blank component
// selects every artifact. Modified files are preserved unless force is true.
// The lock is updated only when every selected artifact was removed cleanly.
func Clean(config *schema.AtmosConfiguration, component string, force, dryRun bool) (*CleanReport, error) {
	lock, err := Load(config)
	if err != nil {
		return nil, err
	}
	report := &CleanReport{}
	selected, remainingOwners, err := selectCleanArtifacts(config, lock, component)
	if err != nil {
		return nil, err
	}
	if err := validateCleanArtifacts(config, selected, remainingOwners, force, report); err != nil {
		return nil, err
	}
	if len(report.Conflicts) > 0 {
		return report, nil
	}
	if err := removeCleanArtifacts(config, selected, remainingOwners, dryRun, report); err != nil {
		return nil, err
	}
	if !dryRun {
		if err := removeCleanArtifactsFromLock(config, lock, selected); err != nil {
			return nil, err
		}
	}
	return report, nil
}

func selectCleanArtifacts(config *schema.AtmosConfiguration, lock *LockFile, component string) (map[string]Artifact, map[string]struct{}, error) {
	selected := map[string]Artifact{}
	remainingOwners := map[string]struct{}{}
	for id, artifact := range lock.Artifacts {
		if component == "" || artifact.Component == component {
			selected[id] = artifact
			continue
		}
		if err := collectArtifactOwners(config, artifact, remainingOwners); err != nil {
			return nil, nil, err
		}
	}
	return selected, remainingOwners, nil
}

func collectArtifactOwners(config *schema.AtmosConfiguration, artifact Artifact, owners map[string]struct{}) error {
	for _, file := range artifact.Files {
		path, err := lockedPath(config, artifact.Target, file.Path)
		if err != nil {
			return err
		}
		owners[path] = struct{}{}
	}
	return nil
}

// validateCleanArtifacts validates every selected path before changing either
// the filesystem or the lock. This keeps a conflict from producing a partial receipt.
func validateCleanArtifacts(config *schema.AtmosConfiguration, selected map[string]Artifact, remainingOwners map[string]struct{}, force bool, report *CleanReport) error {
	for id, artifact := range selected {
		for _, file := range artifact.Files {
			path, err := lockedPath(config, artifact.Target, file.Path)
			if err != nil {
				return err
			}
			if _, shared := remainingOwners[path]; shared {
				continue
			}
			info, err := os.Lstat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("inspect lock-owned file %q: %w", path, err)
			}
			if !force && !matches(file, path, info) {
				report.Conflicts = append(report.Conflicts, Drift{Artifact: id, Path: path, Reason: "checksum mismatch"})
			}
		}
	}
	return nil
}

func removeCleanArtifacts(config *schema.AtmosConfiguration, selected map[string]Artifact, remainingOwners map[string]struct{}, dryRun bool, report *CleanReport) error {
	for _, artifact := range selected {
		for _, file := range artifact.Files {
			path, err := lockedPath(config, artifact.Target, file.Path)
			if err != nil {
				return err
			}
			if _, shared := remainingOwners[path]; shared {
				continue
			}
			if err := removeCleanFile(config, artifact.Target, path, dryRun); err != nil {
				return err
			}
			report.Removed = append(report.Removed, path)
		}
	}
	return nil
}

func removeCleanFile(config *schema.AtmosConfiguration, target, path string, dryRun bool) error {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect lock-owned file %q: %w", path, err)
	}
	if dryRun {
		return nil
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove lock-owned file %q: %w", path, err)
	}
	root, err := lockTargetRoot(config, target)
	if err != nil {
		return err
	}
	removeEmptyParents(filepath.Dir(path), root)
	return nil
}

func removeCleanArtifactsFromLock(config *schema.AtmosConfiguration, lock *LockFile, selected map[string]Artifact) error {
	for id := range selected {
		delete(lock.Artifacts, id)
	}
	return Save(config, lock)
}

// RedactSource removes URL userinfo and query parameters from a source before it
// is persisted to a committed lock file.
func RedactSource(source string) string {
	if source == "" {
		return ""
	}
	if parsed, err := url.Parse(source); err == nil && parsed.Scheme != "" {
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		return parsed.String()
	}
	return strings.SplitN(source, "?", 2)[0]
}

func lockedPath(config *schema.AtmosConfiguration, target, relative string) (string, error) {
	root, err := lockTargetRoot(config, target)
	if err != nil {
		return "", err
	}
	cleaned := filepath.Clean(filepath.FromSlash(relative))
	if cleaned == "." || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return "", fmt.Errorf("invalid lock-owned file path %q", relative)
	}
	return filepath.Join(root, cleaned), nil
}

// projectRelativeTarget canonicalizes a runtime target before it is persisted.
// Absolute targets are accepted only when they resolve beneath the project root.
func projectRelativeTarget(config *schema.AtmosConfiguration, target string) (string, error) {
	base, err := projectBase(config)
	if err != nil {
		return "", err
	}
	cleaned := filepath.Clean(filepath.FromSlash(target))
	if filepath.IsAbs(cleaned) {
		cleaned, err = filepath.Rel(base, cleaned)
		if err != nil {
			return "", fmt.Errorf("make target relative to project root: %w", err)
		}
	}
	if cleaned == "." || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return "", fmt.Errorf("invalid vendor lock target %q", target)
	}
	return filepath.ToSlash(cleaned), nil
}

// lockTargetRoot resolves a persisted target beneath the project root. Persisted
// targets must be relative so a crafted lock cannot redirect filesystem actions.
func lockTargetRoot(config *schema.AtmosConfiguration, target string) (string, error) {
	if filepath.IsAbs(filepath.FromSlash(target)) {
		return "", fmt.Errorf("invalid absolute vendor lock target %q", target)
	}
	relative, err := projectRelativeTarget(config, target)
	if err != nil {
		return "", err
	}
	base, err := projectBase(config)
	if err != nil {
		return "", err
	}
	root := filepath.Join(base, filepath.FromSlash(relative))
	if relative, err := filepath.Rel(base, root); err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("vendor lock target %q escapes project root", target)
	}
	return root, nil
}

func projectBase(config *schema.AtmosConfiguration) (string, error) {
	base := ""
	if config != nil {
		base = config.BasePath
		if base == "" {
			base = config.CliConfigPath
		}
	}
	if base == "" {
		var err error
		base, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get project root: %w", err)
		}
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	return filepath.Clean(abs), nil
}

func matches(file File, path string, info os.FileInfo) bool {
	if file.Type == "symlink" {
		if info.Mode()&os.ModeSymlink == 0 {
			return false
		}
		target, err := os.Readlink(path)
		return err == nil && hashString(target) == file.SHA256
	}
	if !info.Mode().IsRegular() {
		return false
	}
	digest, err := hashFile(path)
	return err == nil && digest == file.SHA256
}

func removeEmptyParents(path, root string) {
	root = filepath.Clean(root)
	for path != root && path != "." {
		if err := os.Remove(path); err != nil {
			return
		}
		path = filepath.Dir(path)
	}
}

func nextOrder(lock *LockFile) int {
	order := 0
	for _, artifact := range lock.Artifacts {
		if artifact.Order > order {
			order = artifact.Order
		}
	}
	return order + 1
}

func ownedByAnother(config *schema.AtmosConfiguration, lock *LockFile, excludedID, path string) bool {
	for id, artifact := range lock.Artifacts {
		if id == excludedID {
			continue
		}
		for _, file := range artifact.Files {
			artifactPath, err := lockedPath(config, artifact.Target, file.Path)
			if err == nil && artifactPath == path {
				return true
			}
		}
	}
	return false
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path) // #nosec G304 -- inventory paths are rooted at a caller-selected vendor target.
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func hashString(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
