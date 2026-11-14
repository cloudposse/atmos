package exec

import (
	bytes "bytes"
	context "context"
	errors "errors"
	url "net/url"
	os "os"
	filepath "path/filepath"
	strings "strings"
	testing "testing"
	time "time"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
	component "github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	schema "github.com/cloudposse/atmos/pkg/schema"
	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	plumbing "github.com/go-git/go-git/v5/plumbing"
	object "github.com/go-git/go-git/v5/plumbing/object"
	assert "github.com/stretchr/testify/assert"
	require "github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

// The compatibility entry points must preserve scheduler selection and dispatch
// through the registered provider, including providers that replace the builtin.
func TestTerraformAdapterScheduledRegistryBoundary(t *testing.T) {
	for _, mode := range []string{"query", "all", "affected", "affected with context"} {
		t.Run(mode, func(t *testing.T) {
			args, baseline, _ := affectedDispatchFixture(t)
			args.RepoPath = baseline
			provider := replaceTerraformProvider(t)
			provider.EXPECT().Execute(gomock.Any()).DoAndReturn(func(request *component.ExecutionContext) error {
				require.Equal(t, "app", request.Component)
				require.Equal(t, "dev", request.Stack)
				require.Equal(t, "plan", request.SubCommand)
				require.Equal(t, []string{"-lock=false"}, request.Args)
				if mode == "affected with context" {
					require.Equal(t, "caller", request.Context.Value(adapterContextKey{}))
				}
				return nil
			}).Times(1)
			info := &schema.ConfigAndStacksInfo{
				Stack: "dev", SubCommand: "plan", Identity: cfg.IdentityFlagDisabledValue,
				AdditionalArgsAndFlags: []string{"-lock=false"},
			}
			var err error
			switch mode {
			case "query":
				info.Query = `.vars.size == "large"`
				err = ExecuteTerraformQuery(info)
			case "all":
				err = ExecuteTerraformAll(info)
			case "affected":
				err = ExecuteTerraformAffected(args, info)
			default:
				ctx := context.WithValue(t.Context(), adapterContextKey{}, "caller")
				err = ExecuteTerraformAffectedWithContext(ctx, args, info)
			}
			require.NoError(t, err)
		})
	}
}

type adapterContextKey struct{}

func TestTerraformAdapterTargetsAndShell(t *testing.T) {
	args, _, _ := affectedDispatchFixture(t)
	targets, err := ListTerraformComponentTargets(args.CLIConfig, "dev", []string{"app"}, `.vars.size == "large"`)
	require.NoError(t, err)
	require.Equal(t, []ComponentStack{{Component: "app", Stack: "dev"}}, targets)
	targets, err = ListTerraformComponentTargets(args.CLIConfig, "dev", []string{"app"}, `.vars.size == "small"`)
	require.NoError(t, err)
	require.Empty(t, targets)

	// Dry-run exercises stack/auth resolution while requiring no Terraform binary or interactive terminal.
	require.NoError(t, ExecuteTerraformShell(&ShellOptions{Component: "app", Stack: "dev", Identity: cfg.IdentityFlagDisabledValue, DryRun: true}, args.CLIConfig))
	_, err = ListTerraformComponentTargets(args.CLIConfig, "dev", nil, `.vars.size == (`)
	require.Error(t, err, "invalid query errors must cross the host adapter")
}

func TestTerraformAdapterAuthErrorContract(t *testing.T) {
	for _, mode := range []string{"terraform", "shared component"} {
		t.Run(mode, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			info := &schema.ConfigAndStacksInfo{Identity: "selected", ComponentFromArg: "app", Stack: "dev"}
			original := defaultAuthManagerCreator
			t.Cleanup(func() { defaultAuthManagerCreator = original })
			failure := errors.New("credential provider unavailable")
			defaultAuthManagerCreator = func(identity string, _ *schema.AuthConfig, _ string, received *schema.AtmosConfiguration, reexec auth.ReExecContext) (auth.AuthManager, error) {
				require.Same(t, config, received)
				require.Equal(t, "selected", identity)
				require.Equal(t, "app", reexec.Component)
				require.Equal(t, "dev", reexec.Stack)
				return nil, failure
			}
			merged := defaultMergedAuthConfigGetter
			t.Cleanup(func() { defaultMergedAuthConfigGetter = merged })
			defaultMergedAuthConfigGetter = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (*schema.AuthConfig, error) {
				return &schema.AuthConfig{}, nil
			}
			var err error
			if mode == "terraform" {
				_, err = SetupTerraformAuthForCLI(config, info)
			} else {
				_, err = SetupComponentAuthForCLI(config, info)
			}
			require.ErrorIs(t, err, failure)
			require.ErrorIs(t, err, errUtils.ErrFailedToInitializeAuthManager)
			require.Nil(t, info.AuthManager)
		})
	}
}

func TestTerraformAdapterRetryPreservesCallerCapture(t *testing.T) {
	attempts, delay := 2, time.Millisecond
	info := &schema.ConfigAndStacksInfo{ComponentRetrySection: &schema.RetryConfig{
		MaxAttempts: &attempts, InitialDelay: &delay, Conditions: []string{"transient failure"},
	}}
	var captured bytes.Buffer
	calls := 0
	failure := errors.New("command failed")
	err := ExecuteShellCommandWithRetry(&schema.AtmosConfiguration{}, info, "plan", func(opts ...ShellCommandOption) error {
		calls++
		var options component.ExecutionOptions
		for _, option := range opts {
			option(&options)
		}
		require.Same(t, t.Context(), options.Ctx)
		_, err := options.StderrCapture.Write([]byte("transient failure\n"))
		require.NoError(t, err)
		return failure
	}, WithProcessContext(t.Context()), WithStderrCapture(&captured))
	require.ErrorIs(t, err, failure)
	require.Equal(t, 2, calls)
	require.Equal(t, "transient failure\ntransient failure\n", captured.String())
}

func TestTerraformAdapterStaticOutputsAndDeclaredVariables(t *testing.T) {
	args, _, _ := affectedDispatchFixture(t)
	config := args.CLIConfig
	manifestPath := filepath.Join(config.BasePath, "stacks", "dev.yaml")
	manifest, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	manifest = append(manifest, []byte("      remote_state_backend_type: static\n      remote_state_backend:\n        static: {endpoint: fixture-endpoint}\n")...)
	require.NoError(t, os.WriteFile(manifestPath, manifest, 0o600))
	ClearFindStacksMapCache()
	t.Cleanup(ClearFindStacksMapCache)
	outputs, err := GetAllTerraformOutputs(config, "app", "dev", true, nil, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"endpoint": "fixture-endpoint"}, outputs)
	value, exists, err := GetStaticRemoteStateOutput(config, "app", "dev", outputs, ".endpoint")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, "fixture-endpoint", value)

	info, err := ProcessStacks(config, schema.ConfigAndStacksInfo{
		ComponentFromArg: "app", ComponentType: "terraform", Stack: "dev",
	}, true, false, false, nil, nil)
	require.NoError(t, err)
	declared, found := TerraformDeclaredVarType(info.ComponentSection, "size")
	require.True(t, found)
	require.Equal(t, "string", declared)
	_, found = TerraformDeclaredVarType(info.ComponentSection, "not_declared")
	require.False(t, found)
}

func TestShellOptionsForUI(t *testing.T) {
	tests := []struct {
		name      string
		component string
		stack     string
	}{
		{
			name:      "typical values",
			component: "vpc",
			stack:     "dev-us-west-2",
		},
		{
			name:      "empty values",
			component: "",
			stack:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := shellOptionsForUI(tt.component, tt.stack)
			assert.Equal(t, tt.component, opts.Component)
			assert.Equal(t, tt.stack, opts.Stack)
			assert.True(t, opts.ProcessTemplates, "UI path must enable template processing")
			assert.True(t, opts.ProcessFunctions, "UI path must enable function processing")
			assert.False(t, opts.DryRun, "UI path does not support dry-run")
			assert.Empty(t, opts.Identity, "UI path does not support identity selection")
			assert.Empty(t, opts.Skip, "UI path does not support skip")
		})
	}
}

func TestShellOptionsForUI_DefaultProcessingOptions(t *testing.T) {
	// Verify that UI shell options have correct default processing options.
	opts := shellOptionsForUI("vpc", "dev")

	assert.True(t, opts.ProcessTemplates)
	assert.True(t, opts.ProcessFunctions)
	assert.Empty(t, opts.Skip)
}

// affectedDispatchFixture uses a local remote so every target mode compares the same
// two commits without network access or rewriting the caller's checkout.
func affectedDispatchFixture(t *testing.T) (*DescribeAffectedCmdArgs, string, string) {
	t.Helper()
	manifest := `name: dev
components:
  terraform:
    app:
      metadata:
        tags: [service]
        labels: {owner: platform}
      settings: {marker: retained}
      vars: {size: small}
`
	root, repo := affectedPolicyRepo(t, map[string]string{
		"atmos.yaml":                       "base_path: .\ncomponents:\n  terraform:\n    base_path: components/terraform\nstacks:\n  base_path: stacks\n  included_paths: ['**/*']\n",
		"stacks/dev.yaml":                  manifest,
		"components/terraform/app/main.tf": "variable \"size\" { type = string }\n",
	})
	remoteURL := (&url.URL{Scheme: "file", Host: "localhost", Path: "/" + strings.TrimPrefix(filepath.ToSlash(root), "/")}).String()
	_, err := repo.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{remoteURL}})
	require.NoError(t, err)
	base, err := repo.Head()
	require.NoError(t, err)
	headDir := t.TempDir()
	head, err := git.PlainClone(headDir, false, &git.CloneOptions{URL: remoteURL})
	require.NoError(t, err)
	require.NoError(t, head.Storer.SetReference(plumbing.NewHashReference("refs/heads/baseline", base.Hash())))
	require.NoError(t, os.WriteFile(filepath.Join(headDir, "stacks", "dev.yaml"), []byte(strings.ReplaceAll(manifest, "small", "large")), 0o600))
	worktree, err := head.Worktree()
	require.NoError(t, err)
	_, err = worktree.Add(".")
	require.NoError(t, err)
	_, err = worktree.Commit("change app", &git.CommitOptions{Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Unix(2, 0)}})
	require.NoError(t, err)
	t.Chdir(headDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", headDir)
	t.Setenv("ATMOS_BASE_PATH", headDir)
	t.Setenv("ATMOS_POLICY_DIR", "")
	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	return &DescribeAffectedCmdArgs{CLIConfig: &config, AuthDisabled: true, IncludeSettings: true}, root, base.Hash().String()
}

func TestGetAffectedComponentsDispatchRealRepositories(t *testing.T) {
	for _, mode := range []string{"repo path", "local reference", "clone default", "clone commit"} {
		t.Run(mode, func(t *testing.T) {
			args, baseline, sha := affectedDispatchFixture(t)
			switch mode {
			case "repo path":
				args.RepoPath = baseline
				// An explicit path takes precedence over the other target selectors.
				args.CloneTargetRef, args.Ref = true, "refs/heads/missing"
			case "local reference":
				args.Ref = "refs/heads/baseline"
			case "clone default":
				args.CloneTargetRef = true
			case "clone commit":
				args.CloneTargetRef, args.Ref, args.SHA = true, "refs/heads/master", sha
			}
			progress := 0
			affected, err := GetAffectedComponentsWithOptions(args, DescribeStacksErrorOptions{
				OnProgress: func(file string, index, total int) {
					require.NotEmpty(t, file)
					require.Less(t, index, total)
					progress++
				},
			})
			require.NoError(t, err)
			require.Len(t, affected, 1)
			require.Equal(t, "app", affected[0].Component)
			require.Equal(t, "dev", affected[0].Stack)
			require.Equal(t, "retained", affected[0].Settings["marker"])
			require.GreaterOrEqual(t, progress, 2, "options reach both HEAD and BASE stack processing")

			args.Labels = map[string]string{"owner": "other-team"}
			affected, err = GetAffectedComponents(args)
			require.NoError(t, err)
			require.Empty(t, affected, "the dispatcher must forward metadata filters")
		})
	}
}

func TestGetAffectedComponentsDispatchErrors(t *testing.T) {
	_, err := GetAffectedComponents(nil)
	require.ErrorIs(t, err, errUtils.ErrNilParam)
	for _, mode := range []string{"invalid target directory", "unknown local reference", "invalid SSH key"} {
		t.Run(mode, func(t *testing.T) {
			args, _, _ := affectedDispatchFixture(t)
			switch mode {
			case "invalid target directory":
				args.RepoPath = t.TempDir()
			case "unknown local reference":
				args.Ref = "refs/heads/does-not-exist"
			case "invalid SSH key":
				args.CloneTargetRef = true
				args.SSHKeyPath = filepath.Join(t.TempDir(), "id_test")
				require.NoError(t, os.WriteFile(args.SSHKeyPath, []byte("invalid private key"), 0o600))
			}
			affected, err := GetAffectedComponents(args)
			require.Error(t, err)
			require.Nil(t, affected)
			if mode == "invalid target directory" {
				require.ErrorIs(t, err, RemoteRepoIsNotGitRepoError)
			}
			if mode == "invalid SSH key" {
				require.ErrorContains(t, err, "no key found")
			}
		})
	}
}
