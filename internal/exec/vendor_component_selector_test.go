package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestResolveVendorComponentSelector_NoFilterReturnsEveryStacksComponents proves stack == "" scopes
// across every stack, and unlike resolveStackVendorComponents (pull's --stack path), a component
// with no component.yaml (eks) IS included -- update/diff/clean/verify key off vendor.yaml by name,
// not manifest presence.
func TestResolveVendorComponentSelector_NoFilterReturnsEveryStacksComponents(t *testing.T) {
	atmosConfig := buildHandleStackVendorFixture(t)

	got, err := ResolveVendorComponentSelector(&atmosConfig, "", nil, []string{cfg.TerraformComponentType})

	require.NoError(t, err)
	assert.Equal(t, []string{"eks", "vpc", "vpc-prod"}, got)
}

// TestResolveVendorComponentSelector_ScopedByStack proves a non-empty stack narrows to just that
// stack's components.
func TestResolveVendorComponentSelector_ScopedByStack(t *testing.T) {
	atmosConfig := buildHandleStackVendorFixture(t)

	got, err := ResolveVendorComponentSelector(&atmosConfig, "dev", nil, []string{cfg.TerraformComponentType})

	require.NoError(t, err)
	assert.Equal(t, []string{"eks", "vpc"}, got)
}

// TestResolveVendorComponentSelector_FilteredByLabels proves labels narrow across all stacks (AND
// match against metadata.labels), independent of --stack.
func TestResolveVendorComponentSelector_FilteredByLabels(t *testing.T) {
	atmosConfig := buildHandleStackVendorFixture(t)

	got, err := ResolveVendorComponentSelector(&atmosConfig, "", map[string]string{"tier": "1"}, []string{cfg.TerraformComponentType})

	require.NoError(t, err)
	assert.Equal(t, []string{"vpc"}, got, "only dev's vpc carries tier=1; eks has no labels and vpc-prod carries tier=2")
}

// TestResolveVendorComponentSelector_StackAndLabelsCompose proves --stack and --labels compose as a
// further narrowing (both resolve the same stack-declared component set) rather than as alternative
// modes: prod's vpc-prod carries tier=2, so scoping to "dev" (tier=1 only) with a tier=2 filter
// matches nothing.
func TestResolveVendorComponentSelector_StackAndLabelsCompose(t *testing.T) {
	atmosConfig := buildHandleStackVendorFixture(t)

	got, err := ResolveVendorComponentSelector(&atmosConfig, "dev", map[string]string{"tier": "2"}, []string{cfg.TerraformComponentType})

	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestResolveVendorComponentSelector_UnknownStackReturnsEmptyNoError proves an unresolvable stack
// name returns an empty (not error) result -- unlike handleStackVendor's own "vendor pull --stack"
// entry point (which hard-errors via errUtils.ErrInvalidArgumentError), callers of this shared
// resolver (vendor update/diff/clean/verify) are responsible for deciding what an empty selector
// match means for them, since some of those commands support NO selector as "operate on everything"
// and must be able to tell that apart from "a selector was given and matched nothing".
func TestResolveVendorComponentSelector_UnknownStackReturnsEmptyNoError(t *testing.T) {
	atmosConfig := buildHandleStackVendorFixture(t)

	got, err := ResolveVendorComponentSelector(&atmosConfig, "does-not-exist", nil, []string{cfg.TerraformComponentType})

	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestResolveVendorComponentSelector_PropagatesDescribeStacksError proves a genuine
// ExecuteDescribeStacksScoped failure (malformed stack YAML, not just an unresolvable stack name --
// see TestResolveVendorComponentSelector_UnknownStackReturnsEmptyNoError above) surfaces as an
// error here too, mirroring TestHandleStackVendor_NamedStackDescribeStacksErrorPropagates
// (vendor_handle_stack_test.go) for the sibling "atmos vendor pull --stack" resolver.
func TestResolveVendorComponentSelector_PropagatesDescribeStacksError(t *testing.T) {
	atmosConfig := buildHandleStackVendorMalformedStackFixture(t)

	got, err := ResolveVendorComponentSelector(&atmosConfig, "dev", nil, []string{cfg.TerraformComponentType})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrExecuteDescribeStacks)
	assert.Contains(t, err.Error(), "failed to describe stacks")
	assert.Nil(t, got)
}

// TestResolveVendorComponentSelector_DedupesAcrossComponentTypes proves this resolver flattens and
// dedupes by component name ALONE across component types (unlike resolveStackVendorComponents,
// which dedupes per type -- see its own doc comment): a component named "shared", declared once as
// terraform and once as helmfile within the same stack, must resolve to a single "shared" entry,
// not two.
func TestResolveVendorComponentSelector_DedupesAcrossComponentTypes(t *testing.T) {
	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))
	devStack := "components:\n  terraform:\n    shared:\n      vars: {}\n  helmfile:\n    shared:\n      vars: {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "dev.yaml"), []byte(devStack), 0o644))
	atmosYAML := "base_path: \".\"\nstacks:\n  base_path: stacks\n  included_paths:\n    - \"**/*.yaml\"\n  excluded_paths: []\ncomponents:\n  terraform:\n    base_path: components/terraform\n  helmfile:\n    base_path: components/helmfile\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYAML), 0o644))
	t.Chdir(tmpDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	got, resolveErr := ResolveVendorComponentSelector(&atmosConfig, "dev", nil, []string{cfg.TerraformComponentType, cfg.HelmfileComponentType})

	require.NoError(t, resolveErr)
	assert.Equal(t, []string{"shared"}, got, "the same component name declared under two component types must dedupe to one entry")
}
