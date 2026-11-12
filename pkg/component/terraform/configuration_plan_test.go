package terraform

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/planfile"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
)

type planRuntimeFixture struct {
	schema.ComponentConfigurationRuntime
	identity string
	state    *schema.ComponentConfigurationState
	err      error
}

func (p *planRuntimeFixture) PlanIdentity(context.Context, *schema.ConfigAndStacksInfo, []schema.ComponentConfigurationSource) (string, error) {
	return p.identity, p.err
}

func (p *planRuntimeFixture) ResolveState(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
	return p.state, p.err
}

func planFixture(t *testing.T) (*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, *componentExecContext, *planRuntimeFixture, string) {
	t.Helper()
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: root, Components: schema.Components{Terraform: schema.Terraform{BasePath: "components"}}}
	info := &schema.ConfigAndStacksInfo{Stack: "live", ComponentFromArg: "network", FinalComponent: "network", SubCommand: "plan", ComponentSettingsSection: map[string]any{"configuration": true}}
	working := ConstructTerraformComponentWorkingDir(config, info)
	require.NoError(t, os.MkdirAll(working, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(working, "main.tf"), []byte("original module"), 0o600))
	binary := filepath.Join(root, "terraform.exe")
	require.NoError(t, os.WriteFile(binary, []byte("selected engine"), 0o700))
	info.Command = binary
	path := filepath.Join(working, "reviewed.planfile")
	require.NoError(t, os.WriteFile(path, []byte("reviewed plan"), 0o600))
	prepared := &schema.PreparedComponentConfiguration{Engine: "terraform", Source: root, Inputs: map[string]any{"value": "reviewed"}, BackendType: "local", Backend: map[string]any{"path": filepath.Join(root, "state")}, Workspace: "default"}
	runtime := &planRuntimeFixture{identity: "provider-source-options", state: &schema.ComponentConfigurationState{BackendType: prepared.BackendType, Backend: prepared.Backend, Workspace: prepared.Workspace}}
	config.ComponentConfigurations = runtime
	execCtx := &componentExecContext{componentPath: working, workingDir: working, preparedConfiguration: prepared}
	return config, info, execCtx, runtime, path
}

func TestConfigurationPlanRestoresReviewedPreparation(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	err := recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"plan", "-out", filepath.Base(path)})
	require.NoError(t, err)
	stat, err := os.Stat(path + configurationPlanSuffix)
	require.NoError(t, err)
	// Windows reports 0666 for every writable file, so POSIX permission bits are only meaningful elsewhere.
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0), stat.Mode().Perm()&0o077)
	}
	info.SubCommand = "apply"
	info.UseTerraformPlan = true
	info.PlanFile = path
	prepared, err := restoreConfigurationPlan(t.Context(), config, info)
	require.NoError(t, err)
	require.Empty(t, prepared.Inputs, "the record stores no inputs")
	require.Equal(t, execCtx.preparedConfiguration.Backend, prepared.Backend, "the verified current backend replaces the unstored one")
	require.True(t, info.UseTerraformPlan)
	require.Equal(t, path, info.PlanFile)
	require.FileExists(t, path)
}

func TestConfigurationPlanRejectsChangedBinding(t *testing.T) {
	for _, change := range []string{"plan", "engine", "provider", "workdir", "downloaded module", "backend", "workspace", "coordinates", "missing binding", "corrupt binding", "missing plan", "runtime error", "unsupported operation", "stored plan"} {
		t.Run(change, func(t *testing.T) {
			config, info, execCtx, runtime, path := planFixture(t)
			module := filepath.Join(execCtx.workingDir, ".terraform", "modules", "child", "script.sh")
			if change == "downloaded module" {
				require.NoError(t, os.MkdirAll(filepath.Dir(module), 0o700))
				require.NoError(t, os.WriteFile(module, []byte("reviewed"), 0o600))
			}
			require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"plan", "-out=" + path}))
			info.SubCommand = "apply"
			info.UseTerraformPlan = true
			info.PlanFile = path
			switch change {
			case "plan":
				require.NoError(t, os.WriteFile(path, []byte("unreviewed"), 0o600))
			case "engine":
				require.NoError(t, os.WriteFile(info.Command, []byte("changed executable"), 0o700))
			case "provider":
				runtime.identity = "other provider"
			case "workdir":
				require.NoError(t, os.WriteFile(filepath.Join(execCtx.workingDir, "main.tf"), []byte("modified"), 0o600))
			case "downloaded module":
				require.NoError(t, os.WriteFile(module, []byte("changed"), 0o600))
			case "backend":
				runtime.state.Backend = map[string]any{"path": "other"}
			case "workspace":
				runtime.state.Workspace = "other"
			case "coordinates":
				info.Stack = "other"
			case "missing binding":
				require.NoError(t, os.Remove(path+configurationPlanSuffix))
			case "corrupt binding":
				require.NoError(t, os.WriteFile(path+configurationPlanSuffix, []byte("not json"), 0o600))
			case "missing plan":
				require.NoError(t, os.Remove(path))
			case "runtime error":
				runtime.err = context.Canceled
			case "unsupported operation":
				info.SubCommand = "destroy"
			case "stored plan":
				info.StoredPlanFile = path
			}
			prepared, err := restoreConfigurationPlan(t.Context(), config, info)
			require.Error(t, err)
			require.Nil(t, prepared)
		})
	}
}

func TestConfigurationPlanRecordingGuards(t *testing.T) {
	config, info, execCtx, runtime, path := planFixture(t)
	for _, args := range [][]string{nil, {"-out"}, {"-out="}} {
		require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, args))
	}
	info.DryRun = true
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path}))
	info.DryRun = false
	info.SubCommand = "apply"
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path}))
	info.SubCommand = "plan"
	config.ComponentConfigurations = nil
	require.ErrorIs(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path}), errUtils.ErrPluginUnsupportedExtension)
	config.ComponentConfigurations = runtime
	runtime.err = context.Canceled
	require.ErrorIs(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path}), context.Canceled)
	runtime.err = nil
	require.Error(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path + "missing"}))
	info.Command = path + "missing"
	require.Error(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path}))
}

func TestConfigurationPlanPathsAndFileErrors(t *testing.T) {
	config, info, _, _, path := planFixture(t)
	require.Equal(t, ConstructTerraformComponentPlanfilePath(config, info), configurationPlanPath(config, info))
	info.PlanFile = filepath.Base(path)
	require.Equal(t, path, configurationPlanPath(config, info))
	info.PlanFile = path
	require.Equal(t, path, configurationPlanPath(config, info))
	require.Error(t, writePlanBinding(filepath.Join(t.TempDir(), "absent", "binding"), nil))
	_, err := fileDigest(t.TempDir())
	require.Error(t, err)
	_, err = configurationWorkdirDigest(filepath.Join(t.TempDir(), "absent"), path)
	require.Error(t, err)
	require.True(t, usesConfigurationPlan(info))
	require.False(t, usesConfigurationPlan(&schema.ConfigAndStacksInfo{}))
}

func TestConfigurationWorkdirDigestIgnoresMutableFiles(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "reviewed.plan")
	first, err := configurationWorkdirDigest(root, plan)
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(root, ".terraform"), 0o700))
	for _, name := range []string{"terraform.tfstate", "reviewed.plan", "reviewed.plan.atmos.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("mutable"), 0o600))
	}
	second, err := configurationWorkdirDigest(root, plan)
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.tf"), []byte("source"), 0o600))
	third, err := configurationWorkdirDigest(root, plan)
	require.NoError(t, err)
	require.NotEqual(t, first, third)
	if err := os.Symlink(filepath.Join(root, "main.tf"), filepath.Join(root, "linked.tf")); err != nil {
		t.Skip("symlinks unavailable")
	}
	_, err = configurationWorkdirDigest(root, plan)
	require.True(t, errors.Is(err, errUtils.ErrPluginUnsupportedExtension))
}

// The binding sits beside the plan in a directory that CI caches and uploads, so it must not
// carry evaluated input values, hook command lines are the only execution data it keeps, and
// backend settings (which can hold credentials) are reduced to a digest.
func TestConfigurationPlanRecordHoldsNoInputsOrBackendValues(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	execCtx.preparedConfiguration.Inputs = map[string]any{"password": "input-secret-value"}
	execCtx.preparedConfiguration.Backend = map[string]any{"access_key": "backend-secret-value"}
	runtime := config.ComponentConfigurations.(*planRuntimeFixture)
	runtime.state.Backend = execCtx.preparedConfiguration.Backend
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out=" + path}))

	data, err := os.ReadFile(path + configurationPlanSuffix)
	require.NoError(t, err)
	require.NotContains(t, string(data), "input-secret-value")
	require.NotContains(t, string(data), "backend-secret-value")
	require.Contains(t, string(data), "BackendDigest")
	// The caller's preparation is untouched, so the rest of the plan command still sees its inputs.
	require.Equal(t, "input-secret-value", execCtx.preparedConfiguration.Inputs["password"])
	require.Equal(t, "backend-secret-value", execCtx.preparedConfiguration.Backend["access_key"])

	info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path
	prepared, err := restoreConfigurationPlan(t.Context(), config, info)
	require.NoError(t, err)
	require.Equal(t, "backend-secret-value", prepared.Backend["access_key"])
}

// Generated files can hold credentials; apply restores them from the verified workdir.
func TestConfigurationPlanRecordHoldsNoSensitiveGeneratedFiles(t *testing.T) {
	for _, policy := range []string{"overwrite", "overwrite_terragrunt", "skip", "error"} {
		t.Run(policy, func(t *testing.T) {
			config, info, execCtx, _, path := planFixture(t)
			backend := `terraform { backend "s3" { access_key = "generated-backend-secret" } }`
			provider := `provider "aws" { secret_key = "generated-provider-secret" }`
			execCtx.preparedConfiguration.Files = []schema.ComponentConfigurationFile{
				{Path: "backend.tf", Content: []byte(backend), IfExists: policy, Sensitive: true},
				{Path: "provider.tf", Content: []byte(provider), IfExists: policy, Sensitive: true},
				{Path: "public.txt", Content: []byte("public-content"), IfExists: "overwrite"},
				{Path: "obsolete.tf", IfExists: "remove"},
			}
			for _, file := range execCtx.preparedConfiguration.Files[:2] {
				require.NoError(t, os.WriteFile(filepath.Join(execCtx.workingDir, file.Path), file.Content, 0o600))
			}
			require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out=" + path}))
			data, err := os.ReadFile(path + configurationPlanSuffix)
			require.NoError(t, err)
			var record configurationPlan
			require.NoError(t, json.Unmarshal(data, &record))
			require.Len(t, record.Prepared.Files, 4)
			for i, file := range execCtx.preparedConfiguration.Files[:2] {
				require.Empty(t, record.Prepared.Files[i].Content, "the decoded record holds no sensitive bytes")
				require.NotEmpty(t, file.Content, "the caller's preparation is untouched")
				require.Equal(t, policy, record.Prepared.Files[i].IfExists)
			}
			require.Equal(t, []byte("public-content"), record.Prepared.Files[2].Content)
			info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path
			prepared, err := restoreConfigurationPlan(t.Context(), config, info)
			require.NoError(t, err)
			require.Equal(t, []byte(backend), prepared.Files[0].Content)
			require.Equal(t, []byte(provider), prepared.Files[1].Content)
			require.Equal(t, []byte("public-content"), prepared.Files[2].Content)
			require.Empty(t, prepared.Files[3].Content)
		})
	}
}

func TestConfigurationWorkdirDigestIncludesDownloadedModules(t *testing.T) {
	root := t.TempDir()
	module := filepath.Join(root, ".terraform", "modules", "child", "script.sh")
	require.NoError(t, os.MkdirAll(filepath.Dir(module), 0o755))
	require.NoError(t, os.WriteFile(module, []byte("before"), 0o600))
	before, err := configurationWorkdirDigest(root, "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(module, []byte("after"), 0o600))
	after, err := configurationWorkdirDigest(root, "")
	require.NoError(t, err)
	require.NotEqual(t, before, after)
}

func TestPlanOutputFlagsUseEffectiveArguments(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider []string
		args     []string
		conflict bool
		output   string
	}{
		{"double dash", nil, []string{"plan", "--out=new.plan"}, false, "new.plan"},
		{"provider output", []string{"--out=provider.plan"}, []string{"plan"}, false, "provider.plan"},
		{"provider and default conflict", []string{"--out=new.plan"}, []string{"plan", "-out=old.plan"}, true, ""},
		{"two separated flags conflict", nil, []string{"plan", "-out", "old.plan", "--out", "new.plan"}, true, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, info, execCtx, _, _ := planFixture(t)
			// A permissive CI exit mapping must not turn a preflight rejection into success.
			config.CI.Enabled = true
			config.Components.Terraform.CI.ExitCodes = map[int]bool{0: true, 1: true, 2: true}
			shell := &fakeShell{}
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.execute
			info.ComponentExecution = &schema.ComponentExecution{ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"plan"}, Arguments: test.provider}}, BeforeHooks: []schema.ComponentHook{execHook("before", "plan")}}
			if test.output != "" {
				require.NoError(t, os.WriteFile(filepath.Join(execCtx.componentPath, test.output), []byte("plan"), 0o600))
			}
			err := executor.runMainCommandPhase(config, info, execCtx, test.args, execCtx.componentPath, false)
			if test.conflict {
				require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
				require.Empty(t, shell.calls, "neither hook nor Terraform should run")
			} else {
				require.NoError(t, err)
				require.FileExists(t, filepath.Join(execCtx.componentPath, test.output)+configurationPlanSuffix)
				require.Len(t, shell.calls, 2)
			}
		})
	}
}

// --from-plan=<path> names the same file as the canonical location, so it must verify the same
// way even when the working directory is spelled relative and the path absolute.
func TestConfigurationPlanExplicitAbsolutePathVerifiesLikeCanonicalPath(t *testing.T) {
	config, info, execCtx, _, _ := planFixture(t)
	t.Chdir(config.BasePath)
	config.BasePath = "."
	relativeWorkdir := ConstructTerraformComponentWorkingDir(config, info)
	require.False(t, filepath.IsAbs(relativeWorkdir))
	execCtx.workingDir, execCtx.componentPath = relativeWorkdir, relativeWorkdir
	canonical := filepath.Join(relativeWorkdir, ConstructTerraformComponentPlanfileName(info))
	require.NoError(t, os.WriteFile(canonical, []byte("reviewed plan"), 0o600))
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out=" + filepath.Base(canonical)}))

	absolute, err := filepath.Abs(canonical)
	require.NoError(t, err)
	for name, planFile := range map[string]string{"canonical": "", "absolute": absolute, "relative": filepath.Base(canonical)} {
		t.Run(name, func(t *testing.T) {
			apply := *info
			apply.SubCommand, apply.UseTerraformPlan, apply.PlanFile = "apply", true, planFile
			_, err := restoreConfigurationPlan(t.Context(), config, &apply)
			require.NoError(t, err)
		})
	}
}

func TestConfigurationPlanMissingPlanExplainsWhatHappened(t *testing.T) {
	t.Run("never planned", func(t *testing.T) {
		config, info, _, _, path := planFixture(t)
		require.NoError(t, os.Remove(path))
		info.SubCommand, info.UseTerraformPlan = "apply", true
		_, err := restoreConfigurationPlan(t.Context(), config, info)
		require.ErrorIs(t, err, errUtils.ErrSavedPlanNotFound)
		require.NotErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
		require.Contains(t, errUtils.Format(err, errUtils.FormatterConfig{}), "no saved plan")
	})
	t.Run("explicit path that does not exist", func(t *testing.T) {
		config, info, _, _, path := planFixture(t)
		require.NoError(t, os.Remove(path))
		info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path
		_, err := restoreConfigurationPlan(t.Context(), config, info)
		require.ErrorIs(t, err, errUtils.ErrSavedPlanNotFound)
	})
	t.Run("plan file without an Atmos record", func(t *testing.T) {
		config, info, execCtx, _, path := planFixture(t)
		require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out=" + path}))
		require.NoError(t, os.Remove(path+configurationPlanSuffix))
		info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path
		_, err := restoreConfigurationPlan(t.Context(), config, info)
		require.ErrorIs(t, err, errUtils.ErrSavedPlanNotFound)
	})
}

func TestFinishConfigurationPlan(t *testing.T) {
	setup := func(t *testing.T, subcommand string) (*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string) {
		config, info, execCtx, _, _ := planFixture(t)
		canonical := ConstructTerraformComponentPlanfilePath(config, info)
		require.NoError(t, os.WriteFile(canonical, []byte("reviewed plan"), 0o600))
		require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out=" + canonical}))
		info.SubCommand = subcommand
		return config, info, canonical
	}
	for _, subcommand := range []string{"apply", "deploy", "destroy"} {
		t.Run(subcommand+" removes the consumed plan and its record", func(t *testing.T) {
			config, info, canonical := setup(t, subcommand)
			finishConfigurationPlan(config, info)
			require.NoFileExists(t, canonical)
			require.NoFileExists(t, canonical+configurationPlanSuffix)
		})
	}
	for _, subcommand := range []string{"plan", "output", "validate", "init", "show", "refresh", "state list"} {
		t.Run(subcommand+" keeps the reviewed plan", func(t *testing.T) {
			config, info, canonical := setup(t, subcommand)
			finishConfigurationPlan(config, info)
			require.FileExists(t, canonical)
			require.FileExists(t, canonical+configurationPlanSuffix)
		})
	}
	t.Run("an explicit plan path keeps the user's file and drops its record", func(t *testing.T) {
		config, info, canonical := setup(t, "apply")
		explicit := filepath.Join(filepath.Dir(canonical), "explicit.planfile")
		require.NoError(t, os.WriteFile(explicit, []byte("explicit"), 0o600))
		require.NoError(t, os.WriteFile(explicit+configurationPlanSuffix, []byte("{}"), 0o600))
		info.PlanFile = explicit
		finishConfigurationPlan(config, info)
		require.FileExists(t, explicit)
		require.NoFileExists(t, explicit+configurationPlanSuffix)
	})
	t.Run("an explicit plan path that names the canonical file keeps it and drops its record", func(t *testing.T) {
		config, info, canonical := setup(t, "apply")
		info.PlanFile = canonical
		finishConfigurationPlan(config, info)
		require.FileExists(t, canonical)
		require.NoFileExists(t, canonical+configurationPlanSuffix)
	})
	t.Run("a failed apply keeps the plan and its record even when its exit code is mapped to success", func(t *testing.T) {
		config, info, canonical := setup(t, "apply")
		info.ExecMetadataRawExitCode = 1
		finishConfigurationPlan(config, info)
		require.FileExists(t, canonical)
		require.FileExists(t, canonical+configurationPlanSuffix)
	})
	t.Run("a dry run and an ordinary component keep every file", func(t *testing.T) {
		config, info, canonical := setup(t, "apply")
		info.DryRun = true
		finishConfigurationPlan(config, info)
		info.DryRun = false
		info.ComponentSettingsSection = map[string]any{}
		finishConfigurationPlan(config, info)
		require.FileExists(t, canonical)
		require.FileExists(t, canonical+configurationPlanSuffix)
	})
}

// Inspection commands keep the saved plan of a plugin-managed and an ordinary component alike,
// while a completed destroy consumes it for both.
func TestCleanupTerraformFilesKeepsReviewedPlanForOwnedComponents(t *testing.T) {
	for _, tc := range []struct {
		subcommand string
		owned      bool
		kept       bool
	}{
		{"destroy", true, false},
		{"destroy", false, false},
		{"output", true, true},
		{"output", false, true},
	} {
		config, info, _, _, _ := planFixture(t)
		if !tc.owned {
			info.ComponentSettingsSection = map[string]any{}
		}
		canonical := ConstructTerraformComponentPlanfilePath(config, info)
		require.NoError(t, os.WriteFile(canonical, []byte("plan"), 0o600))
		info.SubCommand = tc.subcommand
		cleanupTerraformFiles(config, info)
		if tc.kept {
			require.FileExists(t, canonical, "%s owned=%t", tc.subcommand, tc.owned)
		} else {
			require.NoFileExists(t, canonical, "%s owned=%t", tc.subcommand, tc.owned)
		}
	}
}

// The same directory reached through a symlink (/var and /private/var on macOS) must not make the
// plan file look like part of the reviewed working directory.
func TestConfigurationWorkdirDigestIgnoresPlanReachedThroughSymlink(t *testing.T) {
	root := t.TempDir()
	workdir := filepath.Join(root, "real")
	require.NoError(t, os.Mkdir(workdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "main.tf"), []byte("module"), 0o600))
	plan := filepath.Join(workdir, "reviewed.planfile")
	require.NoError(t, os.WriteFile(plan, []byte("plan"), 0o600))
	link := filepath.Join(root, "link")
	if err := os.Symlink(workdir, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	direct, err := configurationWorkdirDigest(workdir, plan)
	require.NoError(t, err)
	viaLink, err := configurationWorkdirDigest(workdir, filepath.Join(link, "reviewed.planfile"))
	require.NoError(t, err)
	require.Equal(t, direct, viaLink)
}

func TestRecoveredPlanBindsEffectiveOutput(t *testing.T) {
	config, info, execCtx, _, _ := planFixture(t)
	config.Components.Terraform.Init.Reconfigure = schema.TerraformInitReconfigureAuto
	plan := filepath.Join(execCtx.componentPath, "recovered.plan")
	require.NoError(t, os.WriteFile(plan, []byte("reviewed"), 0o600))
	info.ComponentExecution = &schema.ComponentExecution{ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"plan"}, Arguments: []string{"--out=" + plan}}}}
	attempts := 0
	shell := &fakeShell{run: func(call *shellCall) error {
		attempts++
		if attempts == 1 {
			return errors.New("Backend configuration changed")
		}
		return nil
	}}
	executor := testExecutor(t)
	executor.services.ExecuteShellCommand = shell.execute
	executor.executeTerraformInitForcedFn = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string, bool, bool, ...ShellCommandOption) error {
		return nil
	}
	require.NoError(t, executor.runMainCommandPhase(config, info, execCtx, []string{"plan"}, execCtx.componentPath, false))
	require.Equal(t, 2, attempts)
	require.FileExists(t, plan+configurationPlanSuffix)
}

type planShowRunner struct{}

//nolint:gocritic // The process.Runner interface fixes the value parameter.
func (planShowRunner) Run(_ context.Context, spec process.TaskSpec) process.Result {
	_, _ = io.WriteString(spec.Streams.Stdout, `{"format_version":"1.2","planned_values":{}}`)
	return process.Result{Started: true}
}

func TestPolicyPlanBindsPublishedOutput(t *testing.T) {
	config, info, execCtx, _, _ := planFixture(t)
	var engine policy.Engine
	require.NoError(t, engine.Register(policy.CELEngine, &policy.CEL{}))
	execCtx.planPolicy = &planfile.Guard{Engine: &engine, Show: planShowRunner{}, Rules: []policy.Rule{{ID: "allow", On: policy.TerraformPlan, Engine: policy.CELEngine, Expression: "false", Effects: policy.Effects{Deny: true}}}}
	shell := &fakeShell{run: func(call *shellCall) error {
		path := plannedOutputPath(execCtx.componentPath, call.args)
		return os.WriteFile(path, []byte("reviewed"), 0o600)
	}}
	executor := testExecutor(t)
	executor.services.ExecuteShellCommand = shell.execute
	path := filepath.Join(execCtx.componentPath, "approved.plan")
	require.NoError(t, executor.runMainCommandPhase(config, info, execCtx, []string{"plan", "-out=" + path}, execCtx.componentPath, false))
	require.FileExists(t, path+configurationPlanSuffix)
}
