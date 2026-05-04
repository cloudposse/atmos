package component

import (
	"context"
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

// Compile-time guards: if any of these field names are renamed, the build
// breaks immediately and the test author knows to update all assertions.
var _ = schema.ConfigAndStacksInfo{
	BaseComponentPath: "",
	FinalComponent:    "",
	Stack:             "",
	ComponentSection:  map[string]any{},
}

var _ = schema.AtmosConfiguration{
	BasePath: "",
}

// ResolveWorkdirSubpath ─────────────────────────────────────────────────────.

func TestResolveWorkdirSubpath_JoinedPathExists(t *testing.T) {
	workdir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workdir, "modules", "iam-policy"), 0o755))

	got, err := ResolveWorkdirSubpath("modules/iam-policy", workdir)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(workdir, "modules", "iam-policy"), got,
		"joined path exists → use it (issue #2364 fix)")
}

// TestResolveWorkdirSubpath_JoinedPathMissingFallsBack covers the
// inheritance-pointer scenario: metadata.component names an abstract base
// component, the cloned repo has its files at the workdir root, no such
// subdirectory exists. Pre-existing behavior must be preserved.
func TestResolveWorkdirSubpath_JoinedPathMissingFallsBack(t *testing.T) {
	workdir := t.TempDir()

	got, err := ResolveWorkdirSubpath("demo-cluster-codepipeline", workdir)

	require.NoError(t, err)
	assert.Equal(t, workdir, got,
		"missing subpath → fall back to workdir root (inheritance-pointer case)")
}

func TestResolveWorkdirSubpath_EmptySubpathReturnsRoot(t *testing.T) {
	workdir := t.TempDir()
	got, err := ResolveWorkdirSubpath("", workdir)
	require.NoError(t, err)
	assert.Equal(t, workdir, got)
}

// TestResolveWorkdirSubpath_AllowsParentSegment codifies the design decision
// that ".." segments are permitted in metadata.component (issue #2364):
// some upstream modules reference shared files via relative parent paths,
// and metadata.component is YAML-author controlled (same trust class as
// !exec / !template / !terraform.state). The joined sibling directory must
// exist on disk for the resolver to use it.
func TestResolveWorkdirSubpath_AllowsParentSegment(t *testing.T) {
	parent := t.TempDir()
	workdir := filepath.Join(parent, "primary-module")
	require.NoError(t, os.Mkdir(workdir, 0o755))
	sibling := filepath.Join(parent, "sibling-module")
	require.NoError(t, os.Mkdir(sibling, 0o755))

	got, err := ResolveWorkdirSubpath("../sibling-module", workdir)

	require.NoError(t, err)
	assert.Equal(t, sibling, got,
		"\"..\" segment must resolve to the sibling directory when it exists on disk")
}

// TestResolveWorkdirSubpath_RejectsAbsolutePath verifies that an absolute
// metadata.component value fails fast rather than being silently coerced
// into a child of workdirRoot by filepath.Join (metadata.component is
// contractually a relative subpath inside the provisioned workdir).
func TestResolveWorkdirSubpath_RejectsAbsolutePath(t *testing.T) {
	workdir := t.TempDir()

	absSubpath := filepath.Join(workdir, "modules", "iam-policy")
	require.True(t, filepath.IsAbs(absSubpath), "test prerequisite: subpath must be absolute")

	_, err := ResolveWorkdirSubpath(absSubpath, workdir)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrWorkdirProvision),
		"absolute subpath must wrap ErrWorkdirProvision")
	assert.Contains(t, err.Error(), "must be relative",
		"error message must explain the contract violation")
}

// TestResolveWorkdirSubpath_RejectsWindowsVolumeQualified guards against
// Windows volume-qualified relative paths like "C:modules\iam-policy".
// Filepath.IsAbs returns false for such inputs on Windows (they are "drive
// relative", interpreted against the drive's current directory), so
// VolumeName is checked alongside IsAbs to reject them. On non-Windows
// builds filepath.VolumeName always returns "" so this test is skipped.
func TestResolveWorkdirSubpath_RejectsWindowsVolumeQualified(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("filepath.VolumeName is OS-specific; meaningful only on Windows")
	}
	workdir := t.TempDir()

	_, err := ResolveWorkdirSubpath(`C:modules\iam-policy`, workdir)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrWorkdirProvision),
		"volume-qualified relative subpath must wrap ErrWorkdirProvision")
	assert.Contains(t, err.Error(), "must be relative",
		"error message must explain the contract violation")
}

// TestResolveWorkdirSubpath_RejectsAbsolutePathOutsideWorkdir is the
// adversarial counterpart: an absolute path pointing outside the workdir
// must also be rejected (not silently joined into a child of workdirRoot).
func TestResolveWorkdirSubpath_RejectsAbsolutePathOutsideWorkdir(t *testing.T) {
	workdir := t.TempDir()

	// Use a path guaranteed to be absolute on both Unix and Windows.
	var absOutside string
	if runtime.GOOS == "windows" {
		absOutside = `C:\Windows\System32`
	} else {
		absOutside = "/etc/passwd"
	}

	_, err := ResolveWorkdirSubpath(absOutside, workdir)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrWorkdirProvision),
		"absolute subpath outside workdir must wrap ErrWorkdirProvision")
}

func TestResolveWorkdirSubpath_RegularFileAtCandidate(t *testing.T) {
	workdir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "exports"), []byte("not a dir"), 0o644))

	_, err := ResolveWorkdirSubpath("exports", workdir)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrWorkdirProvision),
		"non-directory at candidate must wrap ErrWorkdirProvision")
}

// ApplyWorkdirSubpathToSection ──────────────────────────────────────────────.

func TestApplyWorkdirSubpathToSection_JoinsSubpath(t *testing.T) {
	workdirRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workdirRoot, "modules", "iam-policy"), 0o755))
	info := &schema.ConfigAndStacksInfo{
		BaseComponentPath: "modules/iam-policy",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: workdirRoot,
		},
	}

	got, err := ApplyWorkdirSubpathToSection(info)
	require.NoError(t, err)

	expected := filepath.Join(workdirRoot, "modules", "iam-policy")
	assert.Equal(t, expected, got)
	assert.Equal(t, expected, info.ComponentSection[provWorkdir.WorkdirPathKey],
		"WorkdirPathKey should be mutated in place to the joined subpath")
	_, applied := info.ComponentSection[workdirSubpathAppliedKey].(subpathAppliedMarker)
	assert.True(t, applied, "sentinel marker should be set")
}

// TestApplyWorkdirSubpathToSection_InheritancePointerPreservesRoot is the
// regression guard for the case where metadata.component is used as an
// inheritance/identity pointer (an abstract base component name) rather than
// as a real subdirectory. The cloned repo has files at its root and no
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

	got, err := ApplyWorkdirSubpathToSection(info)
	require.NoError(t, err)

	assert.Equal(t, workdirRoot, got, "missing subpath must fall back to workdir root")
	assert.Equal(t, workdirRoot, info.ComponentSection[provWorkdir.WorkdirPathKey],
		"WorkdirPathKey should remain at the workdir root for inheritance-pointer use")
	_, applied := info.ComponentSection[workdirSubpathAppliedKey].(subpathAppliedMarker)
	assert.True(t, applied, "sentinel marker is set even when no join happens, so repeat calls are no-ops")
}

func TestApplyWorkdirSubpathToSection_NoWorkdirPathKey(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		BaseComponentPath: "exports",
		ComponentSection:  map[string]any{},
	}

	got, err := ApplyWorkdirSubpathToSection(info)
	require.NoError(t, err)

	assert.Empty(t, got)
	_, mutated := info.ComponentSection[provWorkdir.WorkdirPathKey]
	assert.False(t, mutated, "must not introduce WorkdirPathKey when it was absent")
	_, applied := info.ComponentSection[workdirSubpathAppliedKey]
	assert.False(t, applied, "must not set the sentinel when nothing was joined")
}

func TestApplyWorkdirSubpathToSection_EmptyWorkdirPath(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		BaseComponentPath: "exports",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: "",
		},
	}

	got, err := ApplyWorkdirSubpathToSection(info)
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

	first, err := ApplyWorkdirSubpathToSection(info)
	require.NoError(t, err)
	assert.Equal(t, expected, first)

	second, err := ApplyWorkdirSubpathToSection(info)
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

	first, err := ApplyWorkdirSubpathToSection(info)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(workdirRoot, "exports"), first)

	delete(info.ComponentSection, workdirSubpathAppliedKey)

	second, err := ApplyWorkdirSubpathToSection(info)
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
				provWorkdir.WorkdirPathKey: workdirRoot,
				workdirSubpathAppliedKey:   forged,
			},
		}

		got, err := ApplyWorkdirSubpathToSection(info)
		require.NoError(t, err)

		assert.Equal(t, filepath.Join(workdirRoot, "exports"), got,
			"forged sentinel %T(%v) must not bypass the join", forged, forged)
	}
}

// BuildAndResolveWorkdirPath ────────────────────────────────────────────────.

func TestBuildAndResolveWorkdirPath_ExistingDir(t *testing.T) {
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

	candidate, exists, err := BuildAndResolveWorkdirPath(atmosConfig, info, cfg.TerraformComponentType)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, expectedCandidate, candidate)
}

// TestBuildAndResolveWorkdirPath_AllComponentTypes verifies the componentType
// parameter is honored across all four executors: each must resolve under
// .workdir/<componentType>/, not curve-fitted to terraform. This is the
// component-type parity check osterman called for in PR #2371.
func TestBuildAndResolveWorkdirPath_AllComponentTypes(t *testing.T) {
	for _, componentType := range []string{
		cfg.TerraformComponentType,
		cfg.HelmfileComponentType,
		cfg.PackerComponentType,
		cfg.AnsibleComponentType,
	} {
		t.Run(componentType, func(t *testing.T) {
			basePath := t.TempDir()
			stack := "dev"
			componentName := "my-component"

			expectedRoot := filepath.Join(basePath, provWorkdir.WorkdirPath, componentType, stack+"-"+componentName)
			require.NoError(t, os.MkdirAll(expectedRoot, 0o755))

			atmosConfig := &schema.AtmosConfiguration{BasePath: basePath}
			info := &schema.ConfigAndStacksInfo{
				FinalComponent:   componentName,
				Stack:            stack,
				ComponentSection: map[string]any{},
			}

			candidate, exists, err := BuildAndResolveWorkdirPath(atmosConfig, info, componentType)
			require.NoError(t, err)
			assert.True(t, exists)
			assert.Equal(t, expectedRoot, candidate,
				"%s component must resolve under .workdir/%s/", componentType, componentType)
			assert.Contains(t, candidate, string(filepath.Separator)+componentType+string(filepath.Separator),
				"resolved path must include the %s subdirectory", componentType)
		})
	}
}

// TestBuildAndResolveWorkdirPath_AllComponentTypesWithSubpath verifies
// metadata.component subpath join works uniformly across all four executors —
// not curve-fitted to terraform.
func TestBuildAndResolveWorkdirPath_AllComponentTypesWithSubpath(t *testing.T) {
	for _, componentType := range []string{
		cfg.TerraformComponentType,
		cfg.HelmfileComponentType,
		cfg.PackerComponentType,
		cfg.AnsibleComponentType,
	} {
		t.Run(componentType, func(t *testing.T) {
			basePath := t.TempDir()
			stack := "dev"
			componentName := "my-component"
			// Production input mirrors a YAML-derived metadata.component value
			// ("modules/foo"); compose the expected on-disk path with split
			// segments so we never feed forward slashes into filepath.Join.
			subpathYAML := "modules/foo"

			workdirRoot := filepath.Join(basePath, provWorkdir.WorkdirPath, componentType, stack+"-"+componentName)
			expectedCandidate := filepath.Join(workdirRoot, "modules", "foo")
			require.NoError(t, os.MkdirAll(expectedCandidate, 0o755))

			atmosConfig := &schema.AtmosConfiguration{BasePath: basePath}
			info := &schema.ConfigAndStacksInfo{
				FinalComponent:    componentName,
				Stack:             stack,
				BaseComponentPath: subpathYAML,
				ComponentSection:  map[string]any{},
			}

			candidate, exists, err := BuildAndResolveWorkdirPath(atmosConfig, info, componentType)
			require.NoError(t, err)
			assert.True(t, exists)
			assert.Equal(t, expectedCandidate, candidate,
				"%s must honor metadata.component subpath %q (issue #2364)", componentType, subpathYAML)
		})
	}
}

// TestBuildAndResolveWorkdirPath_InheritancePointerFallsBack covers the
// case where metadata.component is an inheritance pointer: the workdir root
// exists (provisioned earlier) but the named subdirectory does not. The
// resolver returns the workdir root with exists=true so plan-diff and
// verify-plan can still locate the planfile.
func TestBuildAndResolveWorkdirPath_InheritancePointerFallsBack(t *testing.T) {
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

	candidate, exists, err := BuildAndResolveWorkdirPath(atmosConfig, info, cfg.TerraformComponentType)
	require.NoError(t, err)
	assert.True(t, exists, "workdir root exists; resolver must return exists=true")
	assert.Equal(t, root, candidate, "inheritance-pointer subpath missing → fall back to workdir root")
}

// TestBuildAndResolveWorkdirPath_NonExistentDir returns exists=false, no
// error so callers retain their fallback path. Workdir not provisioned yet.
func TestBuildAndResolveWorkdirPath_NonExistentDir(t *testing.T) {
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

	candidate, exists, err := BuildAndResolveWorkdirPath(atmosConfig, info, cfg.TerraformComponentType)
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Equal(t, expectedRoot, candidate,
		"workdir not provisioned → ResolveWorkdirSubpath falls back to root and stat returns ENOENT")
}

// TestBuildAndResolveWorkdirPath_RegularFileAtCandidate surfaces a
// non-directory at the candidate path as a wrapped error rather than a silent
// exists=false, so corrupt state is not masked.
func TestBuildAndResolveWorkdirPath_RegularFileAtCandidate(t *testing.T) {
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

	_, exists, err := BuildAndResolveWorkdirPath(atmosConfig, info, cfg.TerraformComponentType)
	require.Error(t, err)
	assert.False(t, exists)
	assert.True(t, errors.Is(err, errUtils.ErrWorkdirProvision),
		"non-directory at candidate must wrap ErrWorkdirProvision")
}

func TestBuildAndResolveWorkdirPath_StatErrorPropagates(t *testing.T) {
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

	_, _, err := BuildAndResolveWorkdirPath(atmosConfig, info, cfg.TerraformComponentType)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrWorkdirProvision),
		"non-ENOENT stat failures must wrap ErrWorkdirProvision")
}

// ProvisionAndResolveComponentPath ──────────────────────────────────────────.

// TestProvisionAndResolveComponentPath_NoSourceReturnsFallback verifies the
// short-circuit when no source is configured: the orchestrator must skip
// AutoProvisionSource entirely and report whether the fallback exists.
func TestProvisionAndResolveComponentPath_NoSourceReturnsFallback(t *testing.T) {
	componentDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: componentDir}
	info := &schema.ConfigAndStacksInfo{
		FinalComponent:   "no-source-component",
		Stack:            "dev",
		ComponentSection: map[string]any{},
	}

	got, exists, err := ProvisionAndResolveComponentPath(
		context.Background(), atmosConfig, info, cfg.TerraformComponentType, componentDir,
	)

	require.NoError(t, err)
	assert.True(t, exists, "fallback dir exists on disk → exists=true")
	assert.Equal(t, componentDir, got, "no source declared → return fallback verbatim")
}

// TestProvisionAndResolveComponentPath_NoSourceFallbackIsRegularFile verifies
// the no-source path surfaces an explicit error (rather than silently
// reporting exists=false) when the fallback path exists on disk but is a
// regular file instead of a directory. The previous u.IsDirectory-based
// implementation collapsed this case into ENOENT, hiding corrupt/wrong-type
// state from callers.
func TestProvisionAndResolveComponentPath_NoSourceFallbackIsRegularFile(t *testing.T) {
	tempDir := t.TempDir()
	regularFile := filepath.Join(tempDir, "components-as-a-file")
	require.NoError(t, os.WriteFile(regularFile, []byte("not a directory"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{BasePath: tempDir}
	info := &schema.ConfigAndStacksInfo{
		FinalComponent:   "regular-file-fallback",
		Stack:            "dev",
		ComponentSection: map[string]any{},
	}

	_, exists, err := ProvisionAndResolveComponentPath(
		context.Background(), atmosConfig, info, cfg.TerraformComponentType, regularFile,
	)

	require.Error(t, err)
	assert.False(t, exists)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidComponent),
		"non-directory fallback must wrap ErrInvalidComponent (matches the pattern in cmd/describe_component.go)")
	assert.Contains(t, err.Error(), "exists but is not a directory",
		"error message must distinguish 'exists but not a dir' from ENOENT")
}

// TestProvisionAndResolveComponentPath_NoSourceMissingDir verifies the
// no-source path correctly reports exists=false when the fallback directory
// has not been created.
func TestProvisionAndResolveComponentPath_NoSourceMissingDir(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "does-not-exist")
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	info := &schema.ConfigAndStacksInfo{
		FinalComponent:   "missing",
		Stack:            "dev",
		ComponentSection: map[string]any{},
	}

	got, exists, err := ProvisionAndResolveComponentPath(
		context.Background(), atmosConfig, info, cfg.TerraformComponentType, missingDir,
	)

	require.NoError(t, err, "missing dir is not an error — caller decides what to do")
	assert.False(t, exists)
	assert.Equal(t, missingDir, got)
}
