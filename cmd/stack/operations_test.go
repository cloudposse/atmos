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
	require.Contains(t, rows, listpkg.PathRow{File: "deploy/prod.yaml", Path: "vars", Type: "object"})
	require.Contains(t, rows, listpkg.PathRow{File: "deploy/prod.yaml", Path: "vars.region", Type: "string"})
	require.Contains(t, rows, listpkg.PathRow{File: "deploy/settings.yaml", Path: "settings.enabled", Type: "bool"})
	require.NotContains(t, rows, listpkg.PathRow{File: "deploy/prod.yaml", Path: cfg.ComponentTypeSectionName, Type: "string"})
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
