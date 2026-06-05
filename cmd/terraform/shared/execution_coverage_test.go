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
	"github.com/cloudposse/atmos/pkg/auth"
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

	parent := &cobra.Command{Use: "terraform"}
	parent.PersistentFlags().String(cfg.IdentityFlagName, "", "")
	child := &cobra.Command{Use: "apply"}
	parent.AddCommand(child)
	RegisterCompletions(child)
	assert.NotNil(t, child.ValidArgsFunction)

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

func TestResolveAndPromptForArgsResolvesComponentPath(t *testing.T) {
	fixtureDir := createSharedExecutionFixture(t)

	previousInitCliConfig := initCliConfig
	previousResolver := resolveComponentFromPath
	initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	resolveComponentFromPath = func(_ *schema.AtmosConfiguration, path string, stack string, expectedComponentType string) (string, error) {
		assert.Equal(t, "components/terraform/vpc", path)
		assert.Equal(t, "dev", stack)
		assert.Equal(t, cfg.TerraformComponentType, expectedComponentType)
		return "vpc", nil
	}
	t.Cleanup(func() {
		initCliConfig = previousInitCliConfig
		resolveComponentFromPath = previousResolver
	})

	info := &schema.ConfigAndStacksInfo{
		AtmosConfigDirsFromArg: []string{fixtureDir},
		ComponentFromArg:       "components/terraform/vpc",
		Stack:                  "dev",
		NeedsPathResolution:    true,
	}

	require.NoError(t, ResolveAndPromptForArgs(info, &cobra.Command{Use: "plan"}))
	assert.Equal(t, "vpc", info.ComponentFromArg)
	assert.False(t, info.NeedsPathResolution)
}

func TestResolveAndPromptForArgsReturnsPathResolutionError(t *testing.T) {
	fixtureDir := createSharedExecutionFixture(t)

	previousInitCliConfig := initCliConfig
	previousResolver := resolveComponentFromPath
	initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	resolveComponentFromPath = func(_ *schema.AtmosConfiguration, _ string, _ string, _ string) (string, error) {
		return "", errUtils.ErrComponentNotInStack
	}
	t.Cleanup(func() {
		initCliConfig = previousInitCliConfig
		resolveComponentFromPath = previousResolver
	})

	err := ResolveAndPromptForArgs(&schema.ConfigAndStacksInfo{
		AtmosConfigDirsFromArg: []string{fixtureDir},
		ComponentFromArg:       "components/terraform/missing",
		Stack:                  "dev",
		NeedsPathResolution:    true,
	}, &cobra.Command{Use: "plan"})

	assert.ErrorIs(t, err, errUtils.ErrComponentNotInStack)
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

func TestHandleInteractiveComponentStackSelectionValidatesStackOnlyInput(t *testing.T) {
	restoreStackListingStubs(t, map[string]any{
		"dev":  map[string]any{},
		"prod": map[string]any{},
	}, nil)

	err := HandleInteractiveComponentStackSelection(&schema.ConfigAndStacksInfo{Stack: "dev"}, &cobra.Command{Use: "plan"})
	require.NoError(t, err)

	err = HandleInteractiveComponentStackSelection(&schema.ConfigAndStacksInfo{Stack: "missing"}, &cobra.Command{Use: "plan"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidStack)
}

func TestHandleInteractiveComponentStackSelectionReturnsStackListError(t *testing.T) {
	expectedErr := errors.New("describe stacks")
	restoreStackListingStubs(t, nil, expectedErr)

	err := HandleInteractiveComponentStackSelection(&schema.ConfigAndStacksInfo{Stack: "dev"}, &cobra.Command{Use: "plan"})

	assert.ErrorIs(t, err, expectedErr)
}

func restoreStackListingStubs(t *testing.T, stacks map[string]any, stubErr error) {
	t.Helper()

	previousInitCliConfig := initCliConfig
	previousExecuteDescribeStacks := executeDescribeStacks
	initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	executeDescribeStacks = func(
		_ *schema.AtmosConfiguration,
		_ string,
		_ []string,
		_ []string,
		_ []string,
		_ bool,
		_ bool,
		_ bool,
		_ bool,
		_ []string,
		_ auth.AuthManager,
	) (map[string]any, error) {
		return stacks, stubErr
	}

	t.Cleanup(func() {
		initCliConfig = previousInitCliConfig
		executeDescribeStacks = previousExecuteDescribeStacks
	})
}

func createSharedExecutionFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`
base_path: "./"
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths:
    - "**/*.yaml"
  name_pattern: "{stage}"
`), 0o644))
	return dir
}
