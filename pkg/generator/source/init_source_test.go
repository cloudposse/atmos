package source

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNormalizeInitSource(t *testing.T) {
	for _, tc := range []struct {
		input, ref, want, name string
		copy                   bool
	}{
		{"examples/quick-start-simple", "", "git::https://github.com/cloudposse/atmos.git//examples/quick-start-simple?ref=main", "quick-start-simple", true},
		{"github.com/cloudposse/atmos//examples/demo?depth=0", "", "git::https://github.com/cloudposse/atmos.git//examples/demo?depth=0", "demo", true},
		{"examples/scaffolds/aws/app", "v1", "git::https://github.com/cloudposse/atmos.git//examples/scaffolds/aws/app?ref=v1", "app", true},
		{"github.com/cloudposse/atmos//examples/quick-start-simple", "", "git::https://github.com/cloudposse/atmos.git//examples/quick-start-simple", "quick-start-simple", true},
		{"github.com/cloudposse/atmos/examples/scaffolding", "", "git::https://github.com/cloudposse/atmos.git//examples/scaffolding", "scaffolding", true},
		{"https://github.com/cloudposse/atmos/tree/v1/examples/scaffolding", "ignored", "git::https://github.com/cloudposse/atmos.git//examples/scaffolding?ref=v1", "scaffolding", true},
		{"git::https://github.com/acme/repo.git//tree/example?ref=pinned", "ignored", "git::https://github.com/acme/repo.git//tree/example?ref=pinned", "example", false},
		{"github.com/acme/repo//nested/example", "feature/hello&world", "git::https://github.com/acme/repo.git//nested/example?ref=feature%2Fhello%26world", "example", false},
		{"https://github.com/acme/repo", "", "git::https://github.com/acme/repo.git", "repo", false},
		{"https://example.com/project.tar.gz", "ignored", "https://example.com/project.tar.gz", "project", false},
		{"./examples/scaffolding", "", "./examples/scaffolding", "scaffolding", false},
		{"git::ssh://git@example.com/acme/project.git//demo?ref=v1", "v2", "git::ssh://git@example.com/acme/project.git//demo?ref=v1", "demo", false},
		{"git@github.com:cloudposse/atmos.git//examples/scaffolding", "", "git::ssh://git@github.com/cloudposse/atmos.git//examples/scaffolding", "scaffolding", true},
		{"git::ssh://git@github.com/cloudposse/atmos.git//examples/scaffolding", "", "git::ssh://git@github.com/cloudposse/atmos.git//examples/scaffolding", "scaffolding", true},
		{"git@example.com:acme/project.git", "v1", "git@example.com:acme/project.git?ref=v1", "project", false},
		{"./100% local", "", "./100% local", "100% local", false},
		{"oci://registry.example.com:5000/team/starter:v1", "ignored", "oci://registry.example.com:5000/team/starter:v1", "starter", false},
		{"oci://github.com/team/starter@sha256:abc", "", "oci://github.com/team/starter@sha256:abc", "starter", false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := NormalizeInitSource(tc.input, tc.ref)
			require.NoError(t, err)
			assert.Equal(t, InitSource{Source: tc.want, Name: tc.name, Copy: tc.copy}, got)
		})
	}
}

func TestNormalizeInitSourceRejectsTraversal(t *testing.T) {
	for _, src := range []string{"examples/../secrets", "github.com/acme/repo//a/%2e%2e/b", "github.com/acme/repo//a%5cb"} {
		_, err := NormalizeInitSource(src, "")
		assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
	}
}

func TestNormalizeInitSourceNativeAbsolutePath(t *testing.T) {
	// Percent signs must remain literal, including in Windows drive paths.
	dir := filepath.Join(t.TempDir(), "100% local")
	got, err := NormalizeInitSource(dir, "ignored")
	require.NoError(t, err)
	assert.Equal(t, InitSource{Source: dir, Name: "100% local"}, got)
}

func TestFetchDirectoryDoesNotInterpretScaffold(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaffold.yaml"), []byte("invalid: ["), 0o600))
	directory, cleanup, err := FetchDirectory(&schema.AtmosConfiguration{}, "example", dir, 0)
	require.NoError(t, err)
	cleanup()
	assert.DirExists(t, dir, "local cleanup must not remove the source")
	assert.Equal(t, dir, directory.Path)
	_, err = directory.LoadScaffold("example", dir)
	assert.Error(t, err, "strict scaffold callers must still reject invalid manifests")
}

func TestFetchDirectoryWithoutManifest(t *testing.T) {
	dir := t.TempDir()
	directory, cleanup, err := FetchDirectory(nil, "example", dir, 0)
	require.NoError(t, err)
	defer cleanup()
	_, err = directory.LoadScaffold("example", dir)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldConfigMissing)
}

func TestFileURIProvenanceUsesDecodedPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "with spaces")
	require.NoError(t, os.Mkdir(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaffold.yaml"), []byte(sampleScaffold), 0o600))
	uri := sourceTestGitFileURI(dir)
	conf, cleanup, err := Resolve(nil, "sample", uri, 0)
	require.NoError(t, err)
	defer cleanup()
	assert.Equal(t, dir, conf.Source)
	assert.Equal(t, dir, conf.IncludeSourceDir())
}

func TestLocalDirectoryFileURIPath(t *testing.T) {
	// An absolute drive path in a file URI has an extra slash on Windows.
	// On Unix the same URI refers to a literal directory named "C:" at the root.
	uriPath := "/C:/atmos-file-uri-test/missing"
	wantPath := uriPath
	if filepath.VolumeName("C:") != "" {
		wantPath = filepath.FromSlash(uriPath[1:])
	}
	_, err := localDirectory((&url.URL{Scheme: "file", Path: uriPath}).String())
	var pathError *os.PathError
	require.ErrorAs(t, err, &pathError)
	assert.Equal(t, wantPath, pathError.Path)
}

func TestFetchDirectoryArchiveAndCleanup(t *testing.T) {
	archive := zipArchive(t, map[string]string{"README.md": "{{ literal }}", "template.tmpl": "# atmos:template\n{{ raw }}"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	directory, cleanup, err := FetchDirectory(&schema.AtmosConfiguration{}, "example", server.URL+"/example.zip", 0)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	content, err := os.ReadFile(filepath.Join(directory.Path, "template.tmpl"))
	require.NoError(t, err)
	assert.Equal(t, "# atmos:template\n{{ raw }}", string(content))
	cleanup()
	assert.NoDirExists(t, directory.Path)
}

func TestFetchDirectoryFailureCleansTemporaryFiles(t *testing.T) {
	// Use a private temp root so cleanup can be asserted without observing
	// unrelated processes' temporary download directories.
	root := t.TempDir()
	t.Setenv("TMPDIR", root)
	t.Setenv("TMP", root)
	t.Setenv("TEMP", root)
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	_, cleanup, err := FetchDirectory(&schema.AtmosConfiguration{}, "missing", server.URL+"/missing.zip", 0)
	require.Error(t, err)
	cleanup()
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
