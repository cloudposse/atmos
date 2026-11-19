package terraform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	componentpkg "github.com/cloudposse/atmos/pkg/component"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/policy/planfile"
	"github.com/cloudposse/atmos/pkg/provisioner"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
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
	// A local module is refreshed on every run; see TestPrepareConfigurationRefreshesOnlyLocalModules.
	require.Equal(t, map[string]any{"uri": expected.Source, "ttl": "0s"}, info.ComponentSection["source"])
	require.Equal(t, expected.Backend, info.ComponentBackendSection)
	require.Equal(t, "default", info.TerraformWorkspace)
	require.True(t, config.Components.Terraform.AutoGenerateBackendFile)
}

func TestPrepareConfigurationRefreshesOnlyLocalModules(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		ttl    any
	}{
		{name: "relative path", source: "./modules/unit", ttl: "0s"},
		{name: "absolute path", source: filepath.Join(t.TempDir(), "module"), ttl: "0s"},
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
			require.Equal(t, test.ttl, source["ttl"])
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

func TestRejectRuntimeArtifacts(t *testing.T) {
	require.NoError(t, rejectRuntimeArtifacts([]schema.ComponentConfigurationFile{{Path: "backend.tf", IfExists: "overwrite"}}))
	for _, path := range []string{"examples/terraform.tfstate", "nested/provider.tf", "Nested/PROVIDER.tf", "terraform.tfstate.example"} {
		require.NoError(t, rejectRuntimeArtifacts([]schema.ComponentConfigurationFile{{Path: path}}), path)
	}
	for _, path := range []string{"terraform.tfstate", "terraform.tfstate.backup", ".terraform.tfstate.lock.info", ".terraform/environment", "nested/.terraform/modules/modules.json", "terraform.tfstate.d/stage/terraform.tfstate", ".terraform.lock.hcl", "nested/.dev-app.terraform.lock.hcl", ".atmos/metadata.json"} {
		files := []schema.ComponentConfigurationFile{{Path: "backend.tf", IfExists: "overwrite"}, {Path: path, IfExists: "overwrite"}}
		require.ErrorIs(t, rejectRuntimeArtifacts(files), errUtils.ErrPluginInvalidConfig, path)
		files[1].Path = strings.ToUpper(path)
		require.ErrorIs(t, rejectRuntimeArtifacts(files), errUtils.ErrPluginInvalidConfig, files[1].Path)
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
	runtime := NewMockReviewedPlanRuntime(gomock.NewController(t))
	runtime.EXPECT().ResolveState(gomock.Any(), gomock.Any()).Return(nil, failure)
	config := &schema.AtmosConfiguration{ComponentConfigurations: runtime}

	result, err := executor.GetTerraformState(config, "!terraform.state", "live", "network", "vpc_id", true, nil, nil)
	require.ErrorIs(t, err, failure, "the provider's failure is returned as is, not retried against a guessed backend")
	require.Nil(t, result)
}

func TestConfigurationWorkdirOwnsArtifactsNotState(t *testing.T) {
	for _, mode := range []string{"component directory", "isolated workdir", "explicit workdir", "relative workdir"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "module")
			require.NoError(t, os.MkdirAll(source, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(source, "main.tf"), []byte("variable \"value\" {}\n"), 0o644))
			state := filepath.Join(root, "terragrunt-state", "terraform.tfstate")
			require.NoError(t, os.MkdirAll(filepath.Dir(state), 0o755))
			require.NoError(t, os.WriteFile(state, []byte("existing state"), 0o600))
			config := &schema.AtmosConfiguration{BasePath: root, BasePathAbsolute: root, TerraformDirAbsolutePath: filepath.Join(root, "components")}
			config.Components.Terraform.BasePath = "components"
			provider := NewMockComponentConfigurationRuntime(gomock.NewController(t))
			config.ComponentConfigurations = provider
			binary, err := os.Executable()
			require.NoError(t, err)
			paths := map[string]string{}
			for _, stack := range []string{"dev", "prod"} {
				if mode == "component directory" && stack == "prod" {
					continue
				}
				info := &schema.ConfigAndStacksInfo{Stack: stack, ComponentFromArg: "app", Component: "app", FinalComponent: "app", Command: binary, SubCommand: "plan", ComponentSettingsSection: map[string]any{"configuration": true}, ComponentEnvSection: map[string]any{}, ComponentSection: map[string]any{"component": "app", "atmos_component": "app", "atmos_stack": stack, "source": map[string]any{"uri": "./module"}}}
				if mode != "component directory" {
					info.ComponentSection["provision"] = map[string]any{"workdir": map[string]any{"enabled": true}}
				}
				if mode == "explicit workdir" {
					info.ComponentSection["metadata"] = map[string]any{"working_directory": filepath.Join(root, "chosen", stack)}
				}
				if mode == "relative workdir" {
					info.ComponentSection["settings"] = map[string]any{"working_directory": filepath.Join("chosen", stack)}
				}
				for _, revision := range []string{"first", "second"} {
					provider.EXPECT().Prepare(gomock.Any(), info).Return(&schema.PreparedComponentConfiguration{Engine: "terraform", Source: source, Inputs: map[string]any{"value": revision}, BackendType: "local", Backend: map[string]any{"path": state}, Workspace: "default", Files: []schema.ComponentConfigurationFile{{Path: "generated.tf", Content: []byte("# {{ literal }} " + revision), IfExists: "overwrite"}}}, nil)
					execution, err := testExecutor(t).prepareComponentExecution(context.Background(), provisioner.OutputWriters{}, config, info, false)
					require.NoError(t, err)
					require.Equal(t, execution.componentPath, execution.workingDir)
					if mode != "component directory" {
						require.Equal(t, execution.workingDir, info.ComponentSection[provWorkdir.WorkdirPathKey])
						require.NotEqual(t, filepath.Join(root, "components", "app"), execution.workingDir)
					}
					if mode == "explicit workdir" || mode == "relative workdir" {
						require.Equal(t, filepath.Join(root, "chosen", stack), execution.workingDir)
					}
					generated, err := os.ReadFile(filepath.Join(execution.workingDir, "generated.tf"))
					require.NoError(t, err)
					require.Equal(t, "# {{ literal }} "+revision, string(generated))
					backend, err := os.ReadFile(filepath.Join(execution.workingDir, "backend.tf.json"))
					require.NoError(t, err)
					var decoded map[string]any
					require.NoError(t, json.Unmarshal(backend, &decoded))
					require.Equal(t, state, decoded["terraform"].(map[string]any)["backend"].(map[string]any)["local"].(map[string]any)["path"])
					contents, err := os.ReadFile(state)
					require.NoError(t, err)
					require.Equal(t, "existing state", string(contents))
					require.NoFileExists(t, filepath.Join(source, "generated.tf"))
					require.NoFileExists(t, filepath.Join(source, "backend.tf.json"))
					if previous := paths[stack]; previous != "" {
						require.Equal(t, previous, execution.workingDir)
					}
					paths[stack] = execution.workingDir
				}
			}
			if mode != "component directory" {
				require.NotEqual(t, paths["dev"], paths["prod"])
			}
		})
	}
}

func TestConfigurationExplicitWorkdirRejectsAnotherOwner(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "module")
	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "main.tf"), []byte("# source"), 0o644))
	target := filepath.Join(root, "chosen")
	config := &schema.AtmosConfiguration{BasePath: root, BasePathAbsolute: root, TerraformDirAbsolutePath: filepath.Join(root, "components")}
	config.Components.Terraform.BasePath = "components"
	provider := NewMockComponentConfigurationRuntime(gomock.NewController(t))
	config.ComponentConfigurations = provider
	binary, err := os.Executable()
	require.NoError(t, err)
	for _, stack := range []string{"dev", "prod"} {
		info := &schema.ConfigAndStacksInfo{Stack: stack, ComponentFromArg: "app", Component: "app", FinalComponent: "app", Command: binary, SubCommand: "plan", ComponentSettingsSection: map[string]any{"configuration": true}, ComponentEnvSection: map[string]any{}, ComponentSection: map[string]any{"component": "app", "atmos_component": "app", "atmos_stack": stack, "source": map[string]any{"uri": source}, "provision": map[string]any{"workdir": map[string]any{"enabled": true}}, "metadata": map[string]any{"working_directory": target}}}
		provider.EXPECT().Prepare(gomock.Any(), info).Return(&schema.PreparedComponentConfiguration{Engine: "terraform", Source: source, Inputs: map[string]any{}, BackendType: "local", Backend: map[string]any{}, Workspace: "default", Files: []schema.ComponentConfigurationFile{{Path: "generated.tf", Content: []byte(stack), IfExists: "overwrite"}}}, nil)
		_, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, config, info, false)
		if stack == "dev" {
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(target, "terraform.tfstate"), []byte("existing state"), 0o600))
		} else {
			require.ErrorIs(t, err, errUtils.ErrWorkdirProvision)
		}
		generated, err := os.ReadFile(filepath.Join(target, "generated.tf"))
		require.NoError(t, err)
		require.Equal(t, "dev", string(generated))
		state, err := os.ReadFile(filepath.Join(target, "terraform.tfstate"))
		require.NoError(t, err)
		require.Equal(t, "existing state", string(state))
	}
}

func TestReviewedPlanKeepsGeneratedErrorPolicyFile(t *testing.T) {
	project := newReviewedPlanProject(t)
	plan := project.info(t, "plan")
	prepared := project.prepared()
	prepared.Files = []schema.ComponentConfigurationFile{{Path: "labels.tf", Content: []byte("# reviewed labels"), IfExists: "error", Mode: 0o600}}
	project.provider.EXPECT().Prepare(gomock.Any(), plan).Return(prepared, nil)
	execution, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, plan, false)
	require.NoError(t, err)
	path := filepath.Join(execution.workingDir, "reviewed.planfile")
	require.NoError(t, os.WriteFile(path, []byte("reviewed plan"), 0o600))
	observeFixturePlan(t, project.config, plan, execution, path)
	require.NoError(t, recordConfigurationPlan(t.Context(), project.config, plan, execution, []string{"-out=" + path}))
	apply := project.reviewedApply(t, path)
	_, err = testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, apply, false)
	require.NoError(t, err)
	contents, err := os.ReadFile(filepath.Join(project.workdir, "labels.tf"))
	require.NoError(t, err)
	require.Equal(t, "# reviewed labels", string(contents))
}

func TestReviewedApplyRejectsBeforeHookChanges(t *testing.T) {
	for _, target := range []string{"plan", "workdir"} {
		t.Run(target, func(t *testing.T) {
			project := newReviewedPlanProject(t)
			path := project.review(t)
			apply := project.reviewedApply(t, path)
			_, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, apply, false)
			require.NoError(t, err)
			apply.ComponentExecution = &schema.ComponentExecution{BeforeHooks: []schema.ComponentHook{execHook("change", "apply")}}
			shell := newShellFixture(t, func(call *shellCall) error {
				require.Equal(t, "hook-change", call.command, "apply subprocess must not start")
				changed := path
				if target == "workdir" {
					changed = filepath.Join(project.workdir, "main.tf")
				}
				return os.WriteFile(changed, []byte("changed by before hook"), 0o600)
			})
			err = hookExecutor(shell).runMainShellCommand(project.config, apply, []string{"apply", path}, project.workdir, nil)
			require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
			require.Len(t, shell.calls, 1)
		})
	}
}

func TestPolicySnapshotRejectsPlanRewrittenBeforePublication(t *testing.T) {
	for _, rewrite := range []bool{false, true} {
		t.Run(fmt.Sprint(rewrite), func(t *testing.T) {
			config, info, execution, _, published := planFixture(t)
			private := filepath.Join(t.TempDir(), "policy.plan")
			execution.planPolicy = &planfile.Guard{}
			ctx := execution.observePlanSnapshots(t.Context(), config, info, []string{"plan", "-out=" + published})
			snapshot := ctx.Value(planSnapshotKey{}).(*planSnapshotter)
			run := snapshot.around([]string{"plan", "-out=" + private}, func() error { return os.WriteFile(private, []byte("engine plan"), 0o600) })
			require.NoError(t, run())
			digest, err := fileDigest(private)
			require.NoError(t, err)
			require.Equal(t, digest, snapshot.result.planDigest)
			require.Equal(t, published, snapshot.result.path)
			if rewrite {
				require.NoError(t, os.WriteFile(private, []byte("after hook plan"), 0o600))
			}
			bytes, err := os.ReadFile(private)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(published, bytes, 0o600))
			err = recordConfigurationPlan(t.Context(), config, info, execution, []string{"plan", "-out=" + published})
			if rewrite {
				require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPlanBindingUsesPostInitDirectory(t *testing.T) {
	config, info, execution, _, _ := planFixture(t)
	original := execution.componentPath
	relocated := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(relocated, "main.tf"), []byte("relocated module"), 0o600))
	shell := newShellFixture(t, func(call *shellCall) error {
		require.Equal(t, relocated, call.dir)
		return os.WriteFile(filepath.Join(call.dir, "moved.plan"), []byte("actual plan"), 0o600)
	})
	executor := testExecutor(t)
	executor.services.ExecuteShellCommand = shell.mock.Execute
	err := executor.runMainCommandPhase(config, info, execution, []string{"plan", "-out=moved.plan"}, relocated, false)
	require.NoError(t, err)
	binding, err := loadConfigurationPlan(filepath.Join(relocated, "moved.plan"), info)
	require.NoError(t, err)
	require.Equal(t, relocated, binding.WorkingDirectory)
	require.NoFileExists(t, filepath.Join(original, "moved.plan"+configurationPlanSuffix))
}

func TestPreparedFilesRefreshSensitiveDeclarations(t *testing.T) {
	atmosio.Reset()
	t.Cleanup(atmosio.Reset)
	require.NoError(t, atmosio.Initialize())
	atmosio.ApplyMaskingConfig(&atmosio.Config{DisableMasking: false})
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "main.tf"), []byte(""), 0o600))
	info := &schema.ConfigAndStacksInfo{
		ComponentSection:                map[string]any{"component_info": map[string]any{}},
		ComponentVarsSection:            map[string]any{"token": "private-input", "provider_secret": "", "public": "visible"},
		ConfigurationSensitiveInputKeys: map[string]bool{"provider_secret": true},
	}
	loadTerraformModuleInfo(info, directory)
	require.Empty(t, terraformSensitiveVarKeys(info))
	prepared := &schema.PreparedComponentConfiguration{Files: []schema.ComponentConfigurationFile{{Path: "variables.tf", Content: []byte("variable \"token\" {\n type = string\n sensitive = true\n}\n"), IfExists: "overwrite"}}}
	require.NoError(t, writePreparedConfigurationFiles(info, prepared, directory))
	ComputeTerraformSecretVarKeys(info)
	require.Equal(t, map[string]bool{"token": true, "provider_secret": true}, info.TerraformSecretVarKeys)
	require.Equal(t, map[string]any{"public": "visible"}, VarfileVarsToWrite(info, false, "test.tfvars.json"))
	env, err := SecretVarEnv(info)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"TF_VAR_token=private-input", "TF_VAR_provider_secret="}, env)
}

func TestConfigurationWorkspaceEnvironmentCase(t *testing.T) {
	for _, platform := range []string{"windows", "linux"} {
		for _, key := range []string{"tf_workspace", "Tf_Workspace"} {
			info := &schema.ConfigAndStacksInfo{TerraformWorkspace: "stage", SanitizedEnv: []string{"TF_WORKSPACE=stage"}, ComponentEnvList: []string{key + "=prod"}}
			err := checkConfigurationWorkspaceForPlatform(info, platform)
			if platform == "windows" {
				require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			} else {
				require.NoError(t, err)
			}
			info.ComponentEnvList = []string{key + "=stage"}
			require.NoError(t, checkConfigurationWorkspaceForPlatform(info, platform))
			info.SanitizedEnv = []string{key + "=prod"}
			info.ComponentEnvList = []string{"TF_WORKSPACE=stage"}
			require.NoError(t, checkConfigurationWorkspaceForPlatform(info, platform), "last component value overrides base")
		}
	}
}
