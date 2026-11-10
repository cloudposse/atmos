package terraform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	awsidentity "github.com/cloudposse/atmos/pkg/auth/identities/aws"
	cfg "github.com/cloudposse/atmos/pkg/config"
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

func TestApplyIAMRoleExportsTheSessionToTheComponent(t *testing.T) {
	recorded := fakeAssume(t, nil)
	info := &schema.ConfigAndStacksInfo{Stack: "live/dev", ComponentFromArg: "app", SanitizedEnv: []string{"AWS_PROFILE=atmos", "PATH=/bin"}, ComponentEnvList: []string{"TF_IN_AUTOMATION=true"}}
	iam := &schema.ComponentIAM{Role: "arn:aws:iam::1:role/x", SessionName: "s", DurationSeconds: 1800}
	require.NoError(t, applyIAMRole(t.Context(), info, iam))
	require.Equal(t, 1, recorded.calls)
	require.Equal(t, awsidentity.InlineRole{Role: "arn:aws:iam::1:role/x", SessionName: "s", Duration: 30 * time.Minute}, recorded.role)
	require.Equal(t, []string{"AWS_PROFILE=atmos", "PATH=/bin", "TF_IN_AUTOMATION=true"}, recorded.environ, "the role is assumed on top of the invocation's own credentials")
	require.Equal(t, []string{"TF_IN_AUTOMATION=true", "AWS_ACCESS_KEY_ID=ASIASESSION", "AWS_PROFILE=", "AWS_SESSION_TOKEN=session-token"}, info.ComponentEnvList)
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
