package component

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestConfiguredLocalSourceSubdirSelectsModule(t *testing.T) {
	root := t.TempDir()
	module := filepath.Join(root, "modules", "app")
	require.NoError(t, os.MkdirAll(module, 0o755))
	info := &schema.ConfigAndStacksInfo{BaseComponentPath: "identity-only", ComponentSection: map[string]any{
		"metadata": map[string]any{"source_subdir": "modules/app"},
	}}
	selected, err := ResolveConfiguredSubpath(info, root)
	require.NoError(t, err)
	require.Equal(t, module, selected)
	info.ComponentSection[workdir.WorkdirPathKey] = root
	selected, err = ApplyWorkdirSubpathToSection(info)
	require.NoError(t, err)
	require.Equal(t, module, selected)
	require.NoError(t, os.RemoveAll(module))
	_, err = ResolveConfiguredSubpath(info, root)
	require.Error(t, err, "an explicit module path cannot fall back to the root")
}

func TestPendingSourceSubdirResolvesBeforeTheSourceIsProvisioned(t *testing.T) {
	root := t.TempDir()
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"metadata": map[string]any{"source_subdir": "modules/app"}}}
	expected := filepath.Join(root, "modules", "app")

	selected, present, err := ResolvePendingSubpath(info, root)
	require.NoError(t, err)
	require.False(t, present)
	require.Equal(t, expected, selected, "a missing subdirectory resolves to the path it will have")
	_, err = ResolveConfiguredSubpath(info, root)
	require.Error(t, err, "the strict resolution after provisioning still requires the directory")

	require.NoError(t, os.MkdirAll(expected, 0o755))
	selected, present, err = ResolvePendingSubpath(info, root)
	require.NoError(t, err)
	require.True(t, present)
	require.Equal(t, expected, selected)

	invalid := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"metadata": map[string]any{"source_subdir": "../escape"}}}
	_, _, err = ResolvePendingSubpath(invalid, root)
	require.Error(t, err, "an invalid subdirectory is rejected whether or not the source is provisioned")

	require.NoError(t, os.WriteFile(filepath.Join(root, "file"), nil, 0o644))
	notDir := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"metadata": map[string]any{"source_subdir": "file"}}}
	_, _, err = ResolvePendingSubpath(notDir, root)
	require.Error(t, err, "only a missing subdirectory is tolerated")
}

func TestDryRunReportsAMissingSourceSubdirAsAbsent(t *testing.T) {
	base := t.TempDir()
	section := map[string]any{
		"component": "vpc", "atmos_component": "vpc", "atmos_stack": "dev",
		"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
		"source":    map[string]any{"uri": "./module"},
		"metadata":  map[string]any{"source_subdir": "modules/app"},
	}
	info := &schema.ConfigAndStacksInfo{DryRun: true, FinalComponent: "vpc", Stack: "dev", ComponentSection: section}

	path, exists, err := ProvisionAndResolveComponentPath(context.Background(), provisioner.OutputWriters{}, &schema.AtmosConfiguration{BasePath: base}, info, cfg.TerraformComponentType, filepath.Join(base, "components", "vpc"))

	require.NoError(t, err)
	require.False(t, exists)
	suffix := string(filepath.Separator) + filepath.Join("modules", "app")
	require.True(t, strings.HasSuffix(path, suffix), "the path names the missing module directory: %s", path)
	require.Equal(t, strings.TrimSuffix(path, suffix), section[workdir.WorkdirPathKey], "the section keeps the workdir root for a later resolution")
	require.NotContains(t, section, workdirSubpathAppliedKey)
	entries, err := os.ReadDir(base)
	require.NoError(t, err)
	require.Empty(t, entries, "a dry run creates nothing")
}
