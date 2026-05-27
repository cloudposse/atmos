package shared

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecutionFlagPredicates(t *testing.T) {
	tests := []struct {
		name           string
		info           schema.ConfigAndStacksInfo
		multi          bool
		hasMulti       bool
		hasNonAffected bool
		hasSingle      bool
	}{
		{name: "empty"},
		{name: "all", info: schema.ConfigAndStacksInfo{All: true}, multi: true, hasMulti: true, hasNonAffected: true},
		{name: "affected", info: schema.ConfigAndStacksInfo{Affected: true}, hasMulti: true},
		{name: "components", info: schema.ConfigAndStacksInfo{Components: []string{"vpc"}}, multi: true, hasMulti: true, hasNonAffected: true},
		{name: "query", info: schema.ConfigAndStacksInfo{Query: ".vars.enabled"}, multi: true, hasMulti: true, hasNonAffected: true},
		{name: "stack without component", info: schema.ConfigAndStacksInfo{Stack: "dev"}, multi: true},
		{name: "stack with component", info: schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"}},
		{name: "planfile", info: schema.ConfigAndStacksInfo{PlanFile: "plan.out"}, hasSingle: true},
		{name: "use terraform plan", info: schema.ConfigAndStacksInfo{UseTerraformPlan: true}, hasSingle: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.multi, IsMultiComponentExecution(&tt.info))
			assert.Equal(t, tt.hasMulti, HasMultiComponentFlags(&tt.info))
			assert.Equal(t, tt.hasNonAffected, HasNonAffectedMultiFlags(&tt.info))
			assert.Equal(t, tt.hasSingle, HasSingleComponentFlags(&tt.info))
		})
	}
}

func TestCheckTerraformFlagsInvalidCombinations(t *testing.T) {
	tests := []struct {
		name string
		info schema.ConfigAndStacksInfo
		want error
	}{
		{
			name: "component with multi component flag",
			info: schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", All: true},
			want: errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags,
		},
		{
			name: "affected with all",
			info: schema.ConfigAndStacksInfo{Affected: true, All: true},
			want: errUtils.ErrInvalidTerraformFlagsWithAffectedFlag,
		},
		{
			name: "planfile with query",
			info: schema.ConfigAndStacksInfo{PlanFile: "plan.out", Query: ".vars.enabled"},
			want: errUtils.ErrInvalidTerraformSingleComponentAndMultiComponentFlags,
		},
		{
			name: "valid affected only",
			info: schema.ConfigAndStacksInfo{Affected: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckTerraformFlags(&tt.info)
			if tt.want == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestParseAndApplyRunOptions(t *testing.T) {
	v := viper.New()
	v.Set("process-templates", true)
	v.Set("process-functions", true)
	v.Set("skip", []string{"store.get"})
	v.Set("dry-run", true)
	v.Set("skip-init", true)
	v.Set("init-pass-vars", true)
	v.Set("auto-generate-backend-file", "true")
	v.Set("init-run-reconfigure", "false")
	v.Set("planfile", "tfplan")
	v.Set("skip-planfile", true)
	v.Set("deploy-run-init", true)
	v.Set("verify-plan", true)
	v.Set("query", ".vars.enabled")
	v.Set("components", []string{"vpc", "eks"})
	v.Set("all", true)
	v.Set("affected", true)
	v.Set("upload-status", true)

	opts := ParseRunOptions(v)
	assert.True(t, opts.ProcessTemplates)
	assert.True(t, opts.ProcessFunctions)
	assert.Equal(t, []string{"store.get"}, opts.Skip)
	assert.True(t, opts.DryRun)
	assert.True(t, opts.SkipInit)
	assert.True(t, opts.InitPassVars)
	assert.Equal(t, "true", opts.AutoGenerateBackendFile)
	assert.Equal(t, "false", opts.InitRunReconfigure)
	assert.Equal(t, "tfplan", opts.PlanFile)
	assert.True(t, opts.PlanSkipPlanfile)
	assert.True(t, opts.DeployRunInit)
	assert.True(t, opts.VerifyPlan)
	assert.Equal(t, ".vars.enabled", opts.Query)
	assert.Equal(t, []string{"vpc", "eks"}, opts.Components)
	assert.True(t, opts.All)
	assert.True(t, opts.Affected)
	assert.True(t, opts.UploadStatus)

	var info schema.ConfigAndStacksInfo
	ApplyRunOptions(&info, opts)

	assert.True(t, info.ProcessTemplates)
	assert.True(t, info.ProcessFunctions)
	assert.Equal(t, []string{"store.get"}, info.Skip)
	assert.Equal(t, []string{"vpc", "eks"}, info.Components)
	assert.True(t, info.DryRun)
	assert.True(t, info.SkipInit)
	assert.True(t, info.UploadStatus)
	assert.True(t, info.All)
	assert.True(t, info.Affected)
	assert.Equal(t, ".vars.enabled", info.Query)
	assert.Equal(t, "true", info.AutoGenerateBackendFile)
	assert.Equal(t, "false", info.InitRunReconfigure)
	assert.Equal(t, "true", info.InitPassVars)
	assert.Equal(t, "tfplan", info.PlanFile)
	assert.True(t, info.UseTerraformPlan)
	assert.Equal(t, "true", info.PlanSkipPlanfile)
	assert.Equal(t, "true", info.DeployRunInit)
	assert.True(t, info.VerifyPlan)
}

func TestBackendAndIdentityFlagRegistration(t *testing.T) {
	registry := BackendExecutionFlags()
	require.True(t, registry.Has("auto-generate-backend-file"))
	require.True(t, registry.Has("init-run-reconfigure"))

	parser := flags.NewStandardParser(WithBackendExecutionFlags())
	cmd := &cobra.Command{Use: "terraform"}
	parser.RegisterFlags(cmd)
	assert.NotNil(t, cmd.Flags().Lookup("auto-generate-backend-file"))
	assert.NotNil(t, cmd.Flags().Lookup("init-run-reconfigure"))

	identityRegistry := flags.NewFlagRegistry()
	RegisterIdentityFlags(identityRegistry)
	identity := identityRegistry.Get(cfg.IdentityFlagName)
	require.NotNil(t, identity)
	assert.Equal(t, cfg.IdentityFlagSelectValue, identity.GetNoOptDefVal())
}

func TestCompletionsAndPathResolutionErrors(t *testing.T) {
	cmd := &cobra.Command{Use: "plan"}
	RegisterIdentityFlags(flags.NewFlagRegistry())
	cmd.Flags().String(cfg.IdentityFlagName, "", "")
	RegisterCompletions(cmd)
	assert.NotNil(t, cmd.ValidArgsFunction)

	cmdWithoutIdentity := &cobra.Command{Use: "plan"}
	RegisterCompletions(cmdWithoutIdentity)
	assert.NotNil(t, cmdWithoutIdentity.ValidArgsFunction)

	for _, err := range []error{
		errUtils.ErrAmbiguousComponentPath,
		errUtils.ErrComponentNotInStack,
		errUtils.ErrStackNotFound,
		errUtils.ErrUserAborted,
	} {
		assert.Same(t, err, HandlePathResolutionError(err))
	}

	wrapped := HandlePathResolutionError(errors.New("boom"))
	require.Error(t, wrapped)
	assert.ErrorIs(t, wrapped, errUtils.ErrPathResolutionFailed)
}

func TestIdentityFlagCompletionReturnsConfiguredIdentities(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(`
base_path: ""
stacks:
  base_path: "stacks"
auth:
  identities:
    prod-admin:
      type: "aws"
      role_arn: "arn:aws:iam::123456789012:role/admin"
    dev-user:
      type: "aws"
      role_arn: "arn:aws:iam::123456789012:role/developer"
`), 0o644))
	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

	results, directive := identityFlagCompletion(&cobra.Command{}, nil, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Equal(t, []string{"dev-user", "prod-admin"}, results)
}

func TestIdentityFlagCompletionConfigError(t *testing.T) {
	t.Setenv("ATMOS_CLI_CONFIG_PATH", filepath.Join(t.TempDir(), "missing"))

	results, directive := identityFlagCompletion(&cobra.Command{}, nil, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Nil(t, results)
}

func TestComponentCompletionShortCircuitsAfterFirstArg(t *testing.T) {
	results, directive := ComponentsArgCompletion(&cobra.Command{}, []string{"vpc"}, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Nil(t, results)
}

func TestPromptHelpersReturnEmptyWhenNonInteractive(t *testing.T) {
	component, err := PromptForComponent(&cobra.Command{}, "")
	require.NoError(t, err)
	assert.Empty(t, component)

	stack, err := PromptForStack(&cobra.Command{}, "vpc")
	require.NoError(t, err)
	assert.Empty(t, stack)

	info := &schema.ConfigAndStacksInfo{}
	require.NoError(t, promptMissingComponent(info, &cobra.Command{}))
	assert.Empty(t, info.ComponentFromArg)

	require.NoError(t, promptMissingStack(info, &cobra.Command{}))
	assert.Empty(t, info.Stack)
}

func TestHandleInteractiveIdentitySelectionNoConfiguredIdentities(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(`
base_path: ""
stacks:
  base_path: "stacks"
`), 0o644))

	err := HandleInteractiveIdentitySelection(&schema.ConfigAndStacksInfo{
		AtmosConfigDirsFromArg: []string{tmpDir},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNoIdentitiesAvailable)
}

func TestResolveAndPromptForArgsShortCircuitsForMultiComponent(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{All: true}
	require.NoError(t, ResolveAndPromptForArgs(info, &cobra.Command{Use: "plan"}))

	info = &schema.ConfigAndStacksInfo{NeedHelp: true}
	require.NoError(t, HandleInteractiveComponentStackSelection(info, &cobra.Command{Use: "plan"}))

	info = &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"}
	require.NoError(t, HandleInteractiveComponentStackSelection(info, &cobra.Command{Use: "plan"}))
}
