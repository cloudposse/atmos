package sarif

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
