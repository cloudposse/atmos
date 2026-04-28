package exec

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// applyMetadataComponentSubpath ─────────────────────────────────────────────.

func TestApplyMetadataComponentSubpath_JoinsSubpath(t *testing.T) {
	workdir := t.TempDir()
	result := applyMetadataComponentSubpath("modules/iam-policy", workdir)
	assert.Equal(t, filepath.Join(workdir, "modules", "iam-policy"), result)
}

func TestApplyMetadataComponentSubpath_EmptySubpathReturnsRoot(t *testing.T) {
	workdir := t.TempDir()
	assert.Equal(t, workdir, applyMetadataComponentSubpath("", workdir))
}

// TestApplyMetadataComponentSubpath_AllowsParentEscape codifies the design
// decision that ".." is permitted (issue #2364): some upstream Terraform
// modules reference shared files via relative parent paths.
func TestApplyMetadataComponentSubpath_AllowsParentEscape(t *testing.T) {
	workdir := t.TempDir()
	result := applyMetadataComponentSubpath("../sibling-module", workdir)
	assert.Equal(t, filepath.Join(filepath.Dir(workdir), "sibling-module"), result)
}

// applyWorkdirSubpathToSection ──────────────────────────────────────────────.

func TestApplyWorkdirSubpathToSection_JoinsSubpath(t *testing.T) {
	workdirRoot := t.TempDir()
	info := &schema.ConfigAndStacksInfo{
		BaseComponentPath: "modules/iam-policy",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: workdirRoot,
		},
	}

	got := applyWorkdirSubpathToSection(info)

	expected := filepath.Join(workdirRoot, "modules", "iam-policy")
	assert.Equal(t, expected, got)
	assert.Equal(t, expected, info.ComponentSection[provWorkdir.WorkdirPathKey],
		"WorkdirPathKey should be mutated in place to the joined subpath")
	_, applied := info.ComponentSection[provWorkdir.WorkdirSubpathAppliedKey].(workdirSubpathAppliedMarker)
	assert.True(t, applied, "sentinel marker should be set")
}

func TestApplyWorkdirSubpathToSection_NoWorkdirPathKey(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		BaseComponentPath: "exports",
		ComponentSection:  map[string]any{},
	}

	got := applyWorkdirSubpathToSection(info)

	assert.Empty(t, got)
	_, mutated := info.ComponentSection[provWorkdir.WorkdirPathKey]
	assert.False(t, mutated, "must not introduce WorkdirPathKey when it was absent")
	_, applied := info.ComponentSection[provWorkdir.WorkdirSubpathAppliedKey]
	assert.False(t, applied, "must not set the sentinel when nothing was joined")
}

func TestApplyWorkdirSubpathToSection_EmptyWorkdirPath(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		BaseComponentPath: "exports",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: "",
		},
	}

	got := applyWorkdirSubpathToSection(info)

	assert.Empty(t, got)
	assert.Equal(t, "", info.ComponentSection[provWorkdir.WorkdirPathKey])
}

// TestApplyWorkdirSubpathToSection_DoubleCallAppliesOnce verifies idempotency:
// the second call must not produce <workdir>/<subpath>/<subpath>.
func TestApplyWorkdirSubpathToSection_DoubleCallAppliesOnce(t *testing.T) {
	workdirRoot := t.TempDir()
	info := &schema.ConfigAndStacksInfo{
		BaseComponentPath: "exports",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: workdirRoot,
		},
	}
	expected := filepath.Join(workdirRoot, "exports")

	first := applyWorkdirSubpathToSection(info)
	assert.Equal(t, expected, first)

	second := applyWorkdirSubpathToSection(info)
	assert.Equal(t, expected, second)
	assert.Equal(t, expected, info.ComponentSection[provWorkdir.WorkdirPathKey])
}

// TestApplyWorkdirSubpathToSection_SentinelGatesDoubleJoin is the negative-path
// counterpart to DoubleCallAppliesOnce: deleting the sentinel must re-enable
// the join, proving the sentinel (not some other invariant) is what gates
// idempotency. Required by CLAUDE.md "Include negative-path tests for
// recovery logic".
func TestApplyWorkdirSubpathToSection_SentinelGatesDoubleJoin(t *testing.T) {
	workdirRoot := t.TempDir()
	info := &schema.ConfigAndStacksInfo{
		BaseComponentPath: "exports",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: workdirRoot,
		},
	}

	first := applyWorkdirSubpathToSection(info)
	require.Equal(t, filepath.Join(workdirRoot, "exports"), first)

	delete(info.ComponentSection, provWorkdir.WorkdirSubpathAppliedKey)

	second := applyWorkdirSubpathToSection(info)
	assert.Equal(t, filepath.Join(workdirRoot, "exports", "exports"), second,
		"with the sentinel removed the second call must re-join, proving the sentinel is the gate")
}

// TestApplyWorkdirSubpathToSection_UserYAMLCannotForgeSentinel guards against
// a YAML-author setting `_workdir_subpath_applied: <anything>` and silently
// bypassing the join. The sentinel must be a typed marker, not a presence
// check.
func TestApplyWorkdirSubpathToSection_UserYAMLCannotForgeSentinel(t *testing.T) {
	workdirRoot := t.TempDir()
	for _, forged := range []any{true, "applied", 1, map[string]any{}} {
		info := &schema.ConfigAndStacksInfo{
			BaseComponentPath: "exports",
			ComponentSection: map[string]any{
				provWorkdir.WorkdirPathKey:           workdirRoot,
				provWorkdir.WorkdirSubpathAppliedKey: forged,
			},
		}

		got := applyWorkdirSubpathToSection(info)

		assert.Equal(t, filepath.Join(workdirRoot, "exports"), got,
			"forged sentinel %T(%v) must not bypass the join", forged, forged)
	}
}

// resolveWorkdirComponentPath ───────────────────────────────────────────────.

func TestResolveWorkdirComponentPath_ExistingDir(t *testing.T) {
	basePath := t.TempDir()
	stack := "dev"
	componentName := "null-label-exports"
	subpath := "exports"

	expectedRoot := filepath.Join(basePath, provWorkdir.WorkdirPath, cfg.TerraformComponentType, stack+"-"+componentName)
	expectedCandidate := filepath.Join(expectedRoot, subpath)
	require.NoError(t, os.MkdirAll(expectedCandidate, 0o755))

	atmosConfig := &schema.AtmosConfiguration{BasePath: basePath}
	info := &schema.ConfigAndStacksInfo{
		FinalComponent:    componentName,
		Stack:             stack,
		BaseComponentPath: subpath,
		ComponentSection:  map[string]any{},
	}

	candidate, exists, err := resolveWorkdirComponentPath(atmosConfig, info)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, expectedCandidate, candidate)
}

// TestResolveWorkdirComponentPath_NonExistentDir returns exists=false, no
// error so callers retain their fallback path. The exact constructed path is
// asserted so a regression in BuildPath/applyMetadataComponentSubpath wiring
// fails this test.
func TestResolveWorkdirComponentPath_NonExistentDir(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	info := &schema.ConfigAndStacksInfo{
		FinalComponent:    "missing-component",
		Stack:             "dev",
		BaseComponentPath: "exports",
		ComponentSection:  map[string]any{},
	}
	expectedCandidate := filepath.Join(
		atmosConfig.BasePath,
		provWorkdir.WorkdirPath,
		cfg.TerraformComponentType,
		info.Stack+"-"+info.FinalComponent,
		info.BaseComponentPath,
	)

	candidate, exists, err := resolveWorkdirComponentPath(atmosConfig, info)
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Equal(t, expectedCandidate, candidate)
}

// TestResolveWorkdirComponentPath_RegularFileAtCandidate surfaces a
// non-directory at the candidate path as a wrapped error rather than a silent
// exists=false, so corrupt state is not masked.
func TestResolveWorkdirComponentPath_RegularFileAtCandidate(t *testing.T) {
	basePath := t.TempDir()
	stack := "dev"
	componentName := "corrupt"
	subpath := "exports"

	root := filepath.Join(basePath, provWorkdir.WorkdirPath, cfg.TerraformComponentType, stack+"-"+componentName)
	require.NoError(t, os.MkdirAll(root, 0o755))
	candidate := filepath.Join(root, subpath)
	require.NoError(t, os.WriteFile(candidate, []byte("not a directory"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{BasePath: basePath}
	info := &schema.ConfigAndStacksInfo{
		FinalComponent:    componentName,
		Stack:             stack,
		BaseComponentPath: subpath,
		ComponentSection:  map[string]any{},
	}

	_, exists, err := resolveWorkdirComponentPath(atmosConfig, info)
	require.Error(t, err)
	assert.False(t, exists)
	assert.True(t, errors.Is(err, errUtils.ErrWorkdirProvision),
		"non-directory at candidate must wrap ErrWorkdirProvision")
}

func TestResolveWorkdirComponentPath_StatErrorPropagates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Chmod cannot deny directory traversal on Windows, and os.Getuid is not meaningful there")
	}
	if os.Getuid() == 0 {
		t.Skip("test relies on POSIX permission denial; root bypasses chmod")
	}
	basePath := t.TempDir()
	stack := "dev"
	componentName := "guarded"
	subpath := "exports"

	// Create the workdir root, then chmod the parent so the candidate stat
	// fails with EACCES rather than ENOENT.
	root := filepath.Join(basePath, provWorkdir.WorkdirPath, cfg.TerraformComponentType, stack+"-"+componentName)
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.Chmod(root, 0o000))
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	atmosConfig := &schema.AtmosConfiguration{BasePath: basePath}
	info := &schema.ConfigAndStacksInfo{
		FinalComponent:    componentName,
		Stack:             stack,
		BaseComponentPath: subpath,
		ComponentSection:  map[string]any{},
	}

	_, _, err := resolveWorkdirComponentPath(atmosConfig, info)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrWorkdirProvision),
		"non-ENOENT stat failures must wrap ErrWorkdirProvision")
}
