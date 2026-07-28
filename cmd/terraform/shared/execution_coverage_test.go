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
	v.Set("query", ".vars.enabled")
	v.Set("components", []string{"vpc", "eks"})
	v.Set("all", true)
	v.Set("affected", true)
	v.Set("upload-status", true)

	opts, err := ParseRunOptions(v)
	require.NoError(t, err)
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
}

func TestParseRunOptions_InvalidLabelsFlagReturnsError(t *testing.T) {
	v := viper.New()
	// tags.ParseLabelsFlag expects comma-separated key=value pairs; a bare
	// key with no "=" is invalid.
	v.Set("labels", "not-a-valid-label")

	opts, err := ParseRunOptions(v)
	require.Error(t, err)
	assert.Nil(t, opts)
}

func TestParseRunOptions_PropagatesValidationError(t *testing.T) {
	v := viper.New()
	v.Set("failure-mode", "bogus-mode")

	opts, err := ParseRunOptions(v)
	require.Error(t, err)
	assert.Nil(t, opts)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
}

func TestValidateRunOptions_NilOptsIsNoop(t *testing.T) {
	assert.NoError(t, ValidateRunOptions(nil))
}

func TestValidateRunOptions_FailureModeAndLogOrder(t *testing.T) {
	tests := []struct {
		name        string
		opts        *RunOptions
		wantErr     bool
		wantFailure string
		wantLog     string
	}{
		{
			name:        "normalizes valid failure mode casing/whitespace",
			opts:        &RunOptions{FailureMode: "  Fail-Fast  "},
			wantFailure: TerraformFailureModeFailFast,
		},
		{
			name:        "accepts keep-going",
			opts:        &RunOptions{FailureMode: TerraformFailureModeKeepGoing},
			wantFailure: TerraformFailureModeKeepGoing,
		},
		{
			name:    "rejects invalid failure mode",
			opts:    &RunOptions{FailureMode: "bogus"},
			wantErr: true,
		},
		{
			name:    "rejects invalid log order",
			opts:    &RunOptions{LogOrder: "bogus"},
			wantErr: true,
		},
		{
			name:    "normalizes valid log order casing/whitespace",
			opts:    &RunOptions{LogOrder: "  Grouped  "},
			wantLog: TerraformLogOrderGrouped,
		},
		{
			name:    "accepts stream log order",
			opts:    &RunOptions{LogOrder: TerraformLogOrderStream},
			wantLog: TerraformLogOrderStream,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRunOptions(tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
				return
			}
			require.NoError(t, err)
			if tt.wantFailure != "" {
				assert.Equal(t, tt.wantFailure, tt.opts.FailureMode)
			}
			if tt.wantLog != "" {
				assert.Equal(t, tt.wantLog, tt.opts.LogOrder)
			}
		})
	}
}

func TestTerraformPlanHideContains_NoMatchReturnsFalse(t *testing.T) {
	assert.False(t, TerraformPlanHideContains([]string{"foo", "bar"}, "no-changes"))
	assert.False(t, TerraformPlanHideContains(nil, "no-changes"))
	assert.True(t, TerraformPlanHideContains([]string{"foo", " No-Changes "}, "no-changes"))
}

func TestApplyRunOptions_AppendArgsAppended(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{AdditionalArgsAndFlags: []string{"-existing"}}
	ApplyRunOptions(info, &RunOptions{AppendArgs: []string{"-json", "-no-color"}})
	assert.Equal(t, []string{"-existing", "-json", "-no-color"}, info.AdditionalArgsAndFlags)
}

func TestApplyRunOptions_NoAppendArgsLeavesUnchanged(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{AdditionalArgsAndFlags: []string{"-existing"}}
	ApplyRunOptions(info, &RunOptions{})
	assert.Equal(t, []string{"-existing"}, info.AdditionalArgsAndFlags)
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

func TestAddIdentityCompletion_LogsAlreadyRegisteredError(t *testing.T) {
	// Registering the identity completion func twice on the same command/flag
	// makes cobra's RegisterFlagCompletionFunc return an "already registered"
	// error; addIdentityCompletion must not panic or propagate it, only log it.
	cmd := &cobra.Command{Use: "plan"}
	cmd.Flags().String(cfg.IdentityFlagName, "", "")

	addIdentityCompletion(cmd)
	require.NotPanics(t, func() { addIdentityCompletion(cmd) })
}

func TestResolveComponentPath_InitCliConfigError(t *testing.T) {
	previousInit := initCliConfig
	expectedErr := errors.New("boom: could not init cli config")
	initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, expectedErr
	}
	t.Cleanup(func() { initCliConfig = previousInit })

	err := ResolveComponentPath(&schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev"}, cfg.TerraformComponentType)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPathResolutionFailed)
	assert.ErrorIs(t, err, expectedErr)
}

func TestIdentityFlagCompletion_InitCliConfigErrorReturnsNoFileComp(t *testing.T) {
	// Point at a config path with a syntactically invalid atmos.yaml so
	// InitCliConfig itself fails to parse, rather than merely finding no
	// identities (which is a different, already-covered branch).
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte("not: [valid: yaml"), 0o644))
	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

	results, directive := identityFlagCompletion(&cobra.Command{}, nil, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Nil(t, results)
}

func TestResolveAndPromptForArgsResolvesComponentPath(t *testing.T) {
	fixtureDir := createSharedExecutionFixture(t)

	previousInitCliConfig := initCliConfig
	previousResolver := resolveComponentFromPath
	expectedComponentPath := filepath.Join("components", "terraform", "vpc")
	initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	resolveComponentFromPath = func(_ *schema.AtmosConfiguration, path string, stack string, expectedComponentType string) (string, error) {
		assert.Equal(t, expectedComponentPath, path)
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
		ComponentFromArg:       expectedComponentPath,
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

func TestHandleInteractiveIdentitySelectionInitCliConfigError(t *testing.T) {
	previousInitCliConfig := initCliConfig
	expectedErr := errors.New("boom: could not init cli config")
	initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, expectedErr
	}
	t.Cleanup(func() { initCliConfig = previousInitCliConfig })

	err := HandleInteractiveIdentitySelection(&schema.ConfigAndStacksInfo{})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInitializeCLIConfig)
	assert.ErrorIs(t, err, expectedErr)
}

func TestHandleInteractiveIdentitySelectionAuthManagerCreationFails(t *testing.T) {
	// With identities configured, HandleInteractiveIdentitySelection passes
	// cfg.IdentityFlagSelectValue as both the identity name and the select
	// sentinel to auth.CreateAndAuthenticateManager, which forces interactive
	// identity selection. In this headless test process there is no TTY, so
	// manager creation itself fails before an AuthManager is ever returned —
	// exercising the "failed to initialize auth manager" wrap.
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(`
base_path: ""
stacks:
  base_path: "stacks"
auth:
  identities:
    dev:
      kind: "ambient"
`), 0o644))

	err := HandleInteractiveIdentitySelection(&schema.ConfigAndStacksInfo{
		AtmosConfigDirsFromArg: []string{tmpDir},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitializeAuthManager)
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

func TestPromptMissingComponent_AlreadySetShortCircuits(t *testing.T) {
	// When ComponentFromArg is already populated, promptMissingComponent must
	// return immediately without consulting the interactive picker at all.
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"}
	require.NoError(t, promptMissingComponent(info, &cobra.Command{Use: "plan"}))
	assert.Equal(t, "vpc", info.ComponentFromArg, "must not be overwritten")
}

func TestHandleInteractiveComponentStackSelectionPropagatesComponentPromptError(t *testing.T) {
	// Force interactive mode and make the component picker's underlying stack
	// enumeration fail, so PromptForComponent surfaces ErrLoadSelectionOptions
	// (not UserAborted/InteractiveModeNotAvailable), which HandlePromptError
	// must pass through, and HandleInteractiveComponentStackSelection must
	// propagate from promptMissingComponent without also calling promptMissingStack.
	previousInteractive := isInteractiveFn
	previousInit := initCliConfig
	previousDescribe := executeDescribeStacks
	isInteractiveFn = func() bool { return true }
	initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	describeErr := errors.New("boom: describe stacks")
	executeDescribeStacks = func(
		_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager,
	) (map[string]any, error) {
		return nil, describeErr
	}
	t.Cleanup(func() {
		isInteractiveFn = previousInteractive
		initCliConfig = previousInit
		executeDescribeStacks = previousDescribe
	})

	info := &schema.ConfigAndStacksInfo{}
	err := HandleInteractiveComponentStackSelection(info, &cobra.Command{Use: "plan"})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrLoadSelectionOptions)
	assert.ErrorIs(t, err, describeErr)
	assert.Empty(t, info.Stack, "must fail before reaching the stack prompt")
}

func TestHandleInteractiveComponentStackSelectionPropagatesStackPromptError(t *testing.T) {
	// With the component already resolved, promptMissingComponent short-circuits
	// and the stack prompt's own load failure must propagate from
	// HandleInteractiveComponentStackSelection.
	previousInteractive := isInteractiveFn
	previousInit := initCliConfig
	previousDescribe := executeDescribeStacks
	isInteractiveFn = func() bool { return true }
	initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	describeErr := errors.New("boom: describe stacks for stack picker")
	executeDescribeStacks = func(
		_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager,
	) (map[string]any, error) {
		return nil, describeErr
	}
	t.Cleanup(func() {
		isInteractiveFn = previousInteractive
		initCliConfig = previousInit
		executeDescribeStacks = previousDescribe
	})

	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"}
	err := HandleInteractiveComponentStackSelection(info, &cobra.Command{Use: "plan"})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrLoadSelectionOptions)
	assert.ErrorIs(t, err, describeErr)
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
