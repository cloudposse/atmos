package atmos

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// stackConfigLiveFixture is the standard multi-file live stack project used to
// exercise real provenance resolution: a catalog manifest defines the base
// component config, and an importing stack manifest overrides one key, so
// provenance must pick the correct winning file per-path. The returned config
// is routed through currentStackConfig, matching what every tool's Execute
// does before resolving a stack config target; setupLiveStackProject alone
// initializes with processStacks=false, which leaves the stack lookup caches
// the describe pipeline needs unprimed.
func stackConfigLiveFixture(t *testing.T) (*schema.AtmosConfiguration, string) {
	t.Helper()

	rawConfig, stacksDir := setupLiveStackProject(t, map[string]string{
		filepath.Join("catalog", "vpc.yaml"): `components:
  terraform:
    vpc:
      vars:
        region: us-east-1
        foo: base
`,
		"dev.yaml": `import:
  - catalog/vpc

vars:
  stage: dev

components:
  terraform:
    vpc:
      vars:
        foo: dev-override
`,
	})

	atmosConfig, err := currentStackConfig(rawConfig)
	require.NoError(t, err)
	return atmosConfig, stacksDir
}

// TestResolveStackEditTarget_ProvenanceWinner exercises the case provenance
// resolution is built for: vars.foo is declared in both the catalog manifest
// and the importing stack manifest, so the last (importing) manifest must
// win, both for its reported value and its resolved file.
func TestResolveStackEditTarget_ProvenanceWinner(t *testing.T) {
	atmosConfig, stacksDir := stackConfigLiveFixture(t)

	tgt, err := resolveStackEditTarget(&stackEditRequest{atmosConfig: atmosConfig, stack: "dev", component: "vpc", dotPath: "vars.foo"})
	require.NoError(t, err)
	assert.Equal(t, "dev-override", tgt.value)
	assert.Equal(t, filepath.Join(stacksDir, "dev.yaml"), tgt.file)
}

// TestResolveStackEditTarget_InheritedValueDegradesGracefully covers a
// component-relative value that is only ever declared in the imported
// catalog manifest (vars.region), never re-declared by the importing stack.
// Provenance for any path under an import always records a final entry for
// the importing manifest (to track "this file pulled the value in"), so
// PickProvenanceFile's last-entry-wins heuristic reports the importing
// manifest -- but that manifest doesn't literally contain the key, so the
// GetFile verification step in resolveStackTargetByProvenance catches this
// and falls back: get still reports the (unresolvable) location, while
// set/delete require an explicit file. This mirrors the documented caveat in
// cmd/stack/operations.go's resolveTargetByProvenance ("likely inherited or
// imported").
func TestResolveStackEditTarget_InheritedValueDegradesGracefully(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)

	tgt, err := resolveStackEditTarget(&stackEditRequest{atmosConfig: atmosConfig, stack: "dev", component: "vpc", dotPath: "vars.region"})
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", tgt.value, "the best-effort merged value is still reported")
	assert.Empty(t, tgt.file, "no concrete editable node was found")
	assert.Equal(t, "dev.yaml", tgt.provFile, "the importing manifest is still reported as the (unresolvable) location")

	_, err = resolveStackEditTarget(&stackEditRequest{atmosConfig: atmosConfig, stack: "dev", component: "vpc", dotPath: "vars.region", requireEditable: true})
	require.ErrorIs(t, err, errUtils.ErrAIStackConfigPathNotEditable)
}

func TestResolveStackEditTarget_RequireEditable_SetsCorrectFile(t *testing.T) {
	atmosConfig, stacksDir := stackConfigLiveFixture(t)

	tgt, err := resolveStackEditTarget(&stackEditRequest{atmosConfig: atmosConfig, stack: "dev", component: "vpc", dotPath: "vars.foo", requireEditable: true})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(stacksDir, "dev.yaml"), tgt.file)

	created, err := atmosyaml.SetFileWithType(tgt.file, tgt.yqPath, "dev-updated", atmosyaml.TypeString)
	require.NoError(t, err)
	assert.False(t, created)

	got, err := atmosyaml.GetFile(tgt.file, "components.terraform.vpc.vars.foo")
	require.NoError(t, err)
	assert.Equal(t, "dev-updated", got)

	// The sibling manifest (catalog/vpc.yaml) must be untouched.
	catalogContent, err := os.ReadFile(filepath.Join(filepath.Dir(tgt.file), "catalog", "vpc.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(catalogContent), "foo: base")
}

func TestResolveStackEditTarget_ExplicitFileOverride(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)

	dir := t.TempDir()
	file := filepath.Join(dir, "explicit.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`components:
  terraform:
    vpc:
      vars:
        region: eu-west-1
`), 0o644))

	// The explicit file override must win over provenance, even though the
	// live project resolves vars.region to the catalog manifest.
	tgt, err := resolveStackEditTarget(&stackEditRequest{atmosConfig: atmosConfig, stack: "dev", component: "vpc", dotPath: "vars.region", fileOverride: file})
	require.NoError(t, err)
	assert.Equal(t, file, tgt.file)
	assert.Equal(t, "eu-west-1", tgt.value)
}

func TestResolveStackEditTarget_UndefinedPath_RequireEditable(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)

	_, err := resolveStackEditTarget(&stackEditRequest{atmosConfig: atmosConfig, stack: "dev", component: "vpc", dotPath: "vars.does_not_exist", requireEditable: true})
	require.ErrorIs(t, err, errUtils.ErrAIStackConfigPathNotEditable)
}

func TestResolveStackEditTarget_UndefinedPath_ReadOnly(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)

	tgt, err := resolveStackEditTarget(&stackEditRequest{atmosConfig: atmosConfig, stack: "dev", component: "vpc", dotPath: "vars.does_not_exist"})
	require.NoError(t, err)
	assert.Empty(t, tgt.file)
	assert.Empty(t, tgt.value)
}

func TestResolveStackTargetByProvenance_NoEntries(t *testing.T) {
	tgt := &stackEditTarget{inFilePath: "components.terraform.vpc.vars.region"}

	req := stackEditRequest{atmosConfig: &schema.AtmosConfiguration{}, stack: "dev", component: "vpc", dotPath: "vars.region"}
	got, err := resolveStackTargetByProvenance(&req, &exec.DescribeComponentResult{}, tgt)
	require.NoError(t, err)
	assert.Same(t, tgt, got)

	req.requireEditable = true
	_, err = resolveStackTargetByProvenance(&req, &exec.DescribeComponentResult{}, tgt)
	require.ErrorIs(t, err, errUtils.ErrAIStackConfigPathNotEditable)
}

func TestResolveStackTargetByProvenance_VerifyMissingPath(t *testing.T) {
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "deploy", "prod.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(stackFile), 0o755))
	require.NoError(t, os.WriteFile(stackFile, []byte(`components:
  terraform:
    base:
      vars:
        region: us-east-1
`), 0o644))

	mctx := merge.NewMergeContext()
	mctx.EnableProvenance()
	mctx.RecordProvenance("components.terraform.vpc.vars.region", merge.ProvenanceEntry{File: "deploy/prod.yaml", Line: 5})

	tgt := &stackEditTarget{
		inFilePath: "components.terraform.vpc.vars.region",
		yqPath:     "components.terraform.vpc.vars.region",
	}

	req := stackEditRequest{
		atmosConfig: &schema.AtmosConfiguration{StacksBaseAbsolutePath: dir},
		stack:       "prod", component: "vpc", dotPath: "vars.region",
	}
	got, err := resolveStackTargetByProvenance(&req, &exec.DescribeComponentResult{MergeContext: mctx}, tgt)
	require.NoError(t, err)
	assert.Equal(t, "deploy/prod.yaml", got.provFile)
	assert.Empty(t, got.file)

	req.requireEditable = true
	_, err = resolveStackTargetByProvenance(&req, &exec.DescribeComponentResult{MergeContext: mctx}, tgt)
	require.ErrorIs(t, err, errUtils.ErrAIStackConfigPathNotEditable)
}

func TestStackFormatFilesFromProvenance(t *testing.T) {
	mctx := merge.NewMergeContext()
	mctx.EnableProvenance()
	mctx.RecordProvenance("components.terraform.vpc", merge.ProvenanceEntry{File: "deploy/prod.yaml", Line: 3})
	mctx.RecordProvenance("components.terraform.vpc.vars.region", merge.ProvenanceEntry{File: "deploy/prod.yaml", Line: 5})
	mctx.RecordProvenance("components.terraform.vpc.settings.enabled", merge.ProvenanceEntry{File: "deploy/settings.yaml", Line: 8})
	mctx.RecordProvenance("components.terraform.eks.vars.region", merge.ProvenanceEntry{File: "deploy/eks.yaml", Line: 5})

	dir := t.TempDir()
	files, err := stackFormatFilesFromProvenance(
		&schema.AtmosConfiguration{StacksBaseAbsolutePath: dir},
		&exec.DescribeComponentResult{
			ComponentSection: map[string]any{
				cfg.ComponentTypeSectionName: "terraform",
			},
			MergeContext: mctx,
		},
		"prod", "vpc",
	)
	require.NoError(t, err)
	assert.Equal(t, []string{
		filepath.Join(dir, "deploy", "prod.yaml"),
		filepath.Join(dir, "deploy", "settings.yaml"),
	}, files)
}

func TestStackFormatFilesFromProvenance_NoFiles(t *testing.T) {
	mctx := merge.NewMergeContext()
	mctx.EnableProvenance()

	_, err := stackFormatFilesFromProvenance(
		&schema.AtmosConfiguration{StacksBaseAbsolutePath: t.TempDir()},
		&exec.DescribeComponentResult{
			ComponentSection: map[string]any{
				cfg.ComponentTypeSectionName: "terraform",
			},
			MergeContext: mctx,
		},
		"prod", "vpc",
	)
	require.ErrorIs(t, err, errUtils.ErrAIStackConfigPathNotEditable)
}

func TestStackFormatFilesFromProvenance_NilMergeContext(t *testing.T) {
	_, err := stackFormatFilesFromProvenance(
		&schema.AtmosConfiguration{},
		&exec.DescribeComponentResult{},
		"prod", "vpc",
	)
	require.ErrorIs(t, err, errUtils.ErrAIStackConfigPathNotEditable)
}

func TestStackProvenanceFileForPath(t *testing.T) {
	mctx := merge.NewMergeContext()
	mctx.EnableProvenance()
	mctx.RecordProvenance("components.terraform.vpc.vars.region", merge.ProvenanceEntry{File: "deploy/prod.yaml", Line: 5})

	atmosConfig := &schema.AtmosConfiguration{StacksBaseAbsolutePath: "/repo/stacks"}
	result := &exec.DescribeComponentResult{MergeContext: mctx}

	file, ok := stackProvenanceFileForPath(atmosConfig, result, "terraform", "vpc", "vars.region")
	assert.True(t, ok)
	assert.Equal(t, "deploy/prod.yaml", file)

	_, ok = stackProvenanceFileForPath(atmosConfig, result, "terraform", "vpc", "vars.does_not_exist")
	assert.False(t, ok)

	_, ok = stackProvenanceFileForPath(atmosConfig, &exec.DescribeComponentResult{}, "terraform", "vpc", "vars.region")
	assert.False(t, ok)
}

func TestStackRelativePathForDisplay(t *testing.T) {
	// stacksBase must be a real, platform-absolute path (drive letter on Windows) so
	// filepath.IsAbs recognizes it and the filepath.Rel branch is actually exercised.
	// A fabricated root like filepath.Join(string(filepath.Separator), "repo") is
	// rooted but not absolute on Windows without a volume, so it fell through to the
	// no-op branch there while passing on Unix.
	stacksBase := filepath.Join(t.TempDir(), "stacks")
	assert.Equal(t, "deploy/prod.yaml", stackRelativePathForDisplay(filepath.Join(stacksBase, "deploy", "prod.yaml"), stacksBase))
	assert.Equal(t, "relative/prod.yaml", stackRelativePathForDisplay("relative/prod.yaml", stacksBase))
	assert.Equal(t, "relative/prod.yaml", stackRelativePathForDisplay("relative/prod.yaml", ""))
}

func TestDescribeStackComponentForEdit_EnablesProvenance(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)
	require.False(t, atmosConfig.TrackProvenance)

	result, err := describeStackComponentForEdit(atmosConfig, "dev", "vpc")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.MergeContext)
	assert.True(t, atmosConfig.TrackProvenance)

	componentType, _ := result.ComponentSection[cfg.ComponentTypeSectionName].(string)
	assert.Equal(t, "terraform", componentType)
}
