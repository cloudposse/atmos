package stack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	listpkg "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

func TestResolveTargetByProvenance(t *testing.T) {
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "deploy", "prod.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(stackFile), 0o755))
	require.NoError(t, os.WriteFile(stackFile, []byte(`components:
  terraform:
    vpc:
      vars:
        region: us-east-1
`), 0o644))

	flagComponent = "vpc"
	flagStack = "prod"
	t.Cleanup(func() {
		flagComponent = ""
		flagStack = ""
	})

	mctx := merge.NewMergeContext()
	mctx.EnableProvenance()
	mctx.RecordProvenance("components.terraform.vpc.vars.region", merge.ProvenanceEntry{File: "deploy/prod.yaml", Line: 5})

	tgt := &editTarget{
		inFilePath: "components.terraform.vpc.vars.region",
		yqPath:     "components.terraform.vpc.vars.region",
	}
	got, err := resolveTargetByProvenance(
		&schema.AtmosConfiguration{StacksBaseAbsolutePath: dir},
		&exec.DescribeComponentResult{MergeContext: mctx},
		tgt,
		"vars.region",
		true,
	)
	require.NoError(t, err)
	assert.Equal(t, stackFile, got.file)
	assert.Equal(t, "deploy/prod.yaml", got.provFile)
	assert.Equal(t, 5, got.provLine)
}

func TestResolveTargetByProvenance_NoProvenance(t *testing.T) {
	flagComponent = "vpc"
	flagStack = "prod"
	t.Cleanup(func() {
		flagComponent = ""
		flagStack = ""
	})

	tgt := &editTarget{inFilePath: "components.terraform.vpc.vars.region"}
	got, err := resolveTargetByProvenance(&schema.AtmosConfiguration{}, &exec.DescribeComponentResult{}, tgt, "vars.region", false)
	require.NoError(t, err)
	assert.Same(t, tgt, got)

	_, err = resolveTargetByProvenance(&schema.AtmosConfiguration{}, &exec.DescribeComponentResult{}, tgt, "vars.region", true)
	require.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
}

func TestResolveTargetByProvenance_VerifyMissingPath(t *testing.T) {
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

	tgt := &editTarget{
		inFilePath: "components.terraform.vpc.vars.region",
		yqPath:     "components.terraform.vpc.vars.region",
	}

	got, err := resolveTargetByProvenance(
		&schema.AtmosConfiguration{StacksBaseAbsolutePath: dir},
		&exec.DescribeComponentResult{MergeContext: mctx},
		tgt,
		"vars.region",
		false,
	)
	require.NoError(t, err)
	assert.Equal(t, "deploy/prod.yaml", got.provFile)
	assert.Empty(t, got.file)

	_, err = resolveTargetByProvenance(
		&schema.AtmosConfiguration{StacksBaseAbsolutePath: dir},
		&exec.DescribeComponentResult{MergeContext: mctx},
		tgt,
		"vars.region",
		true,
	)
	require.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
}

func TestBuildStackConfigRowsFromDescribe(t *testing.T) {
	mctx := merge.NewMergeContext()
	mctx.EnableProvenance()
	mctx.RecordProvenance("components.terraform.vpc.vars", merge.ProvenanceEntry{File: "deploy/prod.yaml", Line: 4})
	mctx.RecordProvenance("components.terraform.vpc.vars.region", merge.ProvenanceEntry{File: "deploy/prod.yaml", Line: 5})
	mctx.RecordProvenance("components.terraform.vpc.settings.enabled", merge.ProvenanceEntry{File: "deploy/settings.yaml", Line: 8})

	rows, err := buildStackConfigRowsFromDescribe(
		&schema.AtmosConfiguration{StacksBaseAbsolutePath: "/repo/stacks"},
		&exec.DescribeComponentResult{
			ComponentSection: map[string]any{
				cfg.ComponentTypeSectionName: "terraform",
				"vars": map[string]any{
					"region": "us-east-1",
				},
				"settings": map[string]any{
					"enabled": true,
				},
			},
			MergeContext: mctx,
		},
		"vpc",
	)
	require.NoError(t, err)
	require.Contains(t, rows, listpkg.PathRow{File: "deploy/prod.yaml", Path: "vars", Type: "object", Value: "{1 keys}"})
	require.Contains(t, rows, listpkg.PathRow{File: "deploy/prod.yaml", Path: "vars.region", Type: "string", Value: "us-east-1"})
	require.Contains(t, rows, listpkg.PathRow{File: "deploy/settings.yaml", Path: "settings.enabled", Type: "bool", Value: "true"})
	require.NotContains(t, rows, listpkg.PathRow{File: "deploy/prod.yaml", Path: cfg.ComponentTypeSectionName, Type: "string", Value: "terraform"})
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
		"vpc",
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

	flagStack = "prod"
	t.Cleanup(func() {
		flagStack = ""
	})

	_, err := stackFormatFilesFromProvenance(
		&schema.AtmosConfiguration{StacksBaseAbsolutePath: t.TempDir()},
		&exec.DescribeComponentResult{
			ComponentSection: map[string]any{
				cfg.ComponentTypeSectionName: "terraform",
			},
			MergeContext: mctx,
		},
		"vpc",
	)
	require.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
}

func TestCommandProvider(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	SetAtmosConfig(cfg)
	t.Cleanup(func() {
		SetAtmosConfig(nil)
	})
	assert.Same(t, cfg, atmosConfigPtr)

	provider := &CommandProvider{}
	assert.Same(t, stackCmd, provider.GetCommand())
	assert.Equal(t, "stack", provider.GetName())
	assert.Equal(t, "Stack Introspection", provider.GetGroup())
	assert.Nil(t, provider.GetFlagsBuilder())
	assert.Nil(t, provider.GetPositionalArgsBuilder())
	assert.Nil(t, provider.GetCompatibilityFlags())
	assert.Nil(t, provider.GetAliases())
	assert.False(t, provider.IsExperimental())
}

// resetEditFlags clears the shared package-level flag vars used by the stack
// edit subcommands and restores their previous values on cleanup, so tests
// that mutate flagStack/flagComponent/flagFile/flagType don't leak state.
func resetEditFlags(t *testing.T) {
	t.Helper()

	oldStack, oldComponent, oldFile, oldType := flagStack, flagComponent, flagFile, flagType
	t.Cleanup(func() {
		flagStack = oldStack
		flagComponent = oldComponent
		flagFile = oldFile
		flagType = oldType
	})
}

// chdirToValidAtmosProject copies the minimal "basic" atmos project fixture
// into a fresh t.TempDir() and chdirs there, clearing cross-test caches.
// The function resolveEditTarget always calls describeComponentForEdit()
// (which runs cfg.InitCliConfig and a real describe) before checking
// flagFile, even on the explicit-file path, so any test invoking
// runStackGet/runStackSet/runStackDelete needs a valid atmos.yaml + stacks
// tree *and* a component/stack pair that actually resolves via describe,
// regardless of which manifest --file targets. Operating on a copy (rather
// than chdir'ing straight into the checked-in fixture) is required because
// runStackSet/runStackDelete/runStackFormat mutate files in place, and
// atmosyaml's format normalization does not always round-trip byte-for-byte
// (see the "provenance" set/delete tests), which would otherwise leave the
// committed fixture dirtied after a test run.
// Callers must set flagComponent/flagStack to "mycomponent"/"nonprod" (the
// fixture's only defined component/stack pair) unless testing the describe
// error path itself.
func chdirToValidAtmosProject(t *testing.T) {
	t.Helper()

	exec.ClearBaseComponentConfigCache()
	exec.ClearMergeContexts()
	exec.ClearLastMergeContext()
	exec.ClearFileContentCache()

	require.NoError(t, os.Unsetenv("ATMOS_CLI_CONFIG_PATH"))
	require.NoError(t, os.Unsetenv("ATMOS_BASE_PATH"))

	wd, err := os.Getwd()
	require.NoError(t, err)
	src := filepath.Join(wd, "..", "..", "tests", "fixtures", "scenarios", "basic")
	dst := t.TempDir()
	require.NoError(t, copyDirRecursive(src, dst))

	t.Chdir(dst)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
}

// copyDirRecursive copies a directory tree (files and subdirectories) from
// src to dst, preserving each file's permissions. Used to give each test its
// own disposable copy of a checked-in fixture rather than mutating the
// tracked files directly.
func copyDirRecursive(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode())
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, info.Mode())
	})
}

func TestRunStackGet_ExplicitFile(t *testing.T) {
	resetEditFlags(t)
	chdirToValidAtmosProject(t)
	stdout := initStackConfigTestWriter(t)

	dir := t.TempDir()
	file := filepath.Join(dir, "prod.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`components:
  terraform:
    mycomponent:
      vars:
        region: us-east-1
`), 0o644))

	flagStack = "nonprod"
	flagComponent = "mycomponent"
	flagFile = file

	require.NoError(t, runStackGet([]string{"vars.region"}))
	// The explicit --file value must win over the fixture's own provenance
	// value ("foo nonprod override"/etc.), proving the explicit-file branch
	// actually read from the temp file rather than the describe result.
	assert.Equal(t, "us-east-1\n", stdout.String())
}

func TestRunStackGet_ExplicitFile_PathNotFound(t *testing.T) {
	resetEditFlags(t)
	chdirToValidAtmosProject(t)
	stdout := initStackConfigTestWriter(t)

	dir := t.TempDir()
	file := filepath.Join(dir, "prod.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`components:
  terraform:
    mycomponent:
      vars:
        region: us-east-1
`), 0o644))

	flagStack = "nonprod"
	flagComponent = "mycomponent"
	flagFile = file

	// resolveEditTarget for get is best-effort: an unresolvable dot-path still
	// succeeds with an empty value rather than erroring, since GetFile's error
	// is swallowed for the read-only get case.
	require.NoError(t, runStackGet([]string{"vars.does_not_exist"}))
	assert.Equal(t, "\n", stdout.String())
}

func TestRunStackSet_ExplicitFile(t *testing.T) {
	resetEditFlags(t)
	chdirToValidAtmosProject(t)

	dir := t.TempDir()
	file := filepath.Join(dir, "prod.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`components:
  terraform:
    mycomponent:
      vars:
        region: us-east-1
`), 0o644))

	flagStack = "nonprod"
	flagComponent = "mycomponent"
	flagFile = file
	flagType = atmosyaml.TypeString

	require.NoError(t, runStackSet([]string{"vars.region", "us-west-2"}))

	got, err := atmosyaml.GetFile(file, "components.terraform.mycomponent.vars.region")
	require.NoError(t, err)
	assert.Equal(t, "us-west-2", got)
}

func TestRunStackSet_ExplicitFile_InvalidType(t *testing.T) {
	resetEditFlags(t)
	chdirToValidAtmosProject(t)

	dir := t.TempDir()
	file := filepath.Join(dir, "prod.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`components:
  terraform:
    mycomponent:
      vars:
        region: us-east-1
`), 0o644))

	flagStack = "nonprod"
	flagComponent = "mycomponent"
	flagFile = file
	flagType = "not-a-real-type"

	err := runStackSet([]string{"vars.region", "us-west-2"})
	require.ErrorIs(t, err, atmosyaml.ErrInvalidYAMLExpression)
}

func TestRunStackDelete_ExplicitFile(t *testing.T) {
	resetEditFlags(t)
	chdirToValidAtmosProject(t)

	dir := t.TempDir()
	file := filepath.Join(dir, "prod.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`components:
  terraform:
    mycomponent:
      vars:
        region: us-east-1
        az: a
`), 0o644))

	flagStack = "nonprod"
	flagComponent = "mycomponent"
	flagFile = file

	require.NoError(t, runStackDelete([]string{"vars.region"}))

	_, err := atmosyaml.GetFile(file, "components.terraform.mycomponent.vars.region")
	require.Error(t, err)
	// The sibling key must survive the deletion (src->result isolation check).
	got, err := atmosyaml.GetFile(file, "components.terraform.mycomponent.vars.az")
	require.NoError(t, err)
	assert.Equal(t, "a", got)
}

func TestRunStackDelete_ExplicitFile_MissingComponent(t *testing.T) {
	resetEditFlags(t)
	chdirToValidAtmosProject(t)

	dir := t.TempDir()
	file := filepath.Join(dir, "prod.yaml")
	// This manifest defines "other", not "mycomponent" (the flagComponent
	// below). Unlike provenance resolution, the explicit-file branch of
	// resolveEditTarget never verifies the path exists, and yq's del() is a
	// no-op on a missing path, so this must succeed leaving the file
	// unchanged rather than error.
	original := `components:
  terraform:
    other:
      vars:
        region: us-east-1
`
	require.NoError(t, os.WriteFile(file, []byte(original), 0o644))

	flagStack = "nonprod"
	flagComponent = "mycomponent"
	flagFile = file

	require.NoError(t, runStackDelete([]string{"vars.region"}))

	got, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, original, string(got))
}

func TestRunStackFormat_ExplicitFile(t *testing.T) {
	resetEditFlags(t)
	chdirToValidAtmosProject(t)

	dir := t.TempDir()
	file := filepath.Join(dir, "prod.yaml")
	// Deliberately messy formatting (extra blank lines / spacing) so FormatFile
	// has something to normalize; assert the content actually changes.
	messy := "components:\n  terraform:\n    vpc:\n      vars:\n        region:    us-east-1\n\n\n"
	require.NoError(t, os.WriteFile(file, []byte(messy), 0o644))

	flagStack = "nonprod"
	flagComponent = "mycomponent"
	flagFile = file

	require.NoError(t, runStackFormat())

	got, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.NotEqual(t, messy, string(got))
	assert.Contains(t, string(got), "region: us-east-1")
}

func TestRunStackFormat_ExplicitFile_MissingFile(t *testing.T) {
	resetEditFlags(t)

	flagStack = "nonprod"
	flagComponent = "mycomponent"
	flagFile = filepath.Join(t.TempDir(), "does-not-exist.yaml")

	err := runStackFormat()
	require.Error(t, err)
}

func TestResolveStackFormatFiles_ExplicitFileShortCircuit(t *testing.T) {
	resetEditFlags(t)

	// The explicit-file branch must short-circuit before any provenance/describe
	// work happens, so it should succeed even though flagStack/flagComponent are
	// left unset (which would otherwise make describeComponentForEdit fail).
	flagFile = "some/manifest.yaml"
	flagStack = ""
	flagComponent = ""

	files, err := resolveStackFormatFiles()
	require.NoError(t, err)
	assert.Equal(t, []string{"some/manifest.yaml"}, files)
}

func TestResolveStackFormatFiles_DescribeError(t *testing.T) {
	resetEditFlags(t)

	dir := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
	require.NoError(t, os.Chdir(dir))
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	t.Setenv("ATMOS_BASE_PATH", ".")

	flagFile = ""
	flagStack = "does-not-matter"
	flagComponent = "does-not-matter"

	_, err = resolveStackFormatFiles()
	require.Error(t, err)
}

func TestDescribeComponentForEdit_Error(t *testing.T) {
	resetEditFlags(t)

	// An empty directory with no atmos.yaml and no stacks must fail to
	// initialize the CLI config rather than panic.
	dir := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
	require.NoError(t, os.Chdir(dir))
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	t.Setenv("ATMOS_BASE_PATH", ".")

	flagComponent = "does-not-exist"
	flagStack = "does-not-exist"

	_, _, err = describeComponentForEdit()
	require.Error(t, err)
}

func TestDescribeComponentForEdit_Success(t *testing.T) {
	resetEditFlags(t)
	chdirToValidAtmosProject(t)

	flagComponent = "mycomponent"
	flagStack = "nonprod"

	atmosConfig, result, err := describeComponentForEdit()
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.MergeContext)
	assert.True(t, atmosConfig.TrackProvenance)

	componentType, _ := result.ComponentSection[cfg.ComponentTypeSectionName].(string)
	assert.Equal(t, "terraform", componentType)
}

func TestRunStackGet_ProvenanceBranch(t *testing.T) {
	resetEditFlags(t)
	chdirToValidAtmosProject(t)
	initStackConfigTestWriter(t)

	flagComponent = "mycomponent"
	flagStack = "nonprod"
	flagFile = ""

	require.NoError(t, runStackGet([]string{"vars.foo"}))
}

func TestRunStackSet_ProvenanceBranch(t *testing.T) {
	resetEditFlags(t)
	chdirToValidAtmosProject(t)

	flagComponent = "mycomponent"
	flagStack = "nonprod"
	flagFile = ""
	flagType = atmosyaml.TypeString

	require.NoError(t, runStackSet([]string{"vars.foo", "updated-value"}))

	// Operates on chdirToValidAtmosProject's disposable temp copy of the
	// fixture, so no restoration of the checked-in file is needed.
	got, err := atmosyaml.GetFile("stacks/deploy/nonprod.yaml", "components.terraform.mycomponent.vars.foo")
	require.NoError(t, err)
	assert.Equal(t, "updated-value", got)
}
