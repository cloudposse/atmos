package vendor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/vendoring/updater"
	"github.com/cloudposse/atmos/pkg/vendoring/version"
)

// TestVendorUpdateCommand_MalformedLabelsErrors proves an unparsable --labels value (missing the
// "=" or ":" key/value separator) is rejected before any manifest/config resolution is attempted.
func TestVendorUpdateCommand_MalformedLabelsErrors(t *testing.T) {
	resetCommandFlags(t, vendorUpdateCmd)
	require.NoError(t, vendorUpdateCmd.Flags().Set("labels", "notkeyvalue"))

	err := vendorUpdateCmd.RunE(vendorUpdateCmd, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
}

// TestVendorUpdateCommand_ComponentAndStackRejected drives validateUpdateSelectorFlags' rejection
// of an explicit --component combined with --stack through vendorUpdateCmd.RunE itself -- the
// function has its own direct unit coverage (TestValidateUpdateSelectorFlags in
// component_updater_test.go), but was never previously exercised through the command path.
func TestVendorUpdateCommand_ComponentAndStackRejected(t *testing.T) {
	resetCommandFlags(t, vendorUpdateCmd)
	require.NoError(t, vendorUpdateCmd.Flags().Set("component", "vpc"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("stack", "dev"))

	err := vendorUpdateCmd.RunE(vendorUpdateCmd, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
}

// TestVendorUpdateCommand_LabelsSelector proves --labels alone (no --stack) resolves through
// RunE's stack-resolution branch and scopes the update to just the matching component, leaving a
// differently-labeled sibling untouched -- mirrors clean_test.go's
// TestVendorCleanCmd_LabelsFilterScopesRemoval, adapted to update's --check/--format=json result
// shape since update mutates vendor.yaml in place rather than deleting files.
func TestVendorUpdateCommand_LabelsSelector(t *testing.T) {
	resetCommandFlags(t, vendorUpdateCmd)
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(
		"base_path: \".\"\nstacks:\n  base_path: stacks\n  included_paths:\n    - \"**/*.yaml\"\n  excluded_paths: []\ncomponents:\n  terraform:\n    base_path: components/terraform\n",
	), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "dev.yaml"), []byte(
		"vars:\n  stage: dev\ncomponents:\n  terraform:\n    first:\n      metadata:\n        labels:\n          tier: \"1\"\n    second:\n      metadata:\n        labels:\n          tier: \"2\"\n",
	), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, DefaultVendorManifest), []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: first
      source: github.com/cloudposse/mock-first
      version: 1.0.0
      targets: ["components/terraform/first"]
    - component: second
      source: github.com/cloudposse/mock-second
      version: 1.0.0
      targets: ["components/terraform/second"]
`), 0o644))
	chdirTest(t, dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	lister := &componentUpdaterLister{tags: []string{"1.0.0", "1.1.0"}}
	previous := version.DefaultLister
	version.DefaultLister = lister
	t.Cleanup(func() { version.DefaultLister = previous })

	stdout := captureVendorStdout(t)
	require.NoError(t, vendorUpdateCmd.Flags().Set("labels", "tier=1"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("check", "true"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("format", "json"))

	err := vendorUpdateCmd.RunE(vendorUpdateCmd, nil)
	require.NoError(t, err)

	var result updater.Result
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	require.Len(t, result.Updates, 1, "only the tier=1-labeled component must be selected")
	assert.Equal(t, "first", result.Updates[0].Component)
	assert.Equal(t, "updated", result.Status)
}
