package vendor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestVendorSelectorGroupCount proves --stack and --labels count as ONE selector group (they
// compose with each other, resolving the same stack-declared component set), while --component
// counts as its own, mutually exclusive group. --tags is not a parameter at all anymore: it's an
// independent filter that composes with either base selector (or stands alone), applied downstream
// by resolveVendorSelectorComponents via vendoring.FilterComponentsByDeclaredTags, not a third
// mutually exclusive selector "mode".
func TestVendorSelectorGroupCount(t *testing.T) {
	tests := []struct {
		name      string
		component string
		stack     string
		labels    map[string]string
		want      int
	}{
		{name: "none", want: 0},
		{name: "component only", component: "vpc", want: 1},
		{name: "stack only", stack: "dev", want: 1},
		{name: "labels only", labels: map[string]string{"tier": "1"}, want: 1},
		{name: "stack and labels count as one group", stack: "dev", labels: map[string]string{"tier": "1"}, want: 1},
		{name: "component and stack", component: "vpc", stack: "dev", want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, vendorSelectorGroupCount(tt.component, tt.stack, tt.labels))
		})
	}
}

// writeSelectorVendorManifest creates a vendor.yaml (anywhere -- returned as an absolute path meant
// to be passed as the --file override, so no chdir/atmos.yaml/vendor.base_path setup is needed;
// mirrors how vendoring.VendorFilePresent treats a non-empty override) declaring three sources:
// "vpc" (tags: networking), "eks/cluster" (tags: compute, networking), and "rds" (no tags) -- for
// exercising resolveVendorTagsSelector's matches-any semantics.
func writeSelectorVendorManifest(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	vendorYaml := `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-vpc
      version: 1.0.0
      tags: [networking]
      targets:
        - components/terraform/vpc
    - component: eks/cluster
      source: github.com/cloudposse/terraform-aws-eks-cluster
      version: 1.0.0
      tags: [compute, networking]
      targets:
        - components/terraform/eks/cluster
    - component: rds
      source: github.com/cloudposse/terraform-aws-rds
      version: 1.0.0
      targets:
        - components/terraform/rds
`
	file := filepath.Join(root, "vendor.yaml")
	require.NoError(t, os.WriteFile(file, []byte(vendorYaml), 0o644))
	return file
}

func TestResolveVendorTagsSelector(t *testing.T) {
	vendorFile := writeSelectorVendorManifest(t)

	t.Run("matches any of the given tags", func(t *testing.T) {
		got, err := resolveVendorTagsSelector(vendorFile, []string{"networking"})
		require.NoError(t, err)
		assert.Equal(t, []string{"eks/cluster", "vpc"}, got)
	})

	t.Run("a tag matching nothing returns empty, not an error", func(t *testing.T) {
		got, err := resolveVendorTagsSelector(vendorFile, []string{"does-not-exist"})
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("no vendor.yaml present returns nil, no error", func(t *testing.T) {
		// An empty vendorFile (no --file override) makes VendorFilePresent search cwd/atmos.yaml's
		// vendor.base_path for a vendor.yaml; a non-empty override, by contrast, is always trusted
		// verbatim by VendorFilePresent (even if it doesn't exist), so this must chdir into a fresh,
		// vendor.yaml-less directory and pass "" rather than a made-up missing path.
		chdirTest(t, t.TempDir())
		got, err := resolveVendorTagsSelector("", []string{"networking"})
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("two sources declaring the same component name dedupe to one entry", func(t *testing.T) {
		dupFile := filepath.Join(t.TempDir(), "vendor.yaml")
		require.NoError(t, os.WriteFile(dupFile, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-vpc
      version: 1.0.0
      tags: [networking]
      targets:
        - components/terraform/vpc
    - component: vpc
      source: github.com/cloudposse/terraform-aws-vpc-2
      version: 2.0.0
      tags: [networking]
      targets:
        - components/terraform/vpc2
`), 0o644))

		got, err := resolveVendorTagsSelector(dupFile, []string{"networking"})
		require.NoError(t, err)
		assert.Equal(t, []string{"vpc"}, got, "two sources declaring the same component name must dedupe to one entry")
	})
}

// TestResolveVendorSelectorComponents_UnmatchedSelectorErrors proves a --tags selector matching
// nothing is a hard error, not a silent empty/all fallback -- see resolveVendorSelectorComponents'
// own doc comment for why (an unresolved --stack/--tags/--labels selector must never be
// indistinguishable from "no selector given" to a caller like `vendor clean` that treats the latter
// as "operate on everything").
func TestResolveVendorSelectorComponents_UnmatchedSelectorErrors(t *testing.T) {
	vendorFile := writeSelectorVendorManifest(t)

	_, err := resolveVendorSelectorComponents(&VendorSelectorOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Tags:        []string{"does-not-exist"},
		VendorFile:  vendorFile,
	})

	require.Error(t, err)
}

// TestResolveVendorSelectorComponents_NoSelectorReturnsNilNoError proves no selector at all yields
// a nil, error-free result -- distinct from an unmatched selector's error.
func TestResolveVendorSelectorComponents_NoSelectorReturnsNilNoError(t *testing.T) {
	got, err := resolveVendorSelectorComponents(&VendorSelectorOptions{AtmosConfig: &schema.AtmosConfiguration{}})

	require.NoError(t, err)
	assert.Nil(t, got)
}

// TestResolveVendorSelectorComponents_ComponentAlwaysResolvesToItself proves --component never hits
// the "matched nothing" error path when --tags isn't also given -- it always resolves to exactly
// the name given.
func TestResolveVendorSelectorComponents_ComponentAlwaysResolvesToItself(t *testing.T) {
	got, err := resolveVendorSelectorComponents(&VendorSelectorOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Component:   "vpc",
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"vpc"}, got)
}

// TestResolveVendorSelectorComponents_ComponentAndTagsCompose proves --tags composes with
// --component instead of being rejected as a conflicting selector: "vpc" (declared, tagged
// networking) survives a --tags=networking filter.
func TestResolveVendorSelectorComponents_ComponentAndTagsCompose(t *testing.T) {
	vendorFile := writeSelectorVendorManifest(t)

	got, err := resolveVendorSelectorComponents(&VendorSelectorOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Component:   "vpc",
		Tags:        []string{"networking"},
		VendorFile:  vendorFile,
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"vpc"}, got)
}

// TestResolveVendorSelectorComponents_ComponentAndTagsExcludesMismatch proves --tags composing with
// --component can exclude it: "rds" (declared, no tags at all) doesn't survive a non-empty --tags
// filter, and the result is the same "matched nothing" error an unmatched --stack/--labels/--tags
// selector produces -- not a silent empty success.
func TestResolveVendorSelectorComponents_ComponentAndTagsExcludesMismatch(t *testing.T) {
	vendorFile := writeSelectorVendorManifest(t)

	_, err := resolveVendorSelectorComponents(&VendorSelectorOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Component:   "rds",
		Tags:        []string{"networking"},
		VendorFile:  vendorFile,
	})

	require.Error(t, err)
}
