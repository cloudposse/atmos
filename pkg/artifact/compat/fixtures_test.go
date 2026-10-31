package compat

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/ci/artifact/local"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/plugin/oci/ocitest"
)

// generateFixtures writes missing fixtures under testdata. It never overwrites an existing
// fixture: to refresh one on purpose, a human deletes it and reviews the resulting digest change,
// because a changed legacy archive or plugin manifest digest is an incompatibility, not a refresh.
var generateFixtures = flag.Bool("generate-fixtures", false, "write missing fixtures to testdata (never overwrites)")

const (
	// The legacy planfile key follows the default key pattern {{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan.tar.
	legacyKey = "plat-ue2-dev/vpc/0123456789abcdef0123456789abcdef01234567.tfplan.tar"

	legacyPlanBody = "legacy-planfile-bytes\n"
	legacyLockBody = "# legacy lock file\nprovider \"registry.terraform.io/hashicorp/aws\" {\n  version = \"5.0.0\"\n}\n"
)

// legacyFixtureDir is the committed pre-change CI planfile store (local/dir backend layout).
func legacyFixtureDir() string { return filepath.Join("testdata", "legacy-ci") }

// pluginFixtureDir is the committed plugin release OCI layout.
func pluginFixtureDir() string { return filepath.Join("testdata", "plugin-release") }

// legacyMetadata is fully fixed so the archive and sidecar are byte-deterministic.
func legacyMetadata() *artifact.Metadata {
	return &artifact.Metadata{
		Stack:        "plat-ue2-dev",
		Component:    "vpc",
		SHA:          "0123456789abcdef0123456789abcdef01234567",
		BaseSHA:      "89abcdef0123456789abcdef0123456789abcdef",
		Branch:       "feature/fixture",
		PRNumber:     42,
		RunID:        "run-1",
		Repository:   "acme/infra",
		CreatedAt:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		AtmosVersion: "0.0.0-fixture",
		Custom:       map[string]string{"zone": "a", "env": "dev"},
	}
}

// buildLegacyStore writes the legacy planfile through the CURRENT BundledStore and local backend
// into root and returns the relative slash paths of every file produced.
func buildLegacyStore(t *testing.T, root string) {
	t.Helper()
	backend, err := local.NewStore(artifact.StoreOptions{Type: "local", Options: map[string]any{"path": root}})
	require.NoError(t, err)
	store := artifact.NewBundledStore(backend)
	files := []artifact.FileEntry{
		{Name: "plan.tfplan", Data: bytesReader(legacyPlanBody), Size: -1},
		{Name: ".terraform.lock.hcl", Data: bytesReader(legacyLockBody), Size: -1},
	}
	require.NoError(t, store.Upload(t.Context(), legacyKey, files, legacyMetadata()))
}

// pluginPackageDir assembles the plugin source tree from fixed inputs. Executables are valid
// but never-executed native headers, so the build needs no toolchain.
func pluginPackageDir(t *testing.T, version string) (string, *extension.Registry) {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(directory, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "linux"), ocitest.Binary("linux", "amd64"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "mac"), ocitest.Binary("darwin", "arm64"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(pluginManifest(version)), 0o644))
	descriptors := extension.New()
	require.NoError(t, descriptors.Register(extension.Descriptor{
		Capability: "fixture", Path: []string{"fixtures"}, Version: 1,
		Validate: func(extension.Binding) error { return nil },
	}))
	return directory, descriptors
}

func pluginManifest(version string) string {
	return `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: example/fixture
  version: ` + version + `
spec:
  runtime:
    type: native
    protocol: 1
    executable: bin/plugin
  compatibility:
    protocol: ">=1 <2"
  platforms:
    linux/amd64:
      executable: bin/linux
    darwin/arm64:
      executable: bin/mac
  capabilities:
    fixtures:
      example/fixture: {}
`
}

// packPlugin builds the plugin OCI layout at output (which must not exist) with the CURRENT packer.
func packPlugin(t *testing.T, version, output string) {
	t.Helper()
	directory, descriptors := pluginPackageDir(t, version)
	require.NoError(t, pluginoci.Pack(directory, output, descriptors))
}

// treeBytes returns every regular file below root keyed by slash-separated relative path.
func treeBytes(t *testing.T, root string) map[string][]byte {
	t.Helper()
	tree := make(map[string][]byte)
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		tree[filepath.ToSlash(relative)] = content
		return nil
	}))
	return tree
}

// TestGenerateFixtures is skipped unless -generate-fixtures is passed. It only fills in missing
// fixtures and fails if one already exists, so a regeneration can never mask a change.
func TestGenerateFixtures(t *testing.T) {
	if !*generateFixtures {
		t.Skipf("pass -generate-fixtures to write missing fixtures")
	}
	for _, target := range []string{legacyFixtureDir(), pluginFixtureDir()} {
		_, err := os.Lstat(target)
		require.ErrorIs(t, err, fs.ErrNotExist, "%s already exists; fixtures are frozen. Delete it deliberately and review the digest change", target)
	}
	buildLegacyStore(t, legacyFixtureDir())
	packPlugin(t, "1.2.3", pluginFixtureDir())
}
