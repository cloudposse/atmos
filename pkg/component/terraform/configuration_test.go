package terraform

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	awsidentity "github.com/cloudposse/atmos/pkg/auth/identities/aws"
	componentpkg "github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:generate go run go.uber.org/mock/mockgen -destination=mock_configuration_test.go -package=terraform github.com/cloudposse/atmos/pkg/schema ComponentConfigurationRuntime

func TestPrepareConfiguration(t *testing.T) {
	runtime := NewMockComponentConfigurationRuntime(gomock.NewController(t))
	config := &schema.AtmosConfiguration{ComponentConfigurations: runtime}
	binary, err := os.Executable()
	require.NoError(t, err)
	info := &schema.ConfigAndStacksInfo{Command: binary, SubCommand: "apply", Stack: "live/dev", ComponentFromArg: "app", ComponentSection: map[string]any{}, ComponentSettingsSection: map[string]any{"configuration": true}}
	expected := &schema.PreparedComponentConfiguration{Engine: "terraform", Source: filepath.Join(t.TempDir(), "module"), Inputs: map[string]any{"fresh": "output"}, BackendType: "local", Backend: map[string]any{"path": "state.tfstate"}, Workspace: "default"}
	runtime.EXPECT().Prepare(gomock.Any(), info).Return(expected, nil)
	result, err := prepareConfiguration(t.Context(), config, info)
	require.NoError(t, err)
	require.Same(t, expected, result)
	require.True(t, filepath.IsAbs(info.Command))
	require.Equal(t, expected.Inputs, info.ComponentVarsSection)
	// Local module edits are detected by the source provisioner, so no TTL forces a refresh.
	require.Equal(t, map[string]any{"uri": expected.Source}, info.ComponentSection["source"])
	require.Equal(t, expected.Backend, info.ComponentBackendSection)
	require.Equal(t, "default", info.TerraformWorkspace)
	require.True(t, config.Components.Terraform.AutoGenerateBackendFile)
}

func TestPrepareConfigurationNeverForcesSourceRefresh(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
	}{
		{name: "relative path", source: "./modules/unit"},
		{name: "absolute path", source: filepath.Join(t.TempDir(), "module")},
		{name: "git", source: "git::https://example.invalid/modules.git//unit?ref=v1"},
		{name: "archive", source: "https://example.invalid/unit.zip"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewMockComponentConfigurationRuntime(gomock.NewController(t))
			config := &schema.AtmosConfiguration{ComponentConfigurations: runtime}
			binary, err := os.Executable()
			require.NoError(t, err)
			info := &schema.ConfigAndStacksInfo{Command: binary, SubCommand: "apply", ComponentSection: map[string]any{}, ComponentSettingsSection: map[string]any{"configuration": true}}
			runtime.EXPECT().Prepare(gomock.Any(), info).Return(&schema.PreparedComponentConfiguration{Engine: "terraform", Source: test.source}, nil)

			_, err = prepareConfiguration(t.Context(), config, info)

			require.NoError(t, err)
			source, ok := info.ComponentSection["source"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, test.source, source["uri"])
			require.NotContains(t, source, "ttl", "a TTL of zero would re-copy the module and discard saved plans on every run")
		})
	}
}

func TestPrepareConfigurationGuards(t *testing.T) {
	for _, test := range []struct {
		name    string
		info    schema.ConfigAndStacksInfo
		runtime bool
		want    error
	}{
		{name: "ordinary"},
		{name: "explicitly disabled", info: schema.ConfigAndStacksInfo{ComponentSettingsSection: map[string]any{"configuration": false}}},
		{name: "null", info: schema.ConfigAndStacksInfo{ComponentSettingsSection: map[string]any{"configuration": nil}}},
		{name: "unavailable", info: schema.ConfigAndStacksInfo{ComponentSettingsSection: map[string]any{"configuration": true}}, want: errUtils.ErrPluginUnsupportedExtension},
		{name: "abstract", runtime: true, info: schema.ConfigAndStacksInfo{ComponentSettingsSection: map[string]any{"configuration": true}, SubCommand: "apply", ComponentIsAbstract: true}, want: errUtils.ErrAbstractComponentCantBeProvisioned},
		{name: "dry run", runtime: true, info: schema.ConfigAndStacksInfo{ComponentSettingsSection: map[string]any{"configuration": true}, DryRun: true}},
		{name: "missing binary", runtime: true, info: schema.ConfigAndStacksInfo{ComponentSettingsSection: map[string]any{"configuration": true}, Command: "atmos-no-such-engine-234856"}, want: errUtils.ErrPluginInvalidConfig},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			if test.runtime {
				config.ComponentConfigurations = NewMockComponentConfigurationRuntime(gomock.NewController(t))
			}
			result, err := prepareConfiguration(t.Context(), config, &test.info)
			require.ErrorIs(t, err, test.want)
			require.Nil(t, result)
		})
	}
}

func TestConfigurationArtifactNames(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{Stack: "live/dev", ComponentFromArg: "region/app", ComponentSettingsSection: map[string]any{"configuration": true}}
	names := []string{ConstructTerraformComponentVarfileName(info), ConstructTerraformComponentPlanfileName(info), ConstructTerraformComponentTestVarfileName(info)}
	for _, name := range names {
		require.Equal(t, filepath.Base(name), name)
		require.Contains(t, name, "atmos-")
	}
	first := names[0]
	info.Stack = "live/prod"
	require.NotEqual(t, first, ConstructTerraformComponentVarfileName(info))
	info.Stack = "live/dev"
	info.ComponentFromArg = "other/app"
	require.NotEqual(t, first, ConstructTerraformComponentVarfileName(info))
	// Without ComponentFromArg the component identity still keeps names distinct.
	info.ComponentFromArg, info.Component = "", "region/app"
	app := ConstructTerraformComponentPlanfileName(info)
	info.Component = "region/other"
	require.NotEqual(t, app, ConstructTerraformComponentPlanfileName(info))
	info.ComponentFromArg, info.Component = "region/app", "ignored"
	require.Equal(t, app, ConstructTerraformComponentPlanfileName(info), "ComponentFromArg keeps precedence")
	info.ComponentSettingsSection = nil
	_, owned := configurationArtifactName(info, ".plan")
	require.False(t, owned)
}

// A plan written by one process is applied by another, so the artifact name must never depend on
// process state. The digest is pinned so a change to the naming scheme is deliberate.
func TestConfigurationIdentityIsStableAndSeparatesFields(t *testing.T) {
	const pinned = "060d8d2f24f80685c67572911d8f2cd5"
	first := configurationIdentity("live/dev", "region/app")
	require.Equal(t, pinned, first)
	require.Equal(t, first, configurationIdentity("live/dev", "region/app"))
	require.NotEqual(t, first, configurationIdentity("live/dev", "region/app2"))
	// The separator keeps adjacent fields from sliding into each other.
	require.NotEqual(t, configurationIdentity("ab", "c"), configurationIdentity("a", "bc"))
}

func TestCacheLocalStatePath(t *testing.T) {
	cache := filepath.Join("live", "dev", "unit", ".terragrunt-cache", "abc", "def", "terraform.tfstate")
	for _, test := range []struct {
		name     string
		prepared schema.PreparedComponentConfiguration
		want     bool
	}{
		{"default local state inside the cache", schema.PreparedComponentConfiguration{BackendType: "local", Backend: map[string]any{"path": cache}}, true},
		{"stable local path", schema.PreparedComponentConfiguration{BackendType: "local", Backend: map[string]any{"path": filepath.Join("live", "dev", "unit", "state", "terraform.tfstate")}}, false},
		{"similar name is not the cache", schema.PreparedComponentConfiguration{BackendType: "local", Backend: map[string]any{"path": filepath.Join("a", "x.terragrunt-cache", "terraform.tfstate")}}, false},
		{"remote backend", schema.PreparedComponentConfiguration{BackendType: "s3", Backend: map[string]any{"path": cache}}, false},
		{"no backend settings", schema.PreparedComponentConfiguration{BackendType: "local"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, found := cacheLocalStatePath(&test.prepared)
			require.Equal(t, test.want, found)
			if found {
				require.Equal(t, cache, path)
			}
		})
	}
}

func TestCheckConfigurationWorkspace(t *testing.T) {
	for _, test := range []struct {
		name      string
		base      []string
		component []string
		want      error
	}{
		{name: "unset", base: []string{}},
		{name: "empty", base: []string{"TF_WORKSPACE="}},
		{name: "matching base", base: []string{"TF_WORKSPACE=stage"}},
		{name: "matching component", base: []string{}, component: []string{"TF_WORKSPACE=stage"}},
		{name: "conflicting base", base: []string{"TF_WORKSPACE=prod"}, want: errUtils.ErrPluginInvalidConfig},
		{name: "conflicting component", base: []string{"TF_WORKSPACE=stage"}, component: []string{"TF_WORKSPACE=prod"}, want: errUtils.ErrPluginInvalidConfig},
		{name: "component overrides conflicting base", base: []string{"TF_WORKSPACE=prod"}, component: []string{"TF_WORKSPACE=stage"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			info := &schema.ConfigAndStacksInfo{TerraformWorkspace: "stage", SanitizedEnv: test.base, ComponentEnvList: test.component}
			require.ErrorIs(t, checkConfigurationWorkspace(info), test.want)
		})
	}
}

// Warning about cache-local state must never change where the state lives or fail the command.
func TestPrepareConfigurationKeepsCacheLocalStateLocation(t *testing.T) {
	runtime := NewMockComponentConfigurationRuntime(gomock.NewController(t))
	config := &schema.AtmosConfiguration{ComponentConfigurations: runtime}
	binary, err := os.Executable()
	require.NoError(t, err)
	info := &schema.ConfigAndStacksInfo{Command: binary, SubCommand: "plan", Stack: "live/dev", ComponentFromArg: "app", ComponentSection: map[string]any{}, ComponentSettingsSection: map[string]any{"configuration": true}}
	cache := filepath.Join(t.TempDir(), ".terragrunt-cache", "a", "b", "terraform.tfstate")
	runtime.EXPECT().Prepare(gomock.Any(), info).Return(&schema.PreparedComponentConfiguration{Engine: "terraform", Source: "./module", BackendType: "local", Backend: map[string]any{"path": cache}}, nil)

	_, err = prepareConfiguration(t.Context(), config, info)

	require.NoError(t, err)
	require.Equal(t, cache, info.ComponentBackendSection["path"])
}

// fakeAssume replaces STS and records what it was asked to assume and with which environment.
func fakeAssume(t *testing.T, err error) *struct {
	role    awsidentity.InlineRole
	environ []string
	calls   int
} {
	t.Helper()
	recorded := &struct {
		role    awsidentity.InlineRole
		environ []string
		calls   int
	}{}
	previous := assumeInlineRole
	assumeInlineRole = func(_ context.Context, role awsidentity.InlineRole, environ []string) (map[string]string, error) {
		recorded.role, recorded.environ, recorded.calls = role, environ, recorded.calls+1
		if err != nil {
			return nil, err
		}
		return map[string]string{"AWS_ACCESS_KEY_ID": "ASIASESSION", "AWS_SESSION_TOKEN": "session-token", "AWS_PROFILE": ""}, nil
	}
	t.Cleanup(func() { assumeInlineRole = previous })
	return recorded
}

type stagedIAMRuntime struct {
	iam     *schema.ComponentIAM
	prepare func(*schema.ConfigAndStacksInfo) *schema.PreparedComponentConfiguration
	probes  int
}

func (r *stagedIAMRuntime) Discover(context.Context) (map[string]map[string]any, error) {
	return nil, nil
}

func (r *stagedIAMRuntime) ProbeIAM(_ context.Context, _ *schema.ConfigAndStacksInfo) (*schema.ComponentIAM, error) {
	r.probes++
	return r.iam, nil
}

func (r *stagedIAMRuntime) Prepare(_ context.Context, info *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, error) {
	return r.prepare(info), nil
}

func TestPrepareConfigurationStagesIAMForEvaluation(t *testing.T) {
	recorded := fakeAssume(t, nil)
	binary, err := os.Executable()
	require.NoError(t, err)
	iam := &schema.ComponentIAM{Role: "arn:aws:iam::1:role/x"}
	runtime := &stagedIAMRuntime{iam: iam, prepare: func(info *schema.ConfigAndStacksInfo) *schema.PreparedComponentConfiguration {
		require.Contains(t, info.ComponentEnvList, "AWS_ACCESS_KEY_ID=ASIASESSION")
		return &schema.PreparedComponentConfiguration{Engine: "terraform", Source: "./module", IAM: iam}
	}}
	config := &schema.AtmosConfiguration{ComponentConfigurations: runtime}
	info := &schema.ConfigAndStacksInfo{Command: binary, SubCommand: "plan", ComponentSection: map[string]any{}, ComponentSettingsSection: map[string]any{"configuration": true}, SanitizedEnv: []string{"AWS_ACCESS_KEY_ID=BASE", "AWS_SECRET_ACCESS_KEY=base-secret"}}
	_, err = prepareConfiguration(t.Context(), config, info)
	require.NoError(t, err)
	require.Equal(t, 1, runtime.probes)
	require.Equal(t, 1, recorded.calls, "the staged session is reused for execution")
	require.Contains(t, recorded.environ, "AWS_ACCESS_KEY_ID=BASE")
}

func TestPrepareConfigurationRejectsChangedIAMAfterStaging(t *testing.T) {
	fakeAssume(t, nil)
	binary, err := os.Executable()
	require.NoError(t, err)
	runtime := &stagedIAMRuntime{iam: &schema.ComponentIAM{Role: "role-a"}, prepare: func(*schema.ConfigAndStacksInfo) *schema.PreparedComponentConfiguration {
		return &schema.PreparedComponentConfiguration{Engine: "terraform", Source: "./module", IAM: &schema.ComponentIAM{Role: "role-b"}}
	}}
	info := &schema.ConfigAndStacksInfo{Command: binary, SubCommand: "plan", ComponentSection: map[string]any{}, ComponentSettingsSection: map[string]any{"configuration": true}}
	_, err = prepareConfiguration(t.Context(), &schema.AtmosConfiguration{ComponentConfigurations: runtime}, info)
	require.ErrorContains(t, err, "changed between role and unit evaluation")
}

func TestApplyIAMRoleExportsTheSessionToTheComponent(t *testing.T) {
	recorded := fakeAssume(t, nil)
	info := &schema.ConfigAndStacksInfo{Stack: "live/dev", ComponentFromArg: "app", SanitizedEnv: []string{"AWS_PROFILE=atmos", "PATH=/bin"}, ComponentEnvList: []string{"TF_IN_AUTOMATION=true"}}
	iam := &schema.ComponentIAM{Role: "arn:aws:iam::1:role/x", SessionName: "s", DurationSeconds: 1800}
	require.NoError(t, applyIAMRole(t.Context(), info, iam))
	require.Equal(t, 1, recorded.calls)
	require.Equal(t, awsidentity.InlineRole{Role: "arn:aws:iam::1:role/x", SessionName: "s", Duration: 30 * time.Minute}, recorded.role)
	require.Equal(t, []string{"AWS_PROFILE=atmos", "PATH=/bin", "TF_IN_AUTOMATION=true"}, recorded.environ, "the role is assumed on top of the invocation's own credentials")
	require.Equal(t, []string{"TF_IN_AUTOMATION=true", "AWS_ACCESS_KEY_ID=ASIASESSION", "AWS_PROFILE=", "AWS_SESSION_TOKEN=session-token"}, info.ComponentEnvList)
	require.Equal(t, map[string]string{"AWS_ACCESS_KEY_ID": "ASIASESSION", "AWS_SESSION_TOKEN": "session-token", "AWS_PROFILE": ""}, info.IAMSession, "source downloads use the same session")
}

func TestApplyIAMRoleIsSkippedForAnExplicitIdentity(t *testing.T) {
	recorded := fakeAssume(t, nil)
	asked := func(identity string) *string { return &identity }
	iam := &schema.ComponentIAM{Role: "arn:aws:iam::1:role/x"}

	for name, info := range map[string]*schema.ConfigAndStacksInfo{
		"requested with --identity":   {Identity: "deployer", RequestedIdentity: asked("deployer")},
		"requested interactively":     {Identity: "picked", RequestedIdentity: asked(cfg.IdentityFlagSelectValue)},
		"identity without a recorder": {Identity: "deployer"},
	} {
		t.Run(name, func(t *testing.T) {
			info.Stack, info.ComponentFromArg = "live/dev", "app"
			require.NoError(t, applyIAMRole(t.Context(), info, iam))
			require.Empty(t, info.ComponentEnvList, "the selected identity decides who the component runs as")
			require.Nil(t, info.IAMSession, "source downloads keep the selected identity")
		})
	}
	require.Zero(t, recorded.calls)

	// A default identity that authentication chose, and disabled authentication, request nothing: the
	// role is assumed on top of whatever credentials the invocation has.
	for name, info := range map[string]*schema.ConfigAndStacksInfo{
		"default identity chosen by authentication": {Identity: "mock-identity", RequestedIdentity: asked("")},
		"authentication disabled":                   {Identity: cfg.IdentityFlagDisabledValue, RequestedIdentity: asked(cfg.IdentityFlagDisabledValue)},
		"no identity":                               {},
	} {
		t.Run(name, func(t *testing.T) {
			before := recorded.calls
			info.Stack, info.ComponentFromArg = "live/dev", "app"
			require.NoError(t, applyIAMRole(t.Context(), info, iam))
			require.Equal(t, before+1, recorded.calls)
		})
	}
}

func TestApplyIAMRoleWithoutARoleDoesNothing(t *testing.T) {
	recorded := fakeAssume(t, nil)
	info := &schema.ConfigAndStacksInfo{}
	require.NoError(t, applyIAMRole(t.Context(), info, nil))
	require.Zero(t, recorded.calls)
	require.Empty(t, info.ComponentEnvList)
}

func TestApplyIAMRoleReadsTheTokenFileAtAssumptionTime(t *testing.T) {
	recorded := fakeAssume(t, nil)
	file := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(file, []byte("  file-token \n"), 0o600))
	info := &schema.ConfigAndStacksInfo{Stack: "s", ComponentFromArg: "app"}
	require.NoError(t, applyIAMRole(t.Context(), info, &schema.ComponentIAM{Role: "r", WebIdentityTokenFile: file}))
	require.Equal(t, "file-token", recorded.role.WebIdentityToken)

	require.NoError(t, applyIAMRole(t.Context(), info, &schema.ComponentIAM{Role: "r", WebIdentityToken: "literal", WebIdentityTokenLiteral: true}))
	require.Equal(t, "literal", recorded.role.WebIdentityToken)
}

func TestApplyIAMRoleFailuresNameTheRoleAndLeakNothing(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{Stack: "live/dev", ComponentFromArg: "app"}
	for name, test := range map[string]struct {
		iam    *schema.ComponentIAM
		assume error
	}{
		"STS rejects the role":           {&schema.ComponentIAM{Role: "arn:aws:iam::1:role/x"}, errUtils.ErrAuthenticationFailed},
		"missing token file":             {&schema.ComponentIAM{Role: "arn:aws:iam::1:role/x", WebIdentityTokenFile: filepath.Join(t.TempDir(), "absent")}, nil},
		"literal token lost with a plan": {&schema.ComponentIAM{Role: "arn:aws:iam::1:role/x", WebIdentityTokenLiteral: true}, nil},
		"empty token file":               {&schema.ComponentIAM{Role: "arn:aws:iam::1:role/x", WebIdentityTokenFile: emptyTokenFile(t)}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			fakeAssume(t, test.assume)
			err := applyIAMRole(t.Context(), info, test.iam)
			require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
			rendered := errUtils.Format(err, errUtils.FormatterConfig{})
			require.Contains(t, rendered, "arn:aws:iam::1:role/x")
			require.Contains(t, rendered, "--identity")
		})
	}
}

func TestComponentIAMNeverSerializesALiteralToken(t *testing.T) {
	prepared := &schema.PreparedComponentConfiguration{IAM: &schema.ComponentIAM{Role: "r", WebIdentityToken: "super-secret-token", WebIdentityTokenLiteral: true}}
	record, err := json.Marshal(prepared)
	require.NoError(t, err)
	require.NotContains(t, string(record), "super-secret-token")
	require.Contains(t, string(record), "WebIdentityTokenLiteral")
}

func emptyTokenFile(t *testing.T) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(file, []byte(" \n"), 0o600))
	return file
}

func TestRejectRuntimeArtifacts(t *testing.T) {
	require.NoError(t, rejectRuntimeArtifacts([]schema.ComponentConfigurationFile{{Path: "backend.tf", IfExists: "overwrite"}}))
	for _, path := range []string{"examples/terraform.tfstate", "nested/provider.tf"} {
		require.NoError(t, rejectRuntimeArtifacts([]schema.ComponentConfigurationFile{{Path: path}}), path)
	}
	for _, path := range []string{"terraform.tfstate", "terraform.tfstate.backup", ".terraform.tfstate.lock.info", ".terraform/environment", "nested/.terraform/modules/modules.json", "terraform.tfstate.d/stage/terraform.tfstate", ".terraform.lock.hcl", "nested/.dev-app.terraform.lock.hcl", ".atmos/metadata.json"} {
		files := []schema.ComponentConfigurationFile{{Path: "backend.tf", IfExists: "overwrite"}, {Path: path, IfExists: "overwrite"}}
		require.ErrorIs(t, rejectRuntimeArtifacts(files), errUtils.ErrPluginInvalidConfig, path)
	}
}

func TestPreparedConfigurationSensitiveInputsNeverHitDisk(t *testing.T) {
	atmosio.Reset()
	t.Cleanup(atmosio.Reset)
	require.NoError(t, atmosio.Initialize())
	for _, disabled := range []bool{false, true} {
		atmosio.ApplyMaskingConfig(&atmosio.Config{DisableMasking: disabled})
		runtime := NewMockComponentConfigurationRuntime(gomock.NewController(t))
		config := &schema.AtmosConfiguration{ComponentConfigurations: runtime}
		binary, err := os.Executable()
		require.NoError(t, err)
		info := &schema.ConfigAndStacksInfo{Command: binary, SubCommand: "apply", ComponentSection: map[string]any{}, ComponentSettingsSection: map[string]any{"configuration": true}}
		inputs := map[string]any{"credentials": map[string]any{"secret-input-key": ""}, "empty": map[string]any{}, "null": nil, "public": "visible"}
		prepared := &schema.PreparedComponentConfiguration{Inputs: inputs, SensitiveInputKeys: map[string]bool{"credentials": true, "empty": true, "null": true, "absent": true}}
		runtime.EXPECT().Prepare(gomock.Any(), info).Return(prepared, nil)
		_, err = prepareConfiguration(t.Context(), config, info)
		require.NoError(t, err)
		// Partition repeatedly without relying on registered secret strings.
		ComputeTerraformSecretVarKeys(info)
		ComputeTerraformSecretVarKeys(info)
		require.Equal(t, map[string]bool{"credentials": true, "empty": true, "null": true}, info.TerraformSecretVarKeys)
		require.Equal(t, map[string]any{"public": "visible"}, DiskSafeVars(info))
		env, err := SecretVarEnv(info)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{`TF_VAR_credentials={"secret-input-key":""}`, `TF_VAR_empty={}`, `TF_VAR_null=null`}, env)
	}
}

// failingStateRuntime owns the component but cannot identify its backend.
type failingStateRuntime struct {
	schema.ComponentConfigurationRuntime
	err error
}

func (r failingStateRuntime) ResolveState(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
	return nil, r.err
}

func TestGetTerraformStateStopsWhenProviderCannotResolveBackend(t *testing.T) {
	failure := errors.New("provider could not locate state")
	executor := testExecutor(t)
	executor.services.ExecuteDescribeComponent = func(params *componentpkg.ExecuteDescribeComponentParams) (map[string]any, error) {
		assert.Equal(t, "network", params.Component)
		assert.Equal(t, "live", params.Stack)
		return map[string]any{
			"settings": map[string]any{"configuration": true},
			"backend":  map[string]any{"path": "must-not-be-read"},
		}, nil
	}
	config := &schema.AtmosConfiguration{ComponentConfigurations: failingStateRuntime{err: failure}}

	result, err := executor.GetTerraformState(config, "!terraform.state", "live", "network", "vpc_id", true, nil, nil)
	require.ErrorIs(t, err, failure, "the provider's failure is returned as is, not retried against a guessed backend")
	require.Nil(t, result)
}
