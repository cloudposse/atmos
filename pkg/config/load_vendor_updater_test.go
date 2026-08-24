package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestLoadConfig_SyncsVendorUpdaterConfigToGlobalViper verifies that atmos.yaml's
// vendor.update.* and vendor.ci.* settings -- which have no corresponding CLI flag --
// are synced into the global viper singleton the Component Updater (cmd/vendor) reads.
// Without this sync, every atmos.yaml-only setting here is silently ignored and the
// command always falls back to its hardcoded defaults, regardless of what atmos.yaml
// declares (confirmed by manual end-to-end testing before this fix: a configured
// `vendor.update.groups.platform` was rejected as "not configured", and a custom
// `vendor.ci.pull_request.title`/`labels` never reached a real created pull request).
func TestLoadConfig_SyncsVendorUpdaterConfigToGlobalViper(t *testing.T) {
	tmpDir := t.TempDir()
	atmosYAML := "" +
		"base_path: ./\n" +
		"vendor:\n" +
		"  update:\n" +
		"    execution:\n" +
		"      mode: worktree\n" +
		"    batching:\n" +
		"      mode: scope\n" +
		"    groups:\n" +
		"      platform:\n" +
		"        include: [\"terraform/vpc\"]\n" +
		"        exclude: [\"terraform/legacy\"]\n" +
		"  ci:\n" +
		"    pull_request:\n" +
		"      provider: github\n" +
		"      base_branch: develop\n" +
		"      branch_prefix: custom-prefix\n" +
		"      title: \"custom title\"\n" +
		"      body: \"custom body\"\n" +
		"      labels: [\"custom-label\"]\n" +
		"      draft: true\n" +
		"      reviewers: [\"octocat\"]\n" +
		"      assignees: [\"octocat\"]\n" +
		"    summary:\n" +
		"      enabled: false\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, AtmosConfigFileName), []byte(atmosYAML), 0o644))

	t.Chdir(tmpDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	// Precondition: atmos.yaml was actually parsed into atmosConfig.
	require.Equal(t, "worktree", atmosConfig.Vendor.Update.Execution.Mode)
	require.Contains(t, atmosConfig.Vendor.Update.Groups, "platform")

	v := viper.GetViper()
	assert.Equal(t, "worktree", v.GetString("vendor.update.execution.mode"))
	assert.Equal(t, "scope", v.GetString("vendor.update.batching.mode"))
	assert.True(t, v.IsSet("vendor.update.groups.platform"),
		"a configured group must be visible via IsSet, or --group <name> always fails as \"not configured\"")
	assert.Equal(t, []string{"terraform/vpc"}, v.GetStringSlice("vendor.update.groups.platform.include"))
	assert.Equal(t, []string{"terraform/legacy"}, v.GetStringSlice("vendor.update.groups.platform.exclude"))

	assert.Equal(t, "github", v.GetString("vendor.ci.pull_request.provider"))
	assert.Equal(t, "develop", v.GetString("vendor.ci.pull_request.base_branch"))
	assert.Equal(t, "custom-prefix", v.GetString("vendor.ci.pull_request.branch_prefix"))
	assert.Equal(t, "custom title", v.GetString("vendor.ci.pull_request.title"))
	assert.Equal(t, "custom body", v.GetString("vendor.ci.pull_request.body"))
	assert.Equal(t, []string{"custom-label"}, v.GetStringSlice("vendor.ci.pull_request.labels"))
	assert.True(t, v.GetBool("vendor.ci.pull_request.draft"))
	assert.Equal(t, []string{"octocat"}, v.GetStringSlice("vendor.ci.pull_request.reviewers"))
	assert.Equal(t, []string{"octocat"}, v.GetStringSlice("vendor.ci.pull_request.assignees"))
	assert.True(t, v.IsSet("vendor.ci.summary.enabled"))
	assert.False(t, v.GetBool("vendor.ci.summary.enabled"))
}

// TestLoadConfig_VendorUpdaterConfigOmittedLeavesGlobalViperUnset verifies that when
// atmos.yaml declares none of this config, LoadConfig does not fabricate any of these
// keys on the global viper (so callers' own hardcoded defaults still apply cleanly).
func TestLoadConfig_VendorUpdaterConfigOmittedLeavesGlobalViperUnset(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, AtmosConfigFileName), []byte("base_path: ./\n"), 0o644))

	t.Chdir(tmpDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	_, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	v := viper.GetViper()
	assert.False(t, v.IsSet("vendor.update.execution.mode"))
	assert.False(t, v.IsSet("vendor.update.groups.platform"))
	assert.False(t, v.IsSet("vendor.ci.pull_request.title"))
	assert.False(t, v.IsSet("vendor.ci.summary.enabled"))
}
