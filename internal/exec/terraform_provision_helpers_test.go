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

// resolveWorkdirSubpath ─────────────────────────────────────────────────────.

func TestResolveWorkdirSubpath_JoinedPathExists(t *testing.T) {
	workdir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workdir, "modules", "iam-policy"), 0o755))

	got, err := resolveWorkdirSubpath("modules/iam-policy", workdir)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(workdir, "modules", "iam-policy"), got,
		"joined path exists → use it (issue #2364 fix)")
}

// TestResolveWorkdirSubpath_JoinedPathMissingFallsBack covers the
// inheritance-pointer scenario: metadata.component names an abstract base
// component, the cloned repo has its .tf files at the workdir root, no such
// subdirectory exists. Pre-existing behavior must be preserved.
func TestResolveWorkdirSubpath_JoinedPathMissingFallsBack(t *testing.T) {
	workdir := t.TempDir()

	got, err := resolveWorkdirSubpath("demo-cluster-codepipeline", workdir)

	require.NoError(t, err)
	assert.Equal(t, workdir, got,
		"missing subpath → fall back to workdir root (inheritance-pointer case)")
}

func TestResolveWorkdirSubpath_EmptySubpathReturnsRoot(t *testing.T) {
	workdir := t.TempDir()
	got, err := resolveWorkdirSubpath("", workdir)
	require.NoError(t, err)
	assert.Equal(t, workdir, got)
}

func TestResolveWorkdirSubpath_RegularFileAtCandidate(t *testing.T) {
	workdir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "exports"), []byte("not a dir"), 0o644))

	_, err := resolveWorkdirSubpath("exports", workdir)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrWorkdirProvision),
		"non-directory at candidate must wrap ErrWorkdirProvision")
}

// applyWorkdirSubpathToSection ──────────────────────────────────────────────.

func TestApplyWorkdirSubpathToSection_JoinsSubpath(t *testing.T) {
	workdirRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workdirRoot, "modules", "iam-policy"), 0o755))
	info := &schema.ConfigAndStacksInfo{
		BaseComponentPath: "modules/iam-policy",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: workdirRoot,
		},
	}

	got, err := applyWorkdirSubpathToSection(info)
	require.NoError(t, err)

	expected := filepath.Join(workdirRoot, "modules", "iam-policy")
	assert.Equal(t, expected, got)
	assert.Equal(t, expected, info.ComponentSection[provWorkdir.WorkdirPathKey],
		"WorkdirPathKey should be mutated in place to the joined subpath")
	_, applied := info.ComponentSection[provWorkdir.WorkdirSubpathAppliedKey].(workdirSubpathAppliedMarker)
	assert.True(t, applied, "sentinel marker should be set")
}

// TestApplyWorkdirSubpathToSection_InheritancePointerPreservesRoot is the
// regression guard for the case where metadata.component is used as an
// inheritance/identity pointer (an abstract base component name) rather than
// as a real subdirectory. The cloned repo has .tf files at its root and no
// matching subdirectory; WorkdirPathKey must stay at the workdir root.
func TestApplyWorkdirSubpathToSection_InheritancePointerPreservesRoot(t *testing.T) {
	workdirRoot := t.TempDir()
	// Note: no subdirectory is created for "demo-cluster-codepipeline".
	info := &schema.ConfigAndStacksInfo{
		BaseComponentPath: "demo-cluster-codepipeline",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: workdirRoot,
		},
	}

	got, err := applyWorkdirSubpathToSection(info)
	require.NoError(t, err)

	assert.Equal(t, workdirRoot, got, "missing subpath must fall back to workdir root")
	assert.Equal(t, workdirRoot, info.ComponentSection[provWorkdir.WorkdirPathKey],
		"WorkdirPathKey should remain at the workdir root for inheritance-pointer use")
	_, applied := info.ComponentSection[provWorkdir.WorkdirSubpathAppliedKey].(workdirSubpathAppliedMarker)
	assert.True(t, applied, "sentinel marker is set even when no join happens, so repeat calls are no-ops")
}

func TestApplyWorkdirSubpathToSection_NoWorkdirPathKey(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		BaseComponentPath: "exports",
		ComponentSection:  map[string]any{},
	}

	got, err := applyWorkdirSubpathToSection(info)
	require.NoError(t, err)

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

	got, err := applyWorkdirSubpathToSection(info)
	require.NoError(t, err)

	assert.Empty(t, got)
	assert.Equal(t, "", info.ComponentSection[provWorkdir.WorkdirPathKey])
}

// TestApplyWorkdirSubpathToSection_DoubleCallAppliesOnce verifies idempotency:
// the second call must not produce <workdir>/<subpath>/<subpath>.
func TestApplyWorkdirSubpathToSection_DoubleCallAppliesOnce(t *testing.T) {
	workdirRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workdirRoot, "exports"), 0o755))
	info := &schema.ConfigAndStacksInfo{
		BaseComponentPath: "exports",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: workdirRoot,
		},
	}
	expected := filepath.Join(workdirRoot, "exports")

	first, err := applyWorkdirSubpathToSection(info)
	require.NoError(t, err)
	assert.Equal(t, expected, first)

	second, err := applyWorkdirSubpathToSection(info)
	require.NoError(t, err)
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
	require.NoError(t, os.MkdirAll(filepath.Join(workdirRoot, "exports"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(workdirRoot, "exports", "exports"), 0o755))
	info := &schema.ConfigAndStacksInfo{
		BaseComponentPath: "exports",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: workdirRoot,
		},
	}

	first, err := applyWorkdirSubpathToSection(info)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(workdirRoot, "exports"), first)

	delete(info.ComponentSection, provWorkdir.WorkdirSubpathAppliedKey)

	second, err := applyWorkdirSubpathToSection(info)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(workdirRoot, "exports", "exports"), second,
		"with the sentinel removed the second call must re-join, proving the sentinel is the gate")
}

// TestApplyWorkdirSubpathToSection_UserYAMLCannotForgeSentinel guards against
// a YAML-author setting `_workdir_subpath_applied: <anything>` and silently
// bypassing the join. The sentinel must be a typed marker, not a presence
// check.
func TestApplyWorkdirSubpathToSection_UserYAMLCannotForgeSentinel(t *testing.T) {
	workdirRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workdirRoot, "exports"), 0o755))
	for _, forged := range []any{true, "applied", 1, map[string]any{}} {
		info := &schema.ConfigAndStacksInfo{
			BaseComponentPath: "exports",
			ComponentSection: map[string]any{
				provWorkdir.WorkdirPathKey:           workdirRoot,
				provWorkdir.WorkdirSubpathAppliedKey: forged,
			},
		}

		got, err := applyWorkdirSubpathToSection(info)
		require.NoError(t, err)

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

// TestResolveWorkdirComponentPath_InheritancePointerFallsBack covers the
// case where metadata.component is an inheritance pointer: the workdir root
// exists (provisioned earlier) but the named subdirectory does not. The
// resolver returns the workdir root with exists=true so plan-diff and
// verify-plan can still locate the planfile.
func TestResolveWorkdirComponentPath_InheritancePointerFallsBack(t *testing.T) {
	basePath := t.TempDir()
	stack := "dev"
	componentName := "demo-cluster-codepipeline-iac"
	root := filepath.Join(basePath, provWorkdir.WorkdirPath, cfg.TerraformComponentType, stack+"-"+componentName)
	require.NoError(t, os.MkdirAll(root, 0o755))
	// Note: no "demo-cluster-codepipeline" subdirectory inside root.

	atmosConfig := &schema.AtmosConfiguration{BasePath: basePath}
	info := &schema.ConfigAndStacksInfo{
		FinalComponent:    componentName,
		Stack:             stack,
		BaseComponentPath: "demo-cluster-codepipeline",
		ComponentSection:  map[string]any{},
	}

	candidate, exists, err := resolveWorkdirComponentPath(atmosConfig, info)
	require.NoError(t, err)
	assert.True(t, exists, "workdir root exists; resolver must return exists=true")
	assert.Equal(t, root, candidate, "inheritance-pointer subpath missing → fall back to workdir root")
}

// TestResolveWorkdirComponentPath_NonExistentDir returns exists=false, no
// error so callers retain their fallback path. Workdir not provisioned yet.
func TestResolveWorkdirComponentPath_NonExistentDir(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	info := &schema.ConfigAndStacksInfo{
		FinalComponent:    "missing-component",
		Stack:             "dev",
		BaseComponentPath: "exports",
		ComponentSection:  map[string]any{},
	}
	expectedRoot := filepath.Join(
		atmosConfig.BasePath,
		provWorkdir.WorkdirPath,
		cfg.TerraformComponentType,
		info.Stack+"-"+info.FinalComponent,
	)

	candidate, exists, err := resolveWorkdirComponentPath(atmosConfig, info)
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Equal(t, expectedRoot, candidate,
		"workdir not provisioned → resolveWorkdirSubpath falls back to root and stat returns ENOENT")
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
