package terraform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/planfile"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_plan_runtime_test.go -package=$GOPACKAGE -mock_names=reviewedPlanRuntime=MockReviewedPlanRuntime,planIdentityRuntime=MockPlanIdentityRuntime,planShowRunner=MockPlanShowRunner

// reviewedPlanRuntime is a configuration runtime that prepares, binds saved plans, and resolves state.
type reviewedPlanRuntime interface {
	schema.ComponentConfigurationRuntime
	schema.ComponentConfigurationPlanRuntime
	schema.ComponentConfigurationStateRuntime
}

// planIdentityRuntime can bind plans but cannot resolve backend state.
type planIdentityRuntime interface {
	schema.ComponentConfigurationRuntime
	schema.ComponentConfigurationPlanRuntime
}

// planShowRunner runs the plan show command for plan policy evaluation.
type planShowRunner interface {
	process.Runner
}

// The generated mocks implement the contracts that the code under test asserts.
var (
	_ reviewedPlanRuntime = (*MockReviewedPlanRuntime)(nil)
	_ planIdentityRuntime = (*MockPlanIdentityRuntime)(nil)
	_ planShowRunner      = (*MockPlanShowRunner)(nil)
)

// planRuntimeFixture holds the responses that the generated plan runtime mock returns.
// Tests change its fields to simulate a different provider identity, backend, or failure.
type planRuntimeFixture struct {
	identity string
	state    *schema.ComponentConfigurationState
	err      error
}

// mock returns a generated plan runtime that answers PlanIdentity and ResolveState from the
// fixture at call time. Discover and Prepare have no expectations, so gomock rejects them.
func (p *planRuntimeFixture) mock(t *testing.T) *MockReviewedPlanRuntime {
	t.Helper()
	runtime := NewMockReviewedPlanRuntime(gomock.NewController(t))
	runtime.EXPECT().PlanIdentity(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *schema.ConfigAndStacksInfo, []schema.ComponentConfigurationSource) (string, error) {
			return p.identity, p.err
		},
	).AnyTimes()
	runtime.EXPECT().ResolveState(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
			return p.state, p.err
		},
	).AnyTimes()
	return runtime
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
	config.ComponentConfigurations = runtime.mock(t)
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
	config.ComponentConfigurations = runtime.mock(t)
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
	err := writePlanBinding(filepath.Join(t.TempDir(), "absent", "binding"), nil)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = fileDigest(t.TempDir())
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	_, err = fileDigest(filepath.Join(t.TempDir(), "absent"))
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.ErrorIs(t, err, os.ErrNotExist)
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
	config, info, execCtx, runtime, path := planFixture(t)
	execCtx.preparedConfiguration.Inputs = map[string]any{"password": "input-secret-value"}
	execCtx.preparedConfiguration.Backend = map[string]any{"access_key": "backend-secret-value"}
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

func TestConfigurationPlanRestoreThenFinish(t *testing.T) {
	for _, subcommand := range []string{"apply", "deploy"} {
		for _, selection := range []string{"implicit", "explicit absolute", "explicit relative"} {
			t.Run(subcommand+"/"+selection, func(t *testing.T) {
				config, info, execCtx, _, _ := planFixture(t)
				canonical := ConstructTerraformComponentPlanfilePath(config, info)
				require.NoError(t, os.WriteFile(canonical, []byte("reviewed plan"), 0o600))
				require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out=" + canonical}))
				info.SubCommand, info.UseTerraformPlan = subcommand, true
				switch selection {
				case "explicit absolute":
					info.PlanFile = canonical
				case "explicit relative":
					info.PlanFile = filepath.Base(canonical)
				}
				_, err := restoreConfigurationPlan(t.Context(), config, info)
				require.NoError(t, err)
				require.Equal(t, canonical, info.PlanFile)
				// Failed applies and dry runs preserve both files for another attempt.
				info.ExecMetadataRawExitCode = 1
				finishConfigurationPlan(config, info)
				require.FileExists(t, canonical)
				require.FileExists(t, canonical+configurationPlanSuffix)
				info.ExecMetadataRawExitCode, info.DryRun = 0, true
				finishConfigurationPlan(config, info)
				require.FileExists(t, canonical)
				require.FileExists(t, canonical+configurationPlanSuffix)
				info.DryRun = false
				finishConfigurationPlan(config, info)
				if selection == "implicit" {
					require.NoFileExists(t, canonical)
				} else {
					require.FileExists(t, canonical)
				}
				require.NoFileExists(t, canonical+configurationPlanSuffix)
			})
		}
	}
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

// TestDetailedExitCodePlanRecordsReviewedPlan binds a plan that reports changes with status 2,
// and leaves an ordinary failure unbound.
func TestDetailedExitCodePlanRecordsReviewedPlan(t *testing.T) {
	for _, test := range []struct {
		name   string
		args   []string
		code   int
		record bool
	}{
		{name: "explicit detailed exit code with changes", args: []string{"plan", "-detailed-exitcode", "-out", "changes.plan"}, code: 2, record: true},
		{name: "failure with detailed exit code", args: []string{"plan", "-detailed-exitcode", "-out", "changes.plan"}, code: 1},
		{name: "status 2 without detailed exit code", args: []string{"plan", "-out", "changes.plan"}, code: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, info, execCtx, _, _ := planFixture(t)
			shell := &fakeShell{run: func(call *shellCall) error {
				if err := os.WriteFile(plannedOutputPath(execCtx.componentPath, call.args), []byte("changes"), 0o600); err != nil {
					return err
				}
				return errUtils.ExitCodeError{Code: test.code}
			}}
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.execute
			err := executor.runMainCommandPhase(config, info, execCtx, test.args, execCtx.componentPath, false)
			require.Equal(t, test.code, resolveExitCode(err), "the plan status is returned unchanged")
			binding := filepath.Join(execCtx.componentPath, "changes.plan") + configurationPlanSuffix
			if test.record {
				require.FileExists(t, binding)
				return
			}
			require.NoFileExists(t, binding)
		})
	}
}

// An ignored failure of the main command ends the run successfully, without a plan to bind. A plan
// that completed is still bound when an errors policy is present but does not match.
func TestIgnoredPlanFailureDoesNotRecordAPlanBinding(t *testing.T) {
	for _, test := range []struct {
		name   string
		ignore string
	}{
		{name: "ignored failure", ignore: "backend unavailable"},
		{name: "unmatched policy", ignore: "never matches"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, info, execCtx, _, _ := planFixture(t)
			info.ComponentExecution = &schema.ComponentExecution{Errors: &schema.ComponentErrors{
				Ignore: []schema.ComponentIgnorePolicy{{Name: "ignored", Patterns: []string{test.ignore}}},
			}}
			// The failed plan writes no artifact.
			shell := &fakeShell{run: func(call *shellCall) error {
				if call.options.StderrCapture != nil {
					_, _ = io.WriteString(call.options.StderrCapture, "Error: backend unavailable")
				}
				return errUtils.ExitCodeError{Code: 1}
			}}
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.execute
			path := filepath.Join(execCtx.componentPath, "failed.plan")

			err := executor.runMainCommandPhase(config, info, execCtx, []string{"plan", "-out=" + path}, execCtx.componentPath, false)

			if test.ignore == "backend unavailable" {
				require.NoError(t, err, "the ignored failure completes the run")
			} else {
				require.Error(t, err, "a failure that no policy ignores still fails")
			}
			require.NoFileExists(t, path+configurationPlanSuffix)
		})
	}
}

// newPlanShowRunner returns a generated runner whose plan show prints an empty JSON plan.
func newPlanShowRunner(t *testing.T) process.Runner {
	t.Helper()
	runner := NewMockPlanShowRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, spec process.TaskSpec) process.Result {
			_, _ = io.WriteString(spec.Streams.Stdout, `{"format_version":"1.2","planned_values":{}}`)
			return process.Result{Started: true}
		},
	).AnyTimes()
	return runner
}

func TestPolicyPlanBindsPublishedOutput(t *testing.T) {
	config, info, execCtx, _, _ := planFixture(t)
	var engine policy.Engine
	require.NoError(t, engine.Register(policy.CELEngine, &policy.CEL{}))
	execCtx.planPolicy = &planfile.Guard{Engine: &engine, Show: newPlanShowRunner(t), Rules: []policy.Rule{{ID: "allow", On: policy.TerraformPlan, Engine: policy.CELEngine, Expression: "false", Effects: policy.Effects{Deny: true}}}}
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

func TestDetailedExitCodePlanBinding(t *testing.T) {
	for _, test := range []struct {
		name                                                    string
		hooks, recoverInit, hookFailure, bindingFailure, noFlag bool
		code                                                    int
	}{
		{name: "plain plan", code: 2},
		{name: "hooks complete", hooks: true, code: 2},
		{name: "init recovery", hooks: true, recoverInit: true, code: 2},
		{name: "after hook fails with status two", hooks: true, hookFailure: true, code: 2},
		{name: "binding fails", hooks: true, bindingFailure: true, code: 2},
		{name: "plan fails", hooks: true, code: 1},
		{name: "status two without flag", hooks: true, noFlag: true, code: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, info, execCtx, runtime, path := planFixture(t)
			config.Components.Terraform.Init.Reconfigure = schema.TerraformInitReconfigureAuto
			args := []string{"plan", "-out=" + path}
			if !test.noFlag {
				args = append(args, "-detailed-exitcode")
			}
			if test.hooks {
				info.ComponentExecution = &schema.ComponentExecution{AfterHooks: []schema.ComponentHook{execHook("after", "plan")}}
			}
			bindingErr := errors.New("binding identity unavailable")
			if test.bindingFailure {
				runtime.err = bindingErr
			}
			attempts, initCalls, hookCalls := 0, 0, 0
			shell := &fakeShell{run: func(call *shellCall) error {
				if call.command == "hook-after" {
					hookCalls++
					if test.hookFailure {
						return exitCodeError(2)
					}
					return nil
				}
				attempts++
				if test.recoverInit && attempts == 1 {
					return errors.New("Backend configuration changed")
				}
				return exitCodeError(test.code)
			}}
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.execute
			executor.executeTerraformInitForcedFn = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string, bool, bool, ...ShellCommandOption) error {
				initCalls++
				return nil
			}
			err := executor.runMainCommandPhase(config, info, execCtx, args, execCtx.componentPath, false)
			require.Error(t, err)
			if test.bindingFailure {
				require.ErrorIs(t, err, bindingErr)
			} else {
				require.Equal(t, test.code, errUtils.GetExitCode(err))
			}
			if test.recoverInit {
				require.Equal(t, 2, attempts)
				require.Equal(t, 1, initCalls)
			} else {
				require.Equal(t, 1, attempts)
				require.Zero(t, initCalls)
			}
			if test.hooks && test.code == 2 && !test.noFlag {
				require.Equal(t, 1, hookCalls)
			} else {
				require.Zero(t, hookCalls)
			}
			if test.hookFailure || test.bindingFailure || test.noFlag || test.code != 2 {
				require.NoFileExists(t, path+configurationPlanSuffix)
				return
			}
			require.FileExists(t, path+configurationPlanSuffix)
			info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path
			_, err = restoreConfigurationPlan(t.Context(), config, info)
			require.NoError(t, err, "the status-two plan must be usable by --from-plan")
		})
	}
}

// TestPlanOutputFlagsInEnvironmentAreRejected covers output flags that Terraform
// reads from TF_CLI_ARGS or TF_CLI_ARGS_plan, which Atmos cannot bind.
func TestPlanOutputFlagsInEnvironmentAreRejected(t *testing.T) {
	for _, test := range []struct {
		name      string
		inherited map[string]string
		component []string
		provider  map[string]string
		global    map[string]string
		sanitized []string
		commands  []string
		reject    bool
	}{
		{name: "inherited TF_CLI_ARGS_plan", inherited: map[string]string{"TF_CLI_ARGS_plan": "-out=env.plan"}, reject: true},
		{name: "inherited TF_CLI_ARGS", inherited: map[string]string{"TF_CLI_ARGS": "-lock=false --out env.plan"}, reject: true},
		{name: "component environment", component: []string{"TF_CLI_ARGS_plan=-out env.plan"}, reject: true},
		{name: "provider environment", provider: map[string]string{"TF_CLI_ARGS_plan": "--out=env.plan"}, commands: []string{"plan"}, reject: true},
		{name: "bare flag", component: []string{"TF_CLI_ARGS=-out"}, reject: true},
		{name: "other flags", inherited: map[string]string{"TF_CLI_ARGS_plan": "-lock=false -outfile-like=x"}, component: []string{"TF_CLI_ARGS=-no-color"}},
		{name: "apply only variable", component: []string{"TF_CLI_ARGS_apply=-out=x"}},
		{name: "provider environment for apply", provider: map[string]string{"TF_CLI_ARGS_plan": "-out=env.plan"}, commands: []string{"apply"}},
		{name: "global environment", global: map[string]string{"TF_CLI_ARGS_plan": "-out=env.plan"}, reject: true},
		{name: "sanitized environment omits inherited flag", inherited: map[string]string{"TF_CLI_ARGS_plan": "-out=env.plan"}, sanitized: []string{"PATH=/bin"}},
		{name: "sanitized environment carries flag", sanitized: []string{"TF_CLI_ARGS=-out=env.plan"}, reject: true},
		{name: "component overrides inherited flag", inherited: map[string]string{"TF_CLI_ARGS_plan": "-out=env.plan"}, component: []string{"TF_CLI_ARGS_plan=-lock=false"}},
		{name: "component overrides global flag", global: map[string]string{"TF_CLI_ARGS": "-out=env.plan"}, component: []string{"TF_CLI_ARGS="}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TF_CLI_ARGS", "")
			t.Setenv("TF_CLI_ARGS_plan", "")
			for name, value := range test.inherited {
				t.Setenv(name, value)
			}
			config, info, execCtx, _, path := planFixture(t)
			config.Env = test.global
			info.SanitizedEnv = test.sanitized
			info.ComponentEnvList = test.component
			if test.provider != nil {
				info.ComponentExecution = &schema.ComponentExecution{ExtraArguments: []schema.ComponentExtraArguments{{Commands: test.commands, Environment: test.provider}}, BeforeHooks: []schema.ComponentHook{execHook("before", "plan")}}
			}
			shell := &fakeShell{}
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.execute
			err := executor.runMainCommandPhase(config, info, execCtx, []string{"plan", "-out", filepath.Base(path)}, execCtx.componentPath, false)
			if test.reject {
				require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
				require.Empty(t, shell.calls, "neither hook nor Terraform should run")
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, shell.calls)
			require.FileExists(t, path+configurationPlanSuffix)
		})
	}
}

func TestDetailedExitCodeBindingUsesSubprocessEnvironment(t *testing.T) {
	t.Setenv("TF_CLI_ARGS", "")
	for _, test := range []struct {
		name, ambient            string
		processEnv, componentEnv []string
		success                  bool
	}{
		{name: "inherit ambient", ambient: "-detailed-exitcode", success: true},
		{name: "replacement enables", processEnv: []string{"TF_CLI_ARGS_plan=-detailed-exitcode"}, success: true},
		{name: "empty replacement excludes ambient", ambient: "-detailed-exitcode", processEnv: []string{}},
		{name: "replacement excludes ambient", ambient: "-detailed-exitcode", processEnv: []string{"OTHER=value"}},
		{name: "component enables", processEnv: []string{}, componentEnv: []string{"TF_CLI_ARGS_plan=-detailed-exitcode"}, success: true},
		{name: "component disables", processEnv: []string{"TF_CLI_ARGS_plan=-detailed-exitcode"}, componentEnv: []string{"TF_CLI_ARGS_plan="}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TF_CLI_ARGS_plan", test.ambient)
			config, info, execCtx, _, path := planFixture(t)
			info.ComponentExecution = detailedExitCodeInfo().ComponentExecution
			info.ComponentEnvList = test.componentEnv
			shell := &fakeShell{run: func(call *shellCall) error {
				if call.command == info.Command {
					return exitCodeError(2)
				}
				return nil
			}}
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.execute
			err := executor.runMainCommandPhase(config, info, execCtx, []string{"plan", "-out=" + path}, execCtx.componentPath, false, component.WithEnvironment(test.processEnv))
			require.Equal(t, 2, errUtils.GetExitCode(err))
			require.Len(t, shell.calls, 2)
			if test.success {
				require.Equal(t, "hook-after", shell.calls[1].command)
				require.FileExists(t, path+configurationPlanSuffix)
			} else {
				require.Equal(t, "hook-alert", shell.calls[1].command)
				require.NoFileExists(t, path+configurationPlanSuffix)
			}
		})
	}
}

func TestDetailedExitCodePolicyPlanBinding(t *testing.T) {
	for _, deny := range []bool{false, true} {
		t.Run(fmt.Sprintf("deny=%t", deny), func(t *testing.T) {
			config, info, execCtx, _, path := planFixture(t)
			var engine policy.Engine
			require.NoError(t, engine.Register(policy.CELEngine, &policy.CEL{}))
			execCtx.planPolicy = &planfile.Guard{Engine: &engine, Show: newPlanShowRunner(t), Rules: []policy.Rule{{ID: "guard", On: policy.TerraformPlan, Engine: policy.CELEngine, Expression: fmt.Sprint(deny), Effects: policy.Effects{Deny: true}}}}
			require.NoError(t, os.Remove(path))
			shell := &fakeShell{run: func(call *shellCall) error {
				require.NoError(t, os.WriteFile(plannedOutputPath(execCtx.componentPath, call.args), []byte("reviewed"), 0o600))
				return exitCodeError(2)
			}}
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.execute
			err := executor.runMainCommandPhase(config, info, execCtx, []string{"plan", "-out=" + path, "-detailed-exitcode"}, execCtx.componentPath, false)
			if deny {
				require.Error(t, err)
				require.NotEqual(t, 2, errUtils.GetExitCode(err))
				require.NoFileExists(t, path)
				require.NoFileExists(t, path+configurationPlanSuffix)
			} else {
				require.Equal(t, 2, errUtils.GetExitCode(err))
				require.FileExists(t, path+configurationPlanSuffix)
			}
		})
	}
}

// TestEnvironmentPlanOutputIgnoresUnownedPlans keeps ordinary components unaffected.
func TestEnvironmentPlanOutputIgnoresUnownedPlans(t *testing.T) {
	t.Setenv("TF_CLI_ARGS_plan", "-out=env.plan")
	config, info, execCtx, _, _ := planFixture(t)
	require.ErrorIs(t, validateEnvironmentPlanOutput(config, info, execCtx), errUtils.ErrPlanVerificationFailed)
	info.DryRun = true
	require.NoError(t, validateEnvironmentPlanOutput(config, info, execCtx))
	info.DryRun = false
	info.SubCommand = "apply"
	require.NoError(t, validateEnvironmentPlanOutput(config, info, execCtx))
	info.SubCommand = "plan"
	execCtx.preparedConfiguration = nil
	require.NoError(t, validateEnvironmentPlanOutput(config, info, execCtx))
}
