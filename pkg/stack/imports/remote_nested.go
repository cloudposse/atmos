package imports

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/duration"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/hashicorp/go-getter"
)

const sourceReadyFilePerm = 0o600

// sourceReadyFileName marks a cached source clone as complete. Its JSON contents
// (sourceMetadata) record when the clone was last refreshed, enabling TTL-based reuse
// across runs. Its presence (and freshness) is the existence check for a cached clone.
const sourceReadyFileName = ".atmos-source-ready"

// sourceMetadata is persisted at the root of a cached source clone. It intentionally
// mirrors the UpdatedAt/SourceURI shape used by source provisioning's workdir metadata,
// but is kept local to avoid coupling the imports package to the provisioner. The TTL
// expiry decision itself is shared via duration.IsExpired.
type sourceMetadata struct {
	SourceURI string    `json:"source_uri"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ResolveNested fetches a remote import and returns all local stack files it resolves to,
// preserving remote source context when nested imports should resolve remotely.
//
// The cached source clone is refreshed on every invocation (no cross-run reuse); use
// ResolveRemoteImportNested with a TTL when cross-run cache reuse is desired.
func (r *RemoteImporter) ResolveNested(uri, nestedImports string) ([]RemoteImportMatch, error) {
	defer perf.Track(nil, "imports.RemoteImporter.ResolveNested")()

	return r.resolveNested(uri, nestedImports, "")
}

// resolveNested is the ttl-aware implementation behind ResolveNested. The ttl controls
// cross-run reuse of the cloned source repo (see ensureSourceDir).
func (r *RemoteImporter) resolveNested(uri, nestedImports, ttl string) ([]RemoteImportMatch, error) {
	defer perf.Track(nil, "imports.RemoteImporter.resolveNested")()

	if nestedImports == "" || nestedImports == schema.StackImportNestedImportsLocal {
		return r.resolve(uri, ttl)
	}
	if nestedImports != schema.StackImportNestedImportsRemote {
		return nil, errUtils.Build(errUtils.ErrInvalidImport).
			WithExplanation("nested_imports must be either 'local' or 'remote'").
			WithContext("nested_imports", nestedImports).
			Err()
	}
	if !IsRemote(uri) {
		return nil, errUtils.Build(errUtils.ErrInvalidRemoteImport).
			WithExplanation("URI is not a remote URL").
			WithContext("uri", uri).
			Err()
	}
	if !IsGitURI(uri) {
		return nil, errUtils.Build(errUtils.ErrInvalidRemoteImport).
			WithExplanation("nested_imports: remote requires a git/go-getter source with a subdirectory path").
			WithContext("uri", uri).
			Err()
	}

	sourceURI, subdir := getter.SourceDirSubdir(uri)
	if subdir == "" {
		return nil, errUtils.Build(errUtils.ErrInvalidRemoteImport).
			WithExplanation("nested_imports: remote requires a git/go-getter source with a subdirectory path").
			WithContext("uri", uri).
			Err()
	}

	cacheKey := uri + "|nested_imports=remote"
	if matches, ok := r.cachedMatches(cacheKey); ok {
		return matches, nil
	}

	matches, err := r.resolveGitSubdirWithRemoteBase(uri, sourceURI, subdir, ttl)
	if err != nil {
		return nil, err
	}
	r.storeMatches(cacheKey, matches)
	return matches, nil
}

func (r *RemoteImporter) resolveGitSubdirWithRemoteBase(originalURI, sourceURI, subdir, ttl string) ([]RemoteImportMatch, error) {
	sourceRoot, err := r.ensureSourceDir(sourceURI, ttl)
	if err != nil {
		return nil, err
	}

	remoteBasePath, err := inferRemoteStackBasePath(sourceRoot, subdir, r.remoteStacksBasePath())
	if err != nil {
		return nil, err
	}

	files, err := resolveStackFiles(sourceRoot, subdir)
	if err != nil {
		return nil, err
	}

	matches := make([]RemoteImportMatch, 0, len(files))
	for _, file := range files {
		rel, err := filepath.Rel(sourceRoot, file)
		if err != nil {
			return nil, err
		}
		key := originalURI + "#" + filepath.ToSlash(rel)
		matches = append(matches, RemoteImportMatch{Path: file, Key: key, BasePath: remoteBasePath})
	}

	return matches, nil
}

// ensureSourceDir clones a remote source repo once and returns the local path to the
// cached clone, shared by every subdir import of that repo.
//
// Reuse policy:
//   - Within a run: a source already fetched in this process is reused unconditionally
//     (regardless of ttl). This is the always-on dedup that collapses N subdir imports
//     of one repo into a single clone; the global importer means it spans both
//     describe-affected passes in one process.
//   - Across runs: the persisted clone is reused only when ttl is set and the clone is
//     still fresh per its recorded UpdatedAt. With no ttl, the clone is re-fetched once
//     per run (mutable refs like `main` stay fresh) while still deduped within the run.
func (r *RemoteImporter) ensureSourceDir(sourceURI, ttl string) (string, error) {
	sourceURI = r.detectGitSource(sourceURI)
	destDir := filepath.Join(r.cache.BaseDir(), uriToTempName(sourceURI)+".source")

	r.sourceMu.Lock()
	defer r.sourceMu.Unlock()

	// Within-run dedup: already fetched this process -> reuse, ignoring TTL.
	if r.sessionFetched[sourceURI] {
		if _, err := os.Stat(destDir); err == nil {
			return destDir, nil
		}
	}

	// Cross-run reuse: only when a TTL is configured and the persisted clone is fresh.
	if sourceCacheFresh(destDir, ttl) {
		r.sessionFetched[sourceURI] = true
		return destDir, nil
	}

	if err := r.fetchSourceDir(sourceURI, destDir); err != nil {
		return "", err
	}
	r.sessionFetched[sourceURI] = true
	return destDir, nil
}

// fetchSourceDir clones sourceURI into a temp dir, writes the freshness marker, and
// atomically swaps it into destDir.
func (r *RemoteImporter) fetchSourceDir(sourceURI, destDir string) error {
	tempDir, err := os.MkdirTemp(r.cache.BaseDir(), filepath.Base(destDir)+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if err := r.downloader.Fetch(sourceURI, tempDir, downloader.ClientModeDir, defaultDownloadTimeout); err != nil {
		return err
	}
	if err := writeSourceMetadata(tempDir, sourceURI); err != nil {
		return err
	}
	if err := os.RemoveAll(destDir); err != nil {
		return err
	}
	return os.Rename(tempDir, destDir)
}

// sourceCacheFresh reports whether a persisted clone at destDir may be reused across
// runs. With no ttl, cross-run reuse is disabled (always refresh). With a ttl, the
// recorded UpdatedAt is checked against it via the shared duration.IsExpired helper.
func sourceCacheFresh(destDir, ttl string) bool {
	if ttl == "" {
		return false
	}
	meta, err := readSourceMetadata(destDir)
	if err != nil {
		return false
	}
	expired, err := duration.IsExpired(meta.UpdatedAt, ttl)
	if err != nil || expired {
		return false
	}
	return true
}

// writeSourceMetadata records the source URI and current time in the ready marker.
func writeSourceMetadata(dir, sourceURI string) error {
	meta := sourceMetadata{SourceURI: sourceURI, UpdatedAt: time.Now()}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, sourceReadyFileName), data, sourceReadyFilePerm)
}

// readSourceMetadata reads the ready marker. A missing marker, or a legacy plain-text
// marker (pre-TTL) that fails to parse or carries no timestamp, returns an error so
// the caller treats the clone as not reusable across runs.
func readSourceMetadata(dir string) (*sourceMetadata, error) {
	data, err := os.ReadFile(filepath.Join(dir, sourceReadyFileName))
	if err != nil {
		return nil, err
	}
	var meta sourceMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	if meta.UpdatedAt.IsZero() {
		return nil, errUtils.Build(errUtils.ErrInvalidRemoteImport).
			WithExplanation("cached source metadata is missing a timestamp").
			Err()
	}
	return &meta, nil
}

func (r *RemoteImporter) remoteStacksBasePath() string {
	if r.atmosConfig == nil || r.atmosConfig.Stacks.BasePath == "" {
		return "stacks"
	}
	return r.atmosConfig.Stacks.BasePath
}

func inferRemoteStackBasePath(sourceRoot, subdir, stacksBasePath string) (string, error) {
	cleanSubdir, err := cleanRemoteSubdir(subdir)
	if err != nil {
		return "", err
	}
	cleanBase := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimPrefix(stacksBasePath, "./"))))
	if cleanBase == "." || cleanBase == "" {
		return sourceRoot, nil
	}

	subdirParts := strings.Split(cleanSubdir, "/")
	baseParts := strings.Split(cleanBase, "/")
	for i := 0; i+len(baseParts) <= len(subdirParts); i++ {
		if samePathParts(subdirParts[i:i+len(baseParts)], baseParts) {
			return filepath.Join(sourceRoot, filepath.FromSlash(strings.Join(subdirParts[:i+len(baseParts)], "/"))), nil
		}
	}

	return "", errUtils.Build(errUtils.ErrInvalidRemoteImport).
		WithExplanation("remote import uses nested_imports: remote, but Atmos could not infer the remote stack base path from the import path").
		WithContext("subdir", cleanSubdir).
		WithContext("stacks_base_path", cleanBase).
		Err()
}

func samePathParts(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// ResolveRemoteImportNested is a convenience function that uses the global importer.
// The ttl controls cross-run reuse of the cloned source repo for git subdir imports
// (see ensureSourceDir); an empty ttl refreshes the clone once per invocation.
func ResolveRemoteImportNested(atmosConfig *schema.AtmosConfiguration, uri, nestedImports, ttl string) ([]RemoteImportMatch, error) {
	defer perf.Track(atmosConfig, "imports.ResolveRemoteImportNested")()

	importer, err := getGlobalImporter(atmosConfig)
	if err != nil {
		return nil, err
	}
	return importer.resolveNested(uri, nestedImports, ttl)
}
