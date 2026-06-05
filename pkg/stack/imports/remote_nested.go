package imports

import (
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/hashicorp/go-getter"
)

const sourceReadyFilePerm = 0o600

// ResolveNested fetches a remote import and returns all local stack files it resolves to,
// preserving remote source context when nested imports should resolve remotely.
func (r *RemoteImporter) ResolveNested(uri, nestedImports string) ([]RemoteImportMatch, error) {
	defer perf.Track(nil, "imports.RemoteImporter.ResolveNested")()

	if nestedImports == "" || nestedImports == schema.StackImportNestedImportsLocal {
		return r.Resolve(uri)
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

	matches, err := r.resolveGitSubdirWithRemoteBase(uri, sourceURI, subdir)
	if err != nil {
		return nil, err
	}
	r.storeMatches(cacheKey, matches)
	return matches, nil
}

func (r *RemoteImporter) resolveGitSubdirWithRemoteBase(originalURI, sourceURI, subdir string) ([]RemoteImportMatch, error) {
	sourceRoot, err := r.ensureSourceDir(sourceURI)
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

func (r *RemoteImporter) ensureSourceDir(sourceURI string) (string, error) {
	sourceURI = r.detectGitSource(sourceURI)
	destDir := filepath.Join(r.cache.BaseDir(), uriToTempName(sourceURI)+".source")
	readyFile := filepath.Join(destDir, ".atmos-source-ready")

	r.sourceMu.Lock()
	defer r.sourceMu.Unlock()

	if _, err := os.Stat(readyFile); err == nil {
		return destDir, nil
	}

	tempDir, err := os.MkdirTemp(r.cache.BaseDir(), uriToTempName(sourceURI)+".source-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	if err := r.downloader.Fetch(sourceURI, tempDir, downloader.ClientModeDir, defaultDownloadTimeout); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tempDir, ".atmos-source-ready"), []byte("ok\n"), sourceReadyFilePerm); err != nil {
		return "", err
	}
	if err := os.RemoveAll(destDir); err != nil {
		return "", err
	}
	if err := os.Rename(tempDir, destDir); err != nil {
		return "", err
	}
	return destDir, nil
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
func ResolveRemoteImportNested(atmosConfig *schema.AtmosConfiguration, uri, nestedImports string) ([]RemoteImportMatch, error) {
	defer perf.Track(atmosConfig, "imports.ResolveRemoteImportNested")()

	importer, err := getGlobalImporter(atmosConfig)
	if err != nil {
		return nil, err
	}
	return importer.ResolveNested(uri, nestedImports)
}
