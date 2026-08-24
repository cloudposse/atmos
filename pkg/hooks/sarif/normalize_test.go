package sarif

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newTestMapper builds a mapper over a fake repo laid out the way Atmos runs in
// CI: a working directory under the repo root, with a Terraform component below
// it, plus a real target file the scanners report on.
func newTestMapper(t *testing.T) (uriMapper, string) {
	t.Helper()

	workspace := t.TempDir()
	baseRoot := filepath.Join(workspace, "tests", "fixtures", "scenarios", "native-ci-e2e")
	sourceRoot := filepath.Join(baseRoot, "components", "terraform", "bucket")
	require.NoError(t, os.MkdirAll(sourceRoot, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceRoot, "kics_target.tf"), []byte("# target\n"), 0o644))

	const wantRel = "tests/fixtures/scenarios/native-ci-e2e/components/terraform/bucket/kics_target.tf"
	return uriMapper{
		workspace:  cleanAbs(workspace),
		baseRoot:   cleanAbs(baseRoot),
		sourceRoot: cleanAbs(sourceRoot),
		scanRoot:   cleanAbs(sourceRoot),
	}, wantRel
}

// TestNormalizeRel_WorkdirRelativePathIsNotDoubled covers the kics/Checkov case:
// scanners emit paths relative to the Atmos working directory, which already
// include the component prefix. The mapper must not prepend the component dir
// again (the doubled path GitHub Code Scanning could not anchor).
func TestNormalizeRel_WorkdirRelativePathIsNotDoubled(t *testing.T) {
	m, wantRel := newTestMapper(t)

	got := m.normalize("components/terraform/bucket/kics_target.tf")

	assert.Equal(t, wantRel, got)
	assert.NotContains(t, got, "bucket/components/terraform/bucket", "component prefix must not be doubled")
}

// TestNormalizeRel_FileRelativePathResolvesToSource covers scanners that emit
// just the file name relative to the component dir.
func TestNormalizeRel_FileRelativePathResolvesToSource(t *testing.T) {
	m, wantRel := newTestMapper(t)

	assert.Equal(t, wantRel, m.normalize("kics_target.tf"))
}

// TestNormalizeAbs_AbsolutePathResolvesToSource covers scanners (e.g. Trivy)
// that emit an absolute path under the component dir.
func TestNormalizeAbs_AbsolutePathResolvesToSource(t *testing.T) {
	m, wantRel := newTestMapper(t)

	abs := filepath.Join(m.sourceRoot, "kics_target.tf")
	assert.Equal(t, wantRel, m.normalize(filepath.ToSlash(abs)))
}

func TestPathFromSARIFURI_WindowsDrivePathIsNotURLScheme(t *testing.T) {
	for _, uri := range []string{
		`C:/Users/runneradmin/AppData/Local/Temp/repo/main.tf`,
		`C:\Users\runneradmin\AppData\Local\Temp\repo\main.tf`,
	} {
		t.Run(uri, func(t *testing.T) {
			got, ok := pathFromSARIFURI(uri)
			assert.True(t, ok)
			assert.Equal(t, uri, got)
		})
	}
}

func TestPathFromSARIFURI_ExternalURLIsRejected(t *testing.T) {
	_, ok := pathFromSARIFURI("https://example.com/main.tf")
	assert.False(t, ok)
}

func TestIsWindowsDrivePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{`C:/repo/main.tf`, true},
		{`c:\repo\main.tf`, true},
		{`Z:/x`, true},
		{`C:`, false},        // too short.
		{`Cx/repo`, false},   // missing colon at index 1.
		{`1:/repo`, false},   // non-letter drive.
		{`C:repo`, false},    // no separator after colon.
		{`/abs/path`, false}, // POSIX absolute, not a drive path.
		{``, false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, isWindowsDrivePath(tt.path))
		})
	}
}

func TestCleanAbs(t *testing.T) {
	assert.Equal(t, "", cleanAbs(""), "empty stays empty")

	// A relative path resolves to an absolute, cleaned path.
	got := cleanAbs(filepath.Join("a", "b", "..", "c"))
	assert.True(t, filepath.IsAbs(got), "relative input becomes absolute")
	assert.Equal(t, "c", filepath.Base(got))
}

func TestRelUnder(t *testing.T) {
	root := t.TempDir()
	under := filepath.Join(root, "components", "x.tf")

	rel, ok := relUnder(root, under)
	require.True(t, ok)
	assert.Equal(t, filepath.Join("components", "x.tf"), rel)

	// A sibling directory escapes the root and is rejected.
	_, ok = relUnder(filepath.Join(root, "a"), filepath.Join(root, "b", "x.tf"))
	assert.False(t, ok, "path outside root is unsafe")
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "real.tf")
	require.NoError(t, os.WriteFile(f, []byte("x\n"), 0o600))

	assert.True(t, fileExists(f))
	assert.False(t, fileExists(filepath.Join(dir, "missing.tf")))
}

func TestRelativeBases_DedupesAndDropsEmpty(t *testing.T) {
	m := uriMapper{workspace: "/ws", baseRoot: "/ws", scanRoot: "", sourceRoot: "/src"}
	bases := m.relativeBases()
	assert.Equal(t, []string{"/ws", "/src"}, bases, "duplicate /ws collapsed, empty scanRoot dropped")
}

// normalizeAbs falls back through workspace-relative, source-relative, then the
// original URI when an absolute path resolves under none of the known roots.
func TestNormalizeAbs_Fallbacks(t *testing.T) {
	workspace := t.TempDir()

	// Path under the workspace but outside scan/source roots → workspace-relative.
	m := uriMapper{workspace: cleanAbs(workspace)}
	under := filepath.Join(workspace, "elsewhere", "x.tf")
	assert.Equal(t, "elsewhere/x.tf", m.normalizeAbs(under, "fallback"))

	// Path outside every root and no sourceRoot → returns the fallback verbatim.
	outside := filepath.Join(t.TempDir(), "y.tf")
	assert.Equal(t, "fallback", m.normalizeAbs(outside, "fallback"))

	// Path outside roots but sourceRoot set and path under workspace-as-source.
	m2 := uriMapper{workspace: cleanAbs(workspace), sourceRoot: cleanAbs(workspace)}
	assert.Equal(t, "elsewhere/x.tf", m2.normalizeAbs(under, "fallback"))
}

// normalizeRel preserves the fallback when a relative path resolves to no real
// file under any candidate base and there is no source root to anchor it.
func TestNormalizeRel_UnresolvedReturnsFallback(t *testing.T) {
	m := uriMapper{workspace: cleanAbs(t.TempDir())}
	assert.Equal(t, "fallback", m.normalizeRel("nope/missing.tf", "fallback"))

	// An unsafe (parent-escaping) relative path is rejected outright.
	assert.Equal(t, "fallback", m.normalizeRel(filepath.Join("..", "escape.tf"), "fallback"))
}

func TestAtmosBasePath(t *testing.T) {
	assert.Equal(t, "", atmosBasePath(nil), "nil ctx")
	assert.Equal(t, "", atmosBasePath(&hooks.ExecContext{}), "nil config")

	abs := &hooks.ExecContext{AtmosConfig: &schema.AtmosConfiguration{BasePathAbsolute: "/abs", BasePath: "/rel"}}
	assert.Equal(t, "/abs", atmosBasePath(abs), "absolute base path preferred")

	rel := &hooks.ExecContext{AtmosConfig: &schema.AtmosConfiguration{BasePath: "/rel"}}
	assert.Equal(t, "/rel", atmosBasePath(rel), "falls back to base path")
}

// sourceComponentPath walks the base-path and component-name fallbacks, returning
// "" only when neither a base nor a component can be determined.
func TestSourceComponentPath(t *testing.T) {
	assert.Equal(t, "", sourceComponentPath(nil), "nil ctx")
	assert.Equal(t, "", sourceComponentPath(&hooks.ExecContext{AtmosConfig: &schema.AtmosConfiguration{}}), "nil info")

	// No base path anywhere → "".
	noBase := &hooks.ExecContext{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info:        &schema.ConfigAndStacksInfo{ComponentFromArg: "bucket"},
	}
	assert.Equal(t, "", sourceComponentPath(noBase))

	// Base resolves via BasePathAbsolute; component via ComponentFromArg fallback.
	base := t.TempDir()
	ctx := &hooks.ExecContext{
		AtmosConfig: &schema.AtmosConfiguration{BasePathAbsolute: base},
		Info:        &schema.ConfigAndStacksInfo{ComponentFromArg: "bucket"},
	}
	assert.Equal(t, filepath.Join(base, "bucket"), sourceComponentPath(ctx))

	// Base anywhere but no component name → "".
	noComp := &hooks.ExecContext{
		AtmosConfig: &schema.AtmosConfiguration{BasePath: "/repo"},
		Info:        &schema.ConfigAndStacksInfo{},
	}
	assert.Equal(t, "", sourceComponentPath(noComp))
}

// TestGithubBlobBaseURL verifies githubBlobBaseURL only returns a URL when both
// GITHUB_REPOSITORY and GITHUB_SHA are set (the two pieces it can't construct a
// meaningful blob URL without), defaults to github.com when GITHUB_SERVER_URL is
// unset (GitHub Enterprise sets it explicitly), and returns "" outside CI.
func TestGithubBlobBaseURL(t *testing.T) {
	t.Run("outside CI returns empty", func(t *testing.T) {
		t.Setenv("GITHUB_REPOSITORY", "")
		t.Setenv("GITHUB_SHA", "")
		assert.Empty(t, githubBlobBaseURL())
	})

	t.Run("missing SHA returns empty", func(t *testing.T) {
		t.Setenv("GITHUB_REPOSITORY", "org/repo")
		t.Setenv("GITHUB_SHA", "")
		assert.Empty(t, githubBlobBaseURL())
	})

	t.Run("missing repository returns empty", func(t *testing.T) {
		t.Setenv("GITHUB_REPOSITORY", "")
		t.Setenv("GITHUB_SHA", "abc123")
		assert.Empty(t, githubBlobBaseURL())
	})

	t.Run("repository and SHA set defaults to github.com", func(t *testing.T) {
		t.Setenv("GITHUB_REPOSITORY", "org/repo")
		t.Setenv("GITHUB_SHA", "abc123")
		t.Setenv("GITHUB_SERVER_URL", "")
		assert.Equal(t, "https://github.com/org/repo/blob/abc123", githubBlobBaseURL())
	})

	t.Run("honors GITHUB_SERVER_URL for GitHub Enterprise", func(t *testing.T) {
		t.Setenv("GITHUB_REPOSITORY", "org/repo")
		t.Setenv("GITHUB_SHA", "abc123")
		t.Setenv("GITHUB_SERVER_URL", "https://github.example.com")
		assert.Equal(t, "https://github.example.com/org/repo/blob/abc123", githubBlobBaseURL())
	})
}
