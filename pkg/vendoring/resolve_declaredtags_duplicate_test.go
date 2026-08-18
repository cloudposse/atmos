package vendoring

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestFilterComponentsByDeclaredTags_RejectsDuplicateComponent proves that a component declared
// more than once across a vendor manifest and its imports is rejected outright, matching normal
// `vendor pull` processing (internal/exec's mergeVendorConfigFiles), instead of silently letting
// the last declaration's tags win -- --stack/--tags calls this resolution path directly and must
// not behave differently just because it bypasses vendor.yaml-driven installation.
func TestFilterComponentsByDeclaredTags_RejectsDuplicateComponent(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	writeFile(t, dir, "imported.yaml", `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: oci://ghcr.io/cloudposse/mock-vpc:{{.Version}}
      version: v0.1.0
      tags: [networking]
      targets: ["components/terraform/vpc"]
`)
	writeFile(t, dir, "vendor.yaml", `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  imports:
    - imported.yaml
  sources:
    - component: vpc
      source: oci://ghcr.io/cloudposse/mock-vpc:{{.Version}}
      version: v0.2.0
      tags: [compute]
      targets: ["components/terraform/vpc"]
`)

	got, err := FilterComponentsByDeclaredTags("", []string{"vpc"}, []string{"networking"})

	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrDuplicateVendorComponent)
	require.Nil(t, got)
}

// TestFilterComponentsByDeclaredTags_SkipsSourcesWithNoComponentName proves a declared source with
// no `component:` name (the field is optional per vendor-manifest.md) is skipped when building the
// declared-tags map, rather than colliding with every other blank-named source under the duplicate
// check above -- component-less sources simply can't be selected by name and have nothing to match
// against the --tags filter for "vpc".
func TestFilterComponentsByDeclaredTags_SkipsSourcesWithNoComponentName(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	writeFile(t, dir, "vendor.yaml", `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - source: oci://ghcr.io/cloudposse/mock-unnamed:{{.Version}}
      version: v0.1.0
      tags: [networking]
      targets: ["components/terraform/unnamed"]
    - component: vpc
      source: oci://ghcr.io/cloudposse/mock-vpc:{{.Version}}
      version: v0.2.0
      tags: [networking]
      targets: ["components/terraform/vpc"]
`)

	got, err := FilterComponentsByDeclaredTags("", []string{"vpc"}, []string{"networking"})

	require.NoError(t, err)
	require.Equal(t, []string{"vpc"}, got)
}
