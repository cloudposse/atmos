package terragruntexec

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields these tests depend on.
var (
	_ = schema.ComponentProviderOption{Value: "", Sensitive: true}
	_ = schema.ConfigAndStacksInfo{ComponentVarsSection: nil, ComponentProvidersSection: nil, UseTerraformPlan: true}
)

func request() *Request {
	return &Request{
		Component: "app", Stack: "live/dev", Binary: "/tools/terragrunt", Engine: "/tools/tofu",
		Root: "project", Unit: "live/dev/app/terragrunt.hcl", SubCommand: "plan",
		Arguments: []string{"-no-color"}, Environment: []string{"AWS_PROFILE=dev"},
	}
}

func envValue(env []string, key string) (string, bool) {
	for index := len(env) - 1; index >= 0; index-- {
		if name, value, found := splitEnv(env[index]); found && name == key {
			return value, true
		}
	}
	return "", false
}

func splitEnv(entry string) (name, value string, found bool) {
	for index := range entry {
		if entry[index] == '=' {
			return entry[:index], entry[index+1:], true
		}
	}
	return entry, "", false
}

// unitDir is the absolute unit directory of request(), whose Root is relative.
func unitDir(t *testing.T) string {
	t.Helper()
	directory, err := filepath.Abs(filepath.Join("project", "live", "dev", "app"))
	require.NoError(t, err)
	return directory
}

func TestBuildCommandRunsTheUnitDirectoryWithEngineArgumentsAfterTheSeparator(t *testing.T) {
	command := BuildCommand(request())
	directory := unitDir(t)
	require.True(t, filepath.IsAbs(command.WorkingDir), "a relative root is never resolved twice from the unit directory")
	assert.Equal(t, []string{"--working-dir", directory, "run", "--", "plan", "-no-color"}, command.Args)
	assert.Equal(t, directory, command.WorkingDir)
	assert.Equal(t, "/tools/terragrunt", command.Binary)
	assert.False(t, command.DryRun)
}

func TestBuildCommandEnvironment(t *testing.T) {
	command := BuildCommand(request())
	value, found := envValue(command.Env, "AWS_PROFILE")
	assert.True(t, found)
	assert.Equal(t, "dev", value)
	for key, want := range map[string]string{"TG_NON_INTERACTIVE": "true", "TG_TF_FORWARD_STDOUT": "true", "TG_TF_PATH": "/tools/tofu"} {
		got, found := envValue(command.Env, key)
		assert.True(t, found, key)
		assert.Equal(t, want, got, key)
	}

	// A component `env` entry wins over the default Atmos would add.
	explicit := request()
	explicit.Environment = append(explicit.Environment, "TG_TF_PATH=/custom/tofu", "TG_NON_INTERACTIVE=false")
	got, _ := envValue(BuildCommand(explicit).Env, "TG_TF_PATH")
	assert.Equal(t, "/custom/tofu", got)
	got, _ = envValue(BuildCommand(explicit).Env, "TG_NON_INTERACTIVE")
	assert.Equal(t, "false", got)

	// The input slice is never modified.
	assert.Equal(t, []string{"AWS_PROFILE=dev"}, request().Environment)
}

func TestBuildCommandMapsSubcommands(t *testing.T) {
	cases := map[string]struct {
		subcommand, second string
		arguments          []string
		want               []string
	}{
		"deploy applies without asking": {subcommand: "deploy", want: []string{"apply", "-auto-approve"}},
		"apply keeps user arguments":    {subcommand: "apply", arguments: []string{"-auto-approve"}, want: []string{"apply", "-auto-approve"}},
		"workspace subcommand":          {subcommand: "workspace", second: "select", arguments: []string{"dev"}, want: []string{"workspace", "select", "dev"}},
		"output json":                   {subcommand: "output", arguments: []string{"-json"}, want: []string{"output", "-json"}},
		"no arguments":                  {subcommand: "validate", want: []string{"validate"}},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			r := request()
			r.SubCommand, r.SubCommand2, r.Arguments = test.subcommand, test.second, test.arguments
			args := BuildCommand(r).Args
			separator := -1
			for index, arg := range args {
				if arg == "--" {
					separator = index
					break
				}
			}
			require.GreaterOrEqual(t, separator, 0)
			assert.Equal(t, test.want, args[separator+1:])
		})
	}
}

func TestBuildCommandUsesConfigAndSkipInit(t *testing.T) {
	r := request()
	r.Unit, r.SkipInit = "live/dev/app/custom.hcl", true
	args := BuildCommand(r).Args
	directory := unitDir(t)
	assert.Equal(t, []string{"--working-dir", directory, "run", "--config", filepath.Join(directory, "custom.hcl"), "--no-auto-init", "--"}, args[:7])
}

func TestBuildCommandOptions(t *testing.T) {
	r := request()
	r.Features = map[string]any{"alpha": true, "beta": "from-config"}
	r.Options = map[string]schema.ComponentProviderOption{
		"feature":                            {Value: map[string]any{"beta": "from-cli", "gamma": "3"}},
		"iam_assume_role":                    {Value: "arn:aws:iam::1:role/x"},
		"iam_assume_role_duration":           {Value: 900},
		"iam_assume_role_session_name":       {Value: "ci"},
		"download_dir":                       {Value: "/cache"},
		"iam_assume_role_web_identity_token": {Value: "s3cret", Sensitive: true},
	}
	command := BuildCommand(r)
	want := []string{
		"--feature", "alpha=true", "--feature", "beta=from-cli", "--feature", "gamma=3",
		"--iam-assume-role", "arn:aws:iam::1:role/x", "--iam-assume-role-duration", "900",
		"--iam-assume-role-session-name", "ci", "--download-dir", "/cache", "--",
	}
	assert.Equal(t, want, command.Args[3:3+len(want)])
	for _, arg := range command.Args {
		assert.NotContains(t, arg, "s3cret", "a sensitive value never appears in argv")
	}
	token, found := envValue(command.Env, "TG_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN")
	assert.True(t, found)
	assert.Equal(t, "s3cret", token)
}

func TestBuildCommandMergesEnvironmentFeaturesPerName(t *testing.T) {
	t.Setenv("ATMOS_TERRAGRUNT_FEATURE", "")
	os.Unsetenv("ATMOS_TERRAGRUNT_FEATURE")
	t.Setenv("TG_FEATURE", "tier=environment,region=from-env")
	r := request()
	r.Features = map[string]any{"tier": "config", "size": "config"}
	r.Options = map[string]schema.ComponentProviderOption{"feature": {Value: map[string]any{"region": "from-cli"}}}
	want := []string{"--feature", "region=from-cli", "--feature", "size=config", "--feature", "tier=environment", "--"}
	assert.Equal(t, want, BuildCommand(r).Args[3:3+len(want)], "configuration, then environment, then command line")

	// A component `env` entry is the environment Terragrunt sees, so it wins over the process.
	r.Environment = append(r.Environment, "TG_FEATURE=tier=component")
	want = []string{"--feature", "region=from-cli", "--feature", "size=config", "--feature", "tier=component", "--"}
	assert.Equal(t, want, BuildCommand(r).Args[3:3+len(want)])
}

func TestBuildCommandOmitsAbsentOptions(t *testing.T) {
	r := request()
	assert.Equal(t, []string{"--working-dir", unitDir(t), "run", "--", "plan", "-no-color"}, BuildCommand(r).Args)
	_, found := envValue(BuildCommand(r).Env, "TG_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN")
	assert.False(t, found)
}

func unitInfo(subcommand string) *schema.ConfigAndStacksInfo {
	return &schema.ConfigAndStacksInfo{
		SubCommand: subcommand, ComponentFromArg: "app", Stack: "live/dev",
		ComponentSettingsSection: map[string]any{"configuration": map[string]any{"engine": "terragrunt"}},
	}
}

func TestOwnsFollowsTheDiscoveryEngine(t *testing.T) {
	assert.True(t, Owns(unitInfo("plan")))
	native := unitInfo("plan")
	native.ComponentSettingsSection["configuration"] = map[string]any{"engine": "opentofu"}
	assert.False(t, Owns(native))
	assert.False(t, Owns(&schema.ConfigAndStacksInfo{}))
}

func TestCheckSupported(t *testing.T) {
	for _, command := range []string{"plan", "apply", "deploy", "destroy", "init", "output", "state", "workspace", "validate", "import"} {
		require.NoError(t, CheckSupported(unitInfo(command)), command)
	}
	for _, command := range []string{"shell", "clean", "generate", "console-ui"} {
		require.ErrorIs(t, CheckSupported(unitInfo(command)), errUtils.ErrExecutableModeUnsupported, command)
	}

	saved := unitInfo("apply")
	saved.UseTerraformPlan = true
	require.ErrorIs(t, CheckSupported(saved), errUtils.ErrExecutableModeUnsupported)
	file := unitInfo("apply")
	file.PlanFile = "plan.tfplan"
	require.ErrorIs(t, CheckSupported(file), errUtils.ErrExecutableModeUnsupported)

	vars := unitInfo("plan")
	vars.ComponentVarsSection = map[string]any{"region": "us-east-1"}
	require.ErrorIs(t, CheckSupported(vars), errUtils.ErrExecutableModeUnsupported)
	providers := unitInfo("plan")
	providers.ComponentProvidersSection = map[string]any{"aws": map[string]any{}}
	require.ErrorIs(t, CheckSupported(providers), errUtils.ErrExecutableModeUnsupported)
}

func TestCheckVersion(t *testing.T) {
	cases := map[string]struct {
		output string
		err    error
		target error
	}{
		"current":        {output: "terragrunt version v1.2.1"},
		"minimum":        {output: "terragrunt version v1.0.0"},
		"without prefix": {output: "terragrunt version 2.0.0\n"},
		"too old":        {output: "terragrunt version v0.99.9", target: errUtils.ErrExecutableVersion},
		"prerelease":     {output: "terragrunt version v1.0.0-rc1", target: errUtils.ErrExecutableVersion},
		"newer release":  {output: "terragrunt version v1.1.0-beta.2+abc"},
		"unreadable":     {output: "unexpected", target: errUtils.ErrExecutableVersion},
		"missing":        {err: exec.ErrNotFound, target: errUtils.ErrExecutableNotFound},
		"failing":        {err: errors.New("exit status 1"), target: errUtils.ErrExecutableNotFound},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			host := NewMockHost(gomock.NewController(t))
			host.EXPECT().Version(gomock.Any(), "terragrunt", []string{"A=1"}).Return(test.output, test.err)
			err := CheckVersion(context.Background(), host, "terragrunt", []string{"A=1"})
			if test.target == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, test.target)
		})
	}
}

func TestRunChecksTheVersionThenExecutes(t *testing.T) {
	controller := gomock.NewController(t)
	host := NewMockHost(controller)
	gomock.InOrder(
		host.EXPECT().Version(gomock.Any(), "/tools/terragrunt", gomock.Any()).Return("terragrunt version v1.2.1", nil),
		host.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, command *Command) error {
			assert.Equal(t, "/tools/terragrunt", command.Binary)
			assert.Equal(t, "run", command.Args[2])
			return nil
		}),
	)
	require.NoError(t, Run(context.Background(), host, request()))
}

func TestRunPropagatesTheUnitFailure(t *testing.T) {
	failure := errUtils.ExitCodeError{Code: 3}
	host := NewMockHost(gomock.NewController(t))
	host.EXPECT().Version(gomock.Any(), gomock.Any(), gomock.Any()).Return("terragrunt version v1.2.1", nil)
	host.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(failure)
	err := Run(context.Background(), host, request())
	var exit errUtils.ExitCodeError
	require.ErrorAs(t, err, &exit)
	assert.Equal(t, 3, exit.Code)
}

func TestRunDoesNotExecuteWhenTheVersionIsUnsupported(t *testing.T) {
	host := NewMockHost(gomock.NewController(t))
	host.EXPECT().Version(gomock.Any(), gomock.Any(), gomock.Any()).Return("terragrunt version v0.1.0", nil)
	host.EXPECT().Execute(gomock.Any(), gomock.Any()).Times(0)
	require.ErrorIs(t, Run(context.Background(), host, request()), errUtils.ErrExecutableVersion)
}

func TestDryRunSkipsTheVersionProbe(t *testing.T) {
	host := NewMockHost(gomock.NewController(t))
	host.EXPECT().Version(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	host.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, command *Command) error {
		assert.True(t, command.DryRun)
		return nil
	})
	r := request()
	r.DryRun = true
	require.NoError(t, Run(context.Background(), host, r))
}

func TestUnitDirectory(t *testing.T) {
	directory, config := UnitDirectory("project", "live/dev/app/terragrunt.hcl")
	assert.Equal(t, filepath.Join("project", "live", "dev", "app"), directory)
	assert.Empty(t, config)
	directory, config = UnitDirectory("project", "live/dev/app/other.hcl")
	assert.Equal(t, filepath.Join("project", "live", "dev", "app"), directory)
	assert.Equal(t, filepath.Join("project", "live", "dev", "app", "other.hcl"), config)
}

func TestCancellationIsNotReportedAsAMissingExecutable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	host := NewMockHost(gomock.NewController(t))
	host.EXPECT().Version(gomock.Any(), gomock.Any(), gomock.Any()).Return("", errors.New("signal: killed"))
	host.EXPECT().Execute(gomock.Any(), gomock.Any()).Times(0)
	require.ErrorIs(t, Run(ctx, host, request()), context.Canceled)
}
