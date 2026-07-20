package lock

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/provisioner"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: a rename of these fields fails the build immediately.
var _ = schema.Terraform{Platforms: nil, PluginCache: false}

func cfgWith(platforms []string, pluginCache, cacheEnabled bool) *schema.AtmosConfiguration {
	c := &schema.AtmosConfiguration{}
	c.Components.Terraform.Platforms = platforms
	c.Components.Terraform.PluginCache = pluginCache
	if cacheEnabled {
		c.Components.Terraform.Cache = &schema.TerraformCacheConfig{Enabled: true}
	}
	return c
}

// recordingExecCtx captures the argv passed to Run and simulates `providers lock` writing
// the canonical lock file in the working directory.
func recordingExecCtx(workingDir string, calls *[][]string) *provisioner.TerraformExecContext {
	return &provisioner.TerraformExecContext{
		WorkingDir: workingDir,
		Run: func(args []string) error {
			*calls = append(*calls, args)
			return os.WriteFile(filepath.Join(workingDir, provisioner.CanonicalLockFilename), []byte("locked\n"), 0o644)
		},
	}
}

func TestAutoLockProviders_RunsForPluginCache(t *testing.T) {
	dir := t.TempDir()
	var calls [][]string
	cfg := cfgWith([]string{"linux_amd64", "darwin_arm64"}, true, false)
	cc := map[string]any{"atmos_stack": "dev", "atmos_component": "vpc"} // plain in-repo.

	require.NoError(t, autoLockProviders(context.Background(), cfg, cc, nil, recordingExecCtx(dir, &calls)))

	require.Len(t, calls, 1)
	assert.Equal(t, []string{"providers", "lock", "-platform=linux_amd64", "-platform=darwin_arm64"}, calls[0])

	// Plain component: canonical lock completed in place, no per-instance dotfile.
	_, err := os.Stat(filepath.Join(dir, provisioner.InstanceLockFilename(cc)))
	assert.True(t, os.IsNotExist(err), "plain component must not produce a per-instance lock")
}

func TestAutoLockProviders_RunsForRegistryCache(t *testing.T) {
	dir := t.TempDir()
	var calls [][]string
	cfg := cfgWith([]string{"linux_amd64", "windows_amd64"}, false, true) // plugin cache off, registry cache on.
	cc := map[string]any{"atmos_stack": "dev", "atmos_component": "vpc"}

	require.NoError(t, autoLockProviders(context.Background(), cfg, cc, nil, recordingExecCtx(dir, &calls)))
	require.Len(t, calls, 1)
	assert.Equal(t, []string{"providers", "lock", "-platform=linux_amd64", "-platform=windows_amd64"}, calls[0])
}

func TestAutoLockProviders_PersistsForSourceComponent(t *testing.T) {
	dir := t.TempDir()
	var calls [][]string
	cfg := cfgWith([]string{"linux_amd64", "darwin_arm64"}, true, false)
	cc := map[string]any{"atmos_stack": "dev", "atmos_component": "vpc", "source": map[string]any{"uri": "example.com/mod"}}

	require.NoError(t, autoLockProviders(context.Background(), cfg, cc, nil, recordingExecCtx(dir, &calls)))
	require.Len(t, calls, 1)

	// Vendored component: completed canonical lock is persisted to the per-instance dotfile.
	got, err := os.ReadFile(filepath.Join(dir, provisioner.InstanceLockFilename(cc)))
	require.NoError(t, err)
	assert.Equal(t, "locked\n", string(got))
}

func TestAutoLockProviders_PersistsForLocalWorkdirComponent(t *testing.T) {
	sourceDir := t.TempDir()
	workdirPath := filepath.Join(t.TempDir(), ".workdir", "terraform", "dev-vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))
	require.NoError(t, provWorkdir.WriteMetadata(workdirPath, &provWorkdir.WorkdirMetadata{
		Component:  "vpc",
		Stack:      "dev",
		SourceType: provWorkdir.SourceTypeLocal,
		Source:     sourceDir,
	}))

	var calls [][]string
	cfg := cfgWith([]string{"linux_amd64", "darwin_arm64"}, true, false)
	cc := map[string]any{
		"atmos_stack":              "dev",
		"atmos_component":          "vpc",
		provWorkdir.WorkdirPathKey: workdirPath,
	}

	require.NoError(t, autoLockProviders(context.Background(), cfg, cc, nil, recordingExecCtx(workdirPath, &calls)))
	require.Len(t, calls, 1)

	got, err := os.ReadFile(filepath.Join(sourceDir, provisioner.InstanceLockFilename(cc)))
	require.NoError(t, err)
	assert.Equal(t, "locked\n", string(got))
}

func TestAutoLockProviders_SkipsRemoteWorkdirSource(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	workdirPath := filepath.Join(dir, ".workdir", "terraform", "dev-vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))
	require.NoError(t, provWorkdir.WriteMetadata(workdirPath, &provWorkdir.WorkdirMetadata{
		Component:  "vpc",
		Stack:      "dev",
		SourceType: provWorkdir.SourceTypeRemote,
		Source:     "github.com/cloudposse/terraform-aws-vpc//src",
	}))

	var calls [][]string
	cfg := cfgWith([]string{"linux_amd64", "darwin_arm64"}, true, false)
	cc := map[string]any{
		"atmos_stack":              "dev",
		"atmos_component":          "vpc",
		provWorkdir.WorkdirPathKey: workdirPath,
	}

	require.NoError(t, autoLockProviders(context.Background(), cfg, cc, nil, recordingExecCtx(workdirPath, &calls)))
	require.Len(t, calls, 1)

	_, err := os.Stat(filepath.Join("github.com", "cloudposse", "terraform-aws-vpc", "src", provisioner.InstanceLockFilename(cc)))
	assert.True(t, os.IsNotExist(err), "remote source identifiers must not be treated as local persistence paths")
}

func TestAutoLockProviders_Skips(t *testing.T) {
	host := runtime.GOOS + "_" + runtime.GOARCH
	tests := []struct {
		name    string
		cfg     *schema.AtmosConfiguration
		execCtx *provisioner.TerraformExecContext
	}{
		{"no platforms declared", cfgWith(nil, true, false), nil},
		{"host platform only", cfgWith([]string{host}, true, false), nil},
		{"no customized install method", cfgWith([]string{"linux_amd64", "darwin_arm64"}, false, false), nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			var calls [][]string
			cc := map[string]any{"atmos_stack": "dev", "atmos_component": "vpc"}
			require.NoError(t, autoLockProviders(context.Background(), tt.cfg, cc, nil, recordingExecCtx(dir, &calls)))
			assert.Empty(t, calls, "providers lock must not run when gating conditions are not met")
		})
	}
}

func TestAutoLockProviders_SkipsNilExecContext(t *testing.T) {
	cfg := cfgWith([]string{"linux_amd64", "darwin_arm64"}, true, false)
	cc := map[string]any{"atmos_stack": "dev", "atmos_component": "vpc"}
	assert.NoError(t, autoLockProviders(context.Background(), cfg, cc, nil, nil))
}
