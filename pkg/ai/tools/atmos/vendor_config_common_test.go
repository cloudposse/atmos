package atmos

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const vendorConfigFixture = `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: example
spec:
  sources:
    - component: "vpc"
      source: "oci://ghcr.io/cloudposse/atmos/mock:{{.Version}}"
      version: "v0"
      targets:
        - "components/terraform/vpc"
      tags:
        - networking
`

const vendorConfigImportedFixture = `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: tf
spec:
  sources:
    - component: "eks"
      source: "github.com/cloudposse/terraform-aws-eks"
      version: "1.2.3"
`

// writeVendorConfigFixture writes a hand-written vendor.yaml (and, optionally,
// an imported manifest referenced via spec.imports) to a temp dir and returns
// the root manifest path.
func writeVendorConfigFixture(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "vendor.yaml")
	require.NoError(t, os.WriteFile(path, []byte(vendorConfigFixture), 0o600))
	return path
}

// writeVendorConfigFixtureWithImport writes a root vendor.yaml that imports a
// second manifest, plus the imported manifest itself.
func writeVendorConfigFixtureWithImport(t *testing.T, dir string) (rootFile, importedFile string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vendor"), 0o755))
	importedFile = filepath.Join(dir, "vendor", "eks.yaml")
	require.NoError(t, os.WriteFile(importedFile, []byte(vendorConfigImportedFixture), 0o600))

	rootContent := `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: root
spec:
  imports:
    - vendor/eks.yaml
  sources:
    - component: "vpc"
      source: "oci://ghcr.io/cloudposse/atmos/mock:{{.Version}}"
      version: "v0"
`
	rootFile = filepath.Join(dir, "vendor.yaml")
	require.NoError(t, os.WriteFile(rootFile, []byte(rootContent), 0o600))
	return rootFile, importedFile
}

func TestResolveVendorConfigFile(t *testing.T) {
	t.Run("returns the file override when given", func(t *testing.T) {
		file, err := resolveVendorConfigFile("/tmp/custom-vendor.yaml")
		require.NoError(t, err)
		assert.Equal(t, "/tmp/custom-vendor.yaml", file)
	})

	t.Run("defaults to ./vendor.yaml in the current directory", func(t *testing.T) {
		dir := t.TempDir()
		writeVendorConfigFixture(t, dir)
		t.Chdir(dir)

		file, err := resolveVendorConfigFile("")
		require.NoError(t, err)
		assert.Equal(t, defaultVendorManifest, file)
	})

	t.Run("errors when no vendor.yaml exists and no override given", func(t *testing.T) {
		t.Chdir(t.TempDir())

		_, err := resolveVendorConfigFile("")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAIVendorFileNotFound)
	})
}
