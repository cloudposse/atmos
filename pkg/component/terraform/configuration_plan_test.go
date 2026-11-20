package terraform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/planfile"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
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

// observeFixturePlan simulates a successful plan subprocess for binding tests.
func observeFixturePlan(t *testing.T, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, execCtx *componentExecContext, path string) {
	t.Helper()
	snapshotter := &planSnapshotter{ctx: t.Context(), config: config, info: info, execCtx: execCtx, pathFrom: func([]string) string { return path }}
	// The subprocess must write the plan itself: a file already at the path is not bound.
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, snapshotter.around([]string{"plan", "-out=" + path}, func() error { return os.WriteFile(path, content, 0o600) }, failedOnError)())
	execCtx.planSnapshots = snapshotter
}

func TestConfigurationPlanRestoresReviewedPreparation(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	observeFixturePlan(t, config, info, execCtx, path)
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
			observeFixturePlan(t, config, info, execCtx, path)
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
	_, err = configurationWorkdirDigest(filepath.Join(t.TempDir(), "absent"), path, "")
	require.Error(t, err)
	require.True(t, usesConfigurationPlan(info))
	require.False(t, usesConfigurationPlan(&schema.ConfigAndStacksInfo{}))
}

func TestConfigurationWorkdirDigestIgnoresMutableFiles(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "reviewed.plan")
	first, err := configurationWorkdirDigest(root, plan, "")
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(root, ".terraform"), 0o700))
	for _, name := range []string{"terraform.tfstate", "reviewed.plan", "reviewed.plan.atmos.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("mutable"), 0o600))
	}
	second, err := configurationWorkdirDigest(root, plan, "")
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.tf"), []byte("source"), 0o600))
	third, err := configurationWorkdirDigest(root, plan, "")
	require.NoError(t, err)
	require.NotEqual(t, first, third)
	if err := os.Symlink(filepath.Join(root, "main.tf"), filepath.Join(root, "linked.tf")); err != nil {
		t.Skip("symlinks unavailable")
	}
	_, err = configurationWorkdirDigest(root, plan, "")
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
	observeFixturePlan(t, config, info, execCtx, path)
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
			for _, file := range execCtx.preparedConfiguration.Files[:3] {
				require.NoError(t, os.WriteFile(filepath.Join(execCtx.workingDir, file.Path), file.Content, 0o600))
			}
			observeFixturePlan(t, config, info, execCtx, path)
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
			require.Empty(t, record.Prepared.Files[2].Content, "public bytes are bound by digest, not stored")
			require.True(t, record.PublicFiles["public.txt"].Present)
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

// The working directory digest skips cache directories, so a sensitive generated file there is
// verified against its own recorded digest before apply restores it.
func TestConfigurationPlanRejectsChangedSensitiveFileOutsideTheWorkdirDigest(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	relative := filepath.Join(".terragrunt-cache", "unit", "backend.tf")
	reviewed := []byte(`terraform { backend "s3" { access_key = "reviewed-secret" } }`)
	execCtx.preparedConfiguration.Files = []schema.ComponentConfigurationFile{{Path: filepath.ToSlash(relative), Content: reviewed, IfExists: "overwrite", Sensitive: true}}
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(execCtx.workingDir, relative)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(execCtx.workingDir, relative), reviewed, 0o600))
	observeFixturePlan(t, config, info, execCtx, path)
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out=" + path}))
	info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path

	prepared, err := restoreConfigurationPlan(t.Context(), config, info)
	require.NoError(t, err, "the reviewed bytes are restored")
	require.Equal(t, reviewed, prepared.Files[0].Content)

	require.NoError(t, os.WriteFile(filepath.Join(execCtx.workingDir, relative), []byte(`terraform { backend "s3" { access_key = "tampered" } }`), 0o600))
	_, err = restoreConfigurationPlan(t.Context(), config, info)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.ErrorContains(t, err, "reviewed generated file changed")

	require.NoError(t, os.Remove(filepath.Join(execCtx.workingDir, relative)))
	_, err = restoreConfigurationPlan(t.Context(), config, info)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestConfigurationWorkdirDigestIncludesDownloadedModules(t *testing.T) {
	root := t.TempDir()
	module := filepath.Join(root, ".terraform", "modules", "child", "script.sh")
	require.NoError(t, os.MkdirAll(filepath.Dir(module), 0o755))
	require.NoError(t, os.WriteFile(module, []byte("before"), 0o600))
	before, err := configurationWorkdirDigest(root, "", "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(module, []byte("after"), 0o600))
	after, err := configurationWorkdirDigest(root, "", "")
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
			// The plan subprocess writes the output; a file that was already there is not bound.
			shell := newShellFixture(t, func(call *shellCall) error {
				if test.output == "" || call.command == "hook-before" {
					return nil
				}
				return os.WriteFile(filepath.Join(execCtx.componentPath, test.output), []byte("plan"), 0o600)
			})
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.mock.Execute
			info.ComponentExecution = &schema.ComponentExecution{ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"plan"}, Arguments: test.provider}}, BeforeHooks: []schema.ComponentHook{execHook("before", "plan")}}
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
	observeFixturePlan(t, config, info, execCtx, canonical)
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

func TestRelativeWorkingDirectoryPlanCanMoveToAnotherCheckout(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	info.ComponentSection = map[string]any{"metadata": map[string]any{"working_directory": filepath.Join("components", "network")}}
	observeFixturePlan(t, config, info, execCtx, path)
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out=" + path}))

	otherRoot := t.TempDir()
	otherWorkdir := filepath.Join(otherRoot, "components", "network")
	require.NoError(t, os.MkdirAll(otherWorkdir, 0o700))
	otherPlan := filepath.Join(otherWorkdir, filepath.Base(path))
	for _, suffix := range []string{"", configurationPlanSuffix} {
		data, err := os.ReadFile(path + suffix)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(otherPlan+suffix, data, 0o600))
	}
	config.BasePath = otherRoot
	info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, otherPlan
	_, rebuild, err := restoreOrRebuildConfigurationPlan(t.Context(), config, info)
	require.NoError(t, err)
	require.NotNil(t, rebuild)
	require.Equal(t, otherWorkdir, rebuild.workdir)
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
		observeFixturePlan(t, config, info, execCtx, path)
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
				observeFixturePlan(t, config, info, execCtx, canonical)
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
		observeFixturePlan(t, config, info, execCtx, canonical)
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
	direct, err := configurationWorkdirDigest(workdir, plan, "")
	require.NoError(t, err)
	viaLink, err := configurationWorkdirDigest(workdir, filepath.Join(link, "reviewed.planfile"), "")
	require.NoError(t, err)
	require.Equal(t, direct, viaLink)
}

func TestConfigurationWorkdirDigestIgnoresPlanArtifacts(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte("module"), 0o600))
	baseline, err := configurationWorkdirDigest(dir, filepath.Join(dir, "reviewed.planfile"), "")
	require.NoError(t, err)

	// Plans are reviewed artifacts, not configuration: an earlier plan, the stored plan that deploy
	// downloads, and the fresh plan it generates must not make the directory look edited.
	for _, name := range []string{"earlier.planfile", "stored.plan.tfplan", "new.plan"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("plan "+name), 0o600))
	}
	withPlans, err := configurationWorkdirDigest(dir, filepath.Join(dir, "reviewed.planfile"), "")
	require.NoError(t, err)
	require.Equal(t, baseline, withPlans)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.tf"), []byte("added"), 0o600))
	edited, err := configurationWorkdirDigest(dir, filepath.Join(dir, "reviewed.planfile"), "")
	require.NoError(t, err)
	require.NotEqual(t, baseline, edited)
}

func TestDropCIPlanLocationKeepsEveryPlanTheUserNamed(t *testing.T) {
	tests := []struct {
		name     string
		info     schema.ConfigAndStacksInfo
		wantPlan string
	}{
		{"hook recorded the location while downloading a stored plan", schema.ConfigAndStacksInfo{SubCommand: "deploy", PlanFile: "w/plan", StoredPlanFile: "w/stored"}, ""},
		{"explicit apply plan alongside stored path", schema.ConfigAndStacksInfo{SubCommand: "apply", PlanFile: "w/plan", StoredPlanFile: "w/stored"}, "w/plan"},
		{"a plan the user named", schema.ConfigAndStacksInfo{PlanFile: "w/plan", UseTerraformPlan: true, StoredPlanFile: "w/stored"}, "w/plan"},
		{"a plan named without any stored plan", schema.ConfigAndStacksInfo{PlanFile: "w/plan"}, "w/plan"},
		{"nothing named", schema.ConfigAndStacksInfo{}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dropCIPlanLocation(&test.info)
			require.Equal(t, test.wantPlan, test.info.PlanFile)
		})
	}
}

func TestRecoveredPlanBindsEffectiveOutput(t *testing.T) {
	config, info, execCtx, _, _ := planFixture(t)
	config.Components.Terraform.Init.Reconfigure = schema.TerraformInitReconfigureAuto
	plan := filepath.Join(execCtx.componentPath, "recovered.plan")
	info.ComponentExecution = &schema.ComponentExecution{ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"plan"}, Arguments: []string{"--out=" + plan}}}}
	attempts := 0
	shell := newShellFixture(t, func(call *shellCall) error {
		attempts++
		if attempts == 1 {
			return errors.New("Backend configuration changed")
		}
		return os.WriteFile(plan, []byte("reviewed"), 0o600)
	})
	executor := testExecutor(t)
	executor.services.ExecuteShellCommand = shell.mock.Execute
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
		extra  bool
		args   []string
		code   int
		record bool
		hook   bool
	}{
		{name: "explicit detailed exit code with changes", args: []string{"plan", "-detailed-exitcode", "-out", "changes.plan"}, code: 2, record: true},
		{name: "provider detailed exit code", extra: true, args: []string{"plan", "-out", "changes.plan"}, code: 2, record: true},
		{name: "failure with detailed exit code", args: []string{"plan", "-detailed-exitcode", "-out", "changes.plan"}, code: 1},
		{name: "status 2 without detailed exit code", args: []string{"plan", "-out", "changes.plan"}, code: 2},
		{name: "after hook failure with status 2", args: []string{"plan", "-detailed-exitcode", "-out", "changes.plan"}, code: 2, hook: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, info, execCtx, _, _ := planFixture(t)
			if test.extra {
				info.ComponentExecution = &schema.ComponentExecution{ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"plan"}, Arguments: []string{"-detailed-exitcode"}}}}
			}
			shell := newShellFixture(t, func(call *shellCall) error {
				if err := os.WriteFile(plannedOutputPath(execCtx.componentPath, call.args), []byte("changes"), 0o600); err != nil {
					return err
				}
				if test.hook {
					// A hook that exits 2 after Terraform wrote the plan is a failure, not "changes present".
					return fmt.Errorf("%w: after %q: %w", errUtils.ErrComponentHookFailed, "notify", errUtils.ExitCodeError{Code: test.code})
				}
				return errUtils.ExitCodeError{Code: test.code}
			})
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.mock.Execute
			err := executor.runMainCommandPhase(config, info, execCtx, test.args, execCtx.componentPath, false)
			require.Equal(t, test.code, resolveExitCode(err), "the plan status is returned unchanged")
			if test.hook {
				require.ErrorIs(t, err, errUtils.ErrComponentHookFailed)
			}
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
			shell := newShellFixture(t, func(call *shellCall) error {
				if call.options.StderrCapture != nil {
					_, _ = io.WriteString(call.options.StderrCapture, "Error: backend unavailable")
				}
				return errUtils.ExitCodeError{Code: 1}
			})
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.mock.Execute
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

// TestPlanBindingIgnoresChangesAfterTerraformRan keeps the binding on the configuration Terraform
// read. An after hook runs once Terraform has written the plan, so a hook that rewrites the
// module must leave the binding on the original bytes and make the later apply fail.
func TestPlanBindingIgnoresChangesAfterTerraformRan(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	info.ComponentExecution = &schema.ComponentExecution{AfterHooks: []schema.ComponentHook{execHook("after", "plan")}}
	shell := newShellFixture(t, func(call *shellCall) error {
		if call.command != info.Command {
			return os.WriteFile(filepath.Join(execCtx.workingDir, "main.tf"), []byte("rewritten by the hook"), 0o600)
		}
		return os.WriteFile(plannedOutputPath(execCtx.componentPath, call.args), []byte("reviewed plan"), 0o600)
	})
	executor := testExecutor(t)
	executor.services.ExecuteShellCommand = shell.mock.Execute
	require.NoError(t, executor.runMainCommandPhase(config, info, execCtx, []string{"plan", "-out=" + path}, execCtx.componentPath, false))
	require.Len(t, shell.calls, 2, "Terraform and the after hook both ran")
	require.FileExists(t, path+configurationPlanSuffix)

	info.SubCommand = "apply"
	info.UseTerraformPlan = true
	info.PlanFile = path
	_, err := restoreConfigurationPlan(t.Context(), config, info)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
}

// TestPlanBindingRejectsConfigurationChangedWhileTerraformRan covers a change made during the
// Terraform process itself, which leaves no way to tell which bytes the plan came from.
func TestPlanBindingRejectsConfigurationChangedWhileTerraformRan(t *testing.T) {
	for _, change := range []string{"module", "provider identity"} {
		t.Run(change, func(t *testing.T) {
			config, info, execCtx, runtime, path := planFixture(t)
			shell := newShellFixture(t, func(call *shellCall) error {
				switch change {
				case "module":
					if err := os.WriteFile(filepath.Join(execCtx.workingDir, "main.tf"), []byte("changed during plan"), 0o600); err != nil {
						return err
					}
				case "provider identity":
					runtime.identity = "changed during plan"
				}
				return os.WriteFile(plannedOutputPath(execCtx.componentPath, call.args), []byte("reviewed plan"), 0o600)
			})
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.mock.Execute
			err := executor.runMainCommandPhase(config, info, execCtx, []string{"plan", "-out=" + path}, execCtx.componentPath, false)
			require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
			require.NoFileExists(t, path+configurationPlanSuffix)
		})
	}
}

// TestPlanBindingAllowsFilesTerraformWrites keeps the plan and state out of the comparison,
// so an ordinary run that writes them is not rejected.
func TestPlanBindingAllowsFilesTerraformWrites(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	shell := newShellFixture(t, func(call *shellCall) error {
		if err := os.WriteFile(filepath.Join(execCtx.workingDir, "terraform.tfstate"), []byte("state"), 0o600); err != nil {
			return err
		}
		return os.WriteFile(plannedOutputPath(execCtx.componentPath, call.args), []byte("new plan bytes"), 0o600)
	})
	executor := testExecutor(t)
	executor.services.ExecuteShellCommand = shell.mock.Execute
	require.NoError(t, executor.runMainCommandPhase(config, info, execCtx, []string{"plan", "-out=" + path}, execCtx.componentPath, false))
	require.FileExists(t, path+configurationPlanSuffix)
}

func TestPolicyPlanBindsPublishedOutput(t *testing.T) {
	config, info, execCtx, _, _ := planFixture(t)
	var engine policy.Engine
	require.NoError(t, engine.Register(policy.CELEngine, &policy.CEL{}))
	execCtx.planPolicy = &planfile.Guard{Engine: &engine, Show: newPlanShowRunner(t), Rules: []policy.Rule{{ID: "allow", On: policy.TerraformPlan, Engine: policy.CELEngine, Expression: "false", Effects: policy.Effects{Deny: true}}}}
	shell := newShellFixture(t, func(call *shellCall) error {
		path := plannedOutputPath(execCtx.componentPath, call.args)
		return os.WriteFile(path, []byte("reviewed"), 0o600)
	})
	executor := testExecutor(t)
	executor.services.ExecuteShellCommand = shell.mock.Execute
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
			shell := newShellFixture(t, func(call *shellCall) error {
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
				if err := os.WriteFile(path, []byte("reviewed plan"), 0o600); err != nil {
					return err
				}
				return exitCodeError(test.code)
			})
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.mock.Execute
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

func TestRestoreRefusesToApplyTheStoredPlanItself(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{SubCommand: "deploy", PlanFile: "w/stored", StoredPlanFile: "w/stored"}
	require.False(t, restoresConfigurationPlan(info), "the plan deploy compares against is never what gets applied")
	info.PlanFile = "w/new.plan"
	require.True(t, restoresConfigurationPlan(info), "the fresh plan that matched it is")
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
			shell := newShellFixture(t, func(call *shellCall) error {
				if call.command != info.Command {
					return nil
				}
				return os.WriteFile(path, []byte("reviewed plan"), 0o600)
			})
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.mock.Execute
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
			shell := newShellFixture(t, func(call *shellCall) error {
				if call.command == info.Command {
					if err := os.WriteFile(path, []byte("reviewed plan"), 0o600); err != nil {
						return err
					}
					return exitCodeError(2)
				}
				return nil
			})
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.mock.Execute
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
			shell := newShellFixture(t, func(call *shellCall) error {
				require.NoError(t, os.WriteFile(plannedOutputPath(execCtx.componentPath, call.args), []byte("reviewed"), 0o600))
				return exitCodeError(2)
			})
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.mock.Execute
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

// TestRejectEnvironmentPlanOutputNameCase matches the variable names case-insensitively on Windows only.
func TestRejectEnvironmentPlanOutputNameCase(t *testing.T) {
	for _, test := range []struct {
		name    string
		goos    string
		entries []string
		reject  bool
	}{
		{name: "windows mixed-case plan variable", goos: "windows", entries: []string{"tf_cli_args_plan=-out=env.plan"}, reject: true},
		{name: "windows mixed-case shared variable", goos: "windows", entries: []string{"Tf_Cli_Args=--out env.plan"}, reject: true},
		{name: "windows later case variant clears the flag", goos: "windows", entries: []string{"TF_CLI_ARGS_plan=-out=env.plan", "tf_cli_args_PLAN=-lock=false"}},
		{name: "windows later case variant sets the flag", goos: "windows", entries: []string{"TF_CLI_ARGS_plan=-lock=false", "tf_cli_args_plan=-out=env.plan"}, reject: true},
		{name: "windows earlier case variant is overridden", goos: "windows", entries: []string{"tf_cli_args=-out=env.plan", "TF_CLI_ARGS="}},
		{name: "linux keeps names exact", goos: "linux", entries: []string{"tf_cli_args_plan=-out=env.plan", "Tf_Cli_Args=-out"}},
		{name: "linux exact name is rejected", goos: "linux", entries: []string{"TF_CLI_ARGS_plan=-out=env.plan"}, reject: true},
		{name: "linux later exact name overrides", goos: "linux", entries: []string{"TF_CLI_ARGS_plan=-out=env.plan", "TF_CLI_ARGS_plan=-lock=false"}},
		{name: "tab separates tokens", goos: "linux", entries: []string{"TF_CLI_ARGS_plan=-lock=false\t-out=hidden.plan"}, reject: true},
		{name: "newline separates tokens", goos: "linux", entries: []string{"TF_CLI_ARGS=-lock=false\n--out hidden.plan"}, reject: true},
		{name: "quoted flag", goos: "linux", entries: []string{"TF_CLI_ARGS_plan=-lock=false \"-out=hidden.plan\""}, reject: true},
		{name: "quoted tab stays inside one token", goos: "linux", entries: []string{"TF_CLI_ARGS_plan=-var 'a\t-out=x'"}},
		{name: "unterminated quote fails to parse", goos: "linux", entries: []string{"TF_CLI_ARGS_plan=-var \"a -lock=false"}, reject: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := rejectEnvironmentPlanOutput(test.entries, test.goos)
			if test.reject {
				require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
				return
			}
			require.NoError(t, err)
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

// The plan output check inspects the process environment the subprocess receives: an environment
// chosen for the execution replaces the sanitized one, including when it is empty.
func TestEnvironmentPlanOutputUsesExecutionEnvironment(t *testing.T) {
	t.Setenv("TF_CLI_ARGS_plan", "")
	t.Setenv("TF_CLI_ARGS", "")
	config, info, execCtx, _, _ := planFixture(t)
	flag := "TF_CLI_ARGS_plan=-out=env.plan"

	info.SanitizedEnv = []string{"OTHER=value"}
	require.NoError(t, validateEnvironmentPlanOutput(config, info, execCtx))
	require.ErrorIs(t, validateEnvironmentPlanOutput(config, info, execCtx, component.WithEnvironment([]string{flag})), errUtils.ErrPlanVerificationFailed)

	info.SanitizedEnv = []string{flag}
	require.ErrorIs(t, validateEnvironmentPlanOutput(config, info, execCtx), errUtils.ErrPlanVerificationFailed)
	require.NoError(t, validateEnvironmentPlanOutput(config, info, execCtx, component.WithEnvironment([]string{})), "an empty execution environment replaces the sanitized one")
	require.NoError(t, validateEnvironmentPlanOutput(config, info, execCtx, component.WithEnvironment([]string{"OTHER=value"})))
	require.ErrorIs(t, validateEnvironmentPlanOutput(config, info, execCtx, component.WithEnvironment(nil)), errUtils.ErrPlanVerificationFailed, "a nil environment keeps the sanitized one")

	info.SanitizedEnv = nil
	info.ComponentEnvList = []string{flag}
	require.ErrorIs(t, validateEnvironmentPlanOutput(config, info, execCtx, component.WithEnvironment([]string{})), errUtils.ErrPlanVerificationFailed, "component layers still apply")
}

func TestConfigurationPlanRoundTripNumericBackend(t *testing.T) {
	config, info, execCtx, fixture, path := planFixture(t)
	execCtx.preparedConfiguration.Backend["port"] = 1234
	execCtx.preparedConfiguration.Backend["timeout"] = json.Number("1.0")
	observeFixturePlan(t, config, info, execCtx, path)
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"plan", "-out", filepath.Base(path)}))
	fixture.state.Backend["port"] = float64(1234)
	fixture.state.Backend["timeout"] = 1
	info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path
	_, err := restoreConfigurationPlan(t.Context(), config, info)
	require.NoError(t, err)
	require.False(t, equalBackendConfiguration(map[string]any{"value": json.Number("9007199254740993")}, map[string]any{"value": json.Number("9007199254740992")}))
}

func TestNewConfigurationPlanRejectsAPlanRewrittenAfterTerraformWroteIt(t *testing.T) {
	binary, err := os.Executable()
	require.NoError(t, err)
	dir := t.TempDir()
	path := filepath.Join(dir, "reviewed.planfile")
	require.NoError(t, os.WriteFile(path, []byte("reviewed plan"), 0o600))
	captured, err := fileDigest(path)
	require.NoError(t, err)
	info := &schema.ConfigAndStacksInfo{Command: binary, Stack: "dev", ComponentFromArg: "app"}
	execCtx := &componentExecContext{workingDir: dir, preparedConfiguration: &schema.PreparedComponentConfiguration{Engine: "terraform"}}
	snapshot := &planSnapshot{path: path, identity: "identity", workingDigest: "working", planDigest: captured}

	plan, err := newConfigurationPlan(&schema.AtmosConfiguration{}, info, execCtx, snapshot, path)
	require.NoError(t, err)
	require.Equal(t, captured, plan.PlanDigest, "an unchanged plan keeps the existing binding flow")

	require.NoError(t, os.WriteFile(path, []byte("rewritten by an after hook"), 0o600))
	_, err = newConfigurationPlan(&schema.AtmosConfiguration{}, info, execCtx, snapshot, path)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)

	snapshot.planDigest = ""
	plan, err = newConfigurationPlan(&schema.AtmosConfiguration{}, info, execCtx, snapshot, path)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed, "an unobserved plan must not be bound")
	require.Nil(t, plan)
}

// Configuration that merely starts with the state file's name stays bound to the saved plan, while the
// state file, its backups, and the workspace state directory stay out of the digest.
func TestConfigurationWorkdirDigestSeparatesStateFromConfigurationNamedLikeState(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "reviewed.plan")
	write := func(name string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(name), 0o600))
	}
	write("main.tf")
	baseline, err := configurationWorkdirDigest(root, plan, "")
	require.NoError(t, err)

	for _, state := range []string{"terraform.tfstate", "terraform.tfstate.backup", "terraform.tfstate.1700000000.backup", "terraform.tfstate.d/stage/terraform.tfstate", "terraform.tfstate.d/stage/terraform.tfstate.backup", "terraform.tfstate.d/stage/notes.txt"} {
		write(state)
		same, err := configurationWorkdirDigest(root, plan, "")
		require.NoError(t, err)
		require.Equal(t, baseline, same, "%s is mutable state", state)
	}

	for _, configuration := range []string{"terraform.tfstate.tf", "terraform.tfstate.tf.json", "terraform.tfstate.tofu", "terraform.tfstate.tofu.json", "terraform.tfstate.x.backup", "terraform.tfstate..backup", "terraform.tfstate.backup.tf", "terraform.tfstated"} {
		write(configuration)
		changed, err := configurationWorkdirDigest(root, plan, "")
		require.NoError(t, err)
		require.NotEqual(t, baseline, changed, "%s is retained configuration", configuration)
		baseline = changed
	}
}

func TestConfigurationWorkdirDigestResolvesRelativePaths(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "work")
	require.NoError(t, os.Mkdir(root, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.tf"), []byte("source"), 0o600))
	t.Chdir(base)
	absolute, err := configurationWorkdirDigest(root, filepath.Join(root, "reviewed.plan"), "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "reviewed.plan"), []byte("plan"), 0o600))
	for name, plan := range map[string]string{
		"relative root absolute plan": filepath.Join(root, "reviewed.plan"),
		"relative root relative plan": filepath.Join("work", "reviewed.plan"),
	} {
		t.Run(name, func(t *testing.T) {
			digest, err := configurationWorkdirDigest("work", plan, "")
			require.NoError(t, err)
			require.Equal(t, absolute, digest, "the plan file must be excluded")
		})
	}
	t.Run("a different file is not excluded", func(t *testing.T) {
		digest, err := configurationWorkdirDigest("work", filepath.Join("work", "other.plan"), "")
		require.NoError(t, err)
		require.NotEqual(t, absolute, digest)
	})
}

func TestConfigurationPlanPreservesReviewedOverwriteBytes(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	execCtx.preparedConfiguration.Files = []schema.ComponentConfigurationFile{{Path: "generated.tf", Content: []byte("# original A"), IfExists: "overwrite"}}
	generated := filepath.Join(execCtx.workingDir, "generated.tf")
	reviewed := []byte("# reviewed B")
	require.NoError(t, os.WriteFile(generated, reviewed, 0o600))
	snapshot, err := takePlanSnapshot(t.Context(), config, info, execCtx, path)
	require.NoError(t, err)
	// The plan was observed as written by this run, before the after-hook below.
	snapshot.planDigest, err = fileDigest(path)
	require.NoError(t, err)
	execCtx.planSnapshots = &planSnapshotter{result: snapshot}
	// An after-hook must not change the bytes captured in the record.
	require.NoError(t, os.WriteFile(generated, []byte("# after C"), 0o600))
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out=" + path}))
	info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path
	_, err = restoreConfigurationPlan(t.Context(), config, info)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.NoError(t, os.WriteFile(generated, reviewed, 0o600))
	prepared, err := restoreConfigurationPlan(t.Context(), config, info)
	require.NoError(t, err)
	require.Equal(t, reviewed, prepared.Files[0].Content)
	require.NoError(t, writePreparedConfiguration(execCtx.workingDir, prepared))
	actual, err := os.ReadFile(generated)
	require.NoError(t, err)
	require.Equal(t, reviewed, actual)
	require.Equal(t, []byte("# original A"), execCtx.preparedConfiguration.Files[0].Content)
}

func TestSavedPlanBindsFilteredEnvironmentDefaults(t *testing.T) {
	for _, explicitApply := range []bool{false, true} {
		t.Run(fmt.Sprint(explicitApply), func(t *testing.T) {
			config, info, execCtx, _, path := planFixture(t)
			info.ComponentEnvSection = map[string]any{"PROVIDER_TOKEN": "stack-secret"}
			execution := &schema.ComponentExecution{ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"plan", "apply"}, Environment: map[string]string{"PROVIDER_TOKEN": "provider-secret"}}}}
			if explicitApply {
				execution.Apply = &schema.ComponentExecution{}
			}
			execCtx.preparedConfiguration.Execution = execution
			require.NoError(t, applyStackOverrides(config, info, execCtx.preparedConfiguration))
			observeFixturePlan(t, config, info, execCtx, path)
			require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"plan", "-out", filepath.Base(path)}))
			record, err := os.ReadFile(path + configurationPlanSuffix)
			require.NoError(t, err)
			require.NotContains(t, string(record), "provider-secret")
			require.NotContains(t, string(record), "stack-secret")
			info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path
			_, err = restoreConfigurationPlan(t.Context(), config, info)
			require.NoError(t, err)
			info.ComponentEnvSection = nil
			_, err = restoreConfigurationPlan(t.Context(), config, info)
			if explicitApply {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
			}
		})
	}
}

func TestConfigurationPlanPreservesReviewedFileAbsence(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	execCtx.preparedConfiguration.Files = []schema.ComponentConfigurationFile{{Path: "override.tf", Content: []byte("# prepared but removed by before-hook"), IfExists: "overwrite"}}
	observeFixturePlan(t, config, info, execCtx, path)
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out=" + path}))
	info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path
	prepared, err := restoreConfigurationPlan(t.Context(), config, info)
	require.NoError(t, err)
	require.NoError(t, writePreparedConfiguration(execCtx.workingDir, prepared))
	require.NoFileExists(t, filepath.Join(execCtx.workingDir, "override.tf"))
	require.Equal(t, "overwrite", execCtx.preparedConfiguration.Files[0].IfExists)
	// An empty file is different from an absent one and must survive restoration.
	require.NoError(t, os.WriteFile(filepath.Join(execCtx.workingDir, "override.tf"), []byte{}, 0o600))
	info.SubCommand, info.UseTerraformPlan, info.PlanFile = "plan", false, ""
	observeFixturePlan(t, config, info, execCtx, path)
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out=" + path}))
	info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path
	prepared, err = restoreConfigurationPlan(t.Context(), config, info)
	require.NoError(t, err)
	require.NoError(t, writePreparedConfiguration(execCtx.workingDir, prepared))
	data, err := os.ReadFile(filepath.Join(execCtx.workingDir, "override.tf"))
	require.NoError(t, err)
	require.Empty(t, data)
}

// The rebuild path must check environment identity before preparing a new workdir.
func TestRestoredPlanChecksStackEnvironmentBeforeRebuild(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	info.ComponentSection = map[string]any{"metadata": map[string]any{"working_directory": filepath.Join("components", "network")}}
	info.ComponentEnvSection = map[string]any{"PROVIDER_TOKEN": "reviewed-secret"}
	execCtx.preparedConfiguration.Execution = &schema.ComponentExecution{}
	observeFixturePlan(t, config, info, execCtx, path)
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out=" + path}))
	otherRoot := t.TempDir()
	otherWorkdir := filepath.Join(otherRoot, "components", "network")
	require.NoError(t, os.MkdirAll(otherWorkdir, 0o700))
	otherPlan := filepath.Join(otherWorkdir, filepath.Base(path))
	for _, suffix := range []string{"", configurationPlanSuffix} {
		data, err := os.ReadFile(path + suffix)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(otherPlan+suffix, data, 0o600))
	}
	config.BasePath = otherRoot
	info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, otherPlan
	prepared, rebuild, err := restoreOrRebuildConfigurationPlan(t.Context(), config, info)
	require.NoError(t, err)
	require.NotNil(t, prepared)
	require.NotNil(t, rebuild)
	info.ComponentEnvSection = map[string]any{"PROVIDER_TOKEN": "changed-secret"}
	prepared, rebuild, err = restoreOrRebuildConfigurationPlan(t.Context(), config, info)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.ErrorContains(t, err, "reviewed stack environment overrides changed")
	require.NotContains(t, err.Error(), "reviewed-secret")
	require.NotContains(t, err.Error(), "changed-secret")
	require.Nil(t, prepared)
	require.Nil(t, rebuild)
}

func TestConfigurationPlanRejectsRecordWithoutEnvironmentBinding(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	observeFixturePlan(t, config, info, execCtx, path)
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out=" + path}))
	data, err := os.ReadFile(path + configurationPlanSuffix)
	require.NoError(t, err)
	var legacy map[string]any
	require.NoError(t, json.Unmarshal(data, &legacy))
	legacy["Version"] = 3
	delete(legacy, "StackEnvironmentDigest")
	data, err = json.Marshal(legacy)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path+configurationPlanSuffix, data, 0o600))
	info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path
	prepared, rebuild, err := restoreOrRebuildConfigurationPlan(t.Context(), config, info)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.Nil(t, prepared)
	require.Nil(t, rebuild)
}

func TestEffectiveDataDirPrecedence(t *testing.T) {
	tests := []struct {
		name   string
		base   []string
		global map[string]string
		comp   []string
		want   string
	}{
		{name: "default", base: []string{"A=b"}, want: defaultDataDir},
		{name: "base layer", base: []string{"TF_DATA_DIR=base"}, want: "base"},
		{name: "global overrides base", base: []string{"TF_DATA_DIR=base"}, global: map[string]string{"TF_DATA_DIR": "global"}, want: "global"},
		{name: "component overrides global", base: []string{"TF_DATA_DIR=base"}, global: map[string]string{"TF_DATA_DIR": "global"}, comp: []string{"TF_DATA_DIR=component"}, want: "component"},
		{name: "empty value falls back to the default", base: []string{"TF_DATA_DIR=base"}, comp: []string{"TF_DATA_DIR="}, want: defaultDataDir},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := &schema.ConfigAndStacksInfo{SanitizedEnv: test.base, ComponentEnvList: test.comp}
			if info.SanitizedEnv == nil {
				info.SanitizedEnv = []string{}
			}
			config := &schema.AtmosConfiguration{Env: test.global}
			assert.Equal(t, test.want, effectiveDataDir(config, info))
		})
	}
}

func TestResolveDataDir(t *testing.T) {
	// The workdir root reaches resolveDataDir already resolved, as absolutePaths returns it.
	root, err := resolvedPath(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, defaultDataDir), resolveDataDir(root, ""))
	assert.Equal(t, filepath.Join(root, "custom"), resolveDataDir(root, "custom"))
	other, err := resolvedPath(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, other, resolveDataDir(root, other))
}

func TestHasDownloadedModulesHonorsDataDir(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "custom", "modules"), 0o700))
	assert.False(t, hasDownloadedModules(root, ""), "the default directory holds no modules")
	assert.True(t, hasDownloadedModules(root, "custom"))
	assert.True(t, hasDownloadedModules(root, filepath.Join(root, "custom")))
	require.NoError(t, os.MkdirAll(filepath.Join(root, defaultDataDir, "modules"), 0o700))
	assert.True(t, hasDownloadedModules(root, ""))
}

// A custom data directory holds mutable data (the workspace marker, providers) that must not move the
// digests, while its modules subtree is executed content and stays bound.
func TestWorkdirDigestExcludesCustomDataDirOutsideModules(t *testing.T) {
	root := t.TempDir()
	write := func(name string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(name+t.Name()), 0o600))
	}
	rewrite := func(name, content string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o600))
	}
	plan := filepath.Join(root, "reviewed.planfile")
	write("main.tf")
	write("data/modules/vpc/main.tf")

	digest := func(dataDir string) (workdir, rebuild string) {
		t.Helper()
		var err error
		workdir, err = configurationWorkdirDigest(root, plan, dataDir)
		require.NoError(t, err)
		rebuild, err = configurationRebuildDigest(root, plan, dataDir, nil)
		require.NoError(t, err)
		return workdir, rebuild
	}
	baseWorkdir, baseRebuild := digest("data")

	write("data/environment")
	write("data/providers/registry/lock.json")
	rewrite("data/environment", "other-workspace")
	workdir, rebuild := digest("data")
	assert.Equal(t, baseWorkdir, workdir, "the workspace marker is mutable execution data")
	assert.Equal(t, baseRebuild, rebuild, "the workspace marker is mutable execution data")

	rewrite("data/modules/vpc/main.tf", "changed module")
	workdir, rebuild = digest("data")
	assert.NotEqual(t, baseWorkdir, workdir, "downloaded modules stay bound")
	assert.NotEqual(t, baseRebuild, rebuild, "downloaded modules stay bound")

	// Without the data directory the marker is ordinary content, and the default directory is still skipped.
	write(".terraform/environment")
	unscoped, _ := digest("")
	write(".terraform/environment-2")
	again, _ := digest("")
	assert.Equal(t, unscoped, again, "the default .terraform data stays excluded")
	rewrite("data/environment", "third-workspace")
	changed, _ := digest("")
	assert.NotEqual(t, again, changed, "outside the effective data directory the file is content")
}

// writeDigestFile writes content to a slash-separated path below dir, creating parents.
func writeDigestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// bothDigests returns the working directory digest and the rebuild digest of root.
func bothDigests(t *testing.T, root, plan, dataDir string) (workdir, rebuild string) {
	t.Helper()
	workdir, err := configurationWorkdirDigest(root, plan, dataDir)
	require.NoError(t, err)
	rebuild, err = configurationRebuildDigest(root, plan, dataDir, nil)
	require.NoError(t, err)
	return workdir, rebuild
}

// Downloaded modules below a data directory outside the workdir are executed content, so both digests
// must bind them. The mutable data beside them (workspace marker, providers) still does not move them.
func TestWorkdirDigestBindsModulesOfADataDirOutsideTheWorkdir(t *testing.T) {
	root := t.TempDir()
	dataDir := t.TempDir()
	plan := filepath.Join(root, "reviewed.planfile")
	writeDigestFile(t, root, "main.tf", "module")
	writeDigestFile(t, dataDir, "modules/vpc/script.sh", "reviewed script")
	baseWorkdir, baseRebuild := bothDigests(t, root, plan, dataDir)

	writeDigestFile(t, dataDir, "environment", "other-workspace")
	writeDigestFile(t, dataDir, "providers/registry/lock.json", "provider")
	workdir, rebuild := bothDigests(t, root, plan, dataDir)
	assert.Equal(t, baseWorkdir, workdir, "the workspace marker and providers are mutable execution data")
	assert.Equal(t, baseRebuild, rebuild, "the workspace marker and providers are mutable execution data")

	writeDigestFile(t, dataDir, "modules/vpc/script.sh", "tampered script")
	workdir, rebuild = bothDigests(t, root, plan, dataDir)
	assert.NotEqual(t, baseWorkdir, workdir, "a downloaded module outside the workdir stays bound")
	assert.NotEqual(t, baseRebuild, rebuild, "a downloaded module outside the workdir stays bound")
}

// A data directory below the default `.terraform` directory (TF_DATA_DIR=.terraform/custom) must keep its
// downloaded modules reachable, while the runtime paths beside them and the default layout stay as before.
func TestWorkdirDigestBindsModulesOfADataDirBelowTheDefaultCache(t *testing.T) {
	tests := []struct {
		name    string
		dataDir string
		prefix  string
	}{
		{name: "one level below .terraform", dataDir: filepath.Join(defaultDataDir, "custom"), prefix: ".terraform/custom"},
		{name: "two levels below .terraform", dataDir: filepath.Join(defaultDataDir, "a", "b"), prefix: ".terraform/a/b"},
		{name: "default data directory", dataDir: "", prefix: ".terraform"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			plan := filepath.Join(root, "reviewed.planfile")
			writeDigestFile(t, root, "main.tf", "module")
			writeDigestFile(t, root, test.prefix+"/modules/vpc/script.sh", "reviewed script")
			baseWorkdir, baseRebuild := bothDigests(t, root, plan, test.dataDir)

			writeDigestFile(t, root, test.prefix+"/environment", "other-workspace")
			writeDigestFile(t, root, test.prefix+"/terraform.tfstate", "state")
			writeDigestFile(t, root, test.prefix+"/providers/registry/lock.json", "provider")
			writeDigestFile(t, root, ".terraform/sibling/cache.json", "sibling cache")
			workdir, rebuild := bothDigests(t, root, plan, test.dataDir)
			assert.Equal(t, baseWorkdir, workdir, "runtime paths and sibling cache stay excluded")
			assert.Equal(t, baseRebuild, rebuild, "runtime paths and sibling cache stay excluded")

			writeDigestFile(t, root, test.prefix+"/modules/vpc/script.sh", "tampered script")
			workdir, rebuild = bothDigests(t, root, plan, test.dataDir)
			assert.NotEqual(t, baseWorkdir, workdir, "a downloaded module stays bound")
			assert.NotEqual(t, baseRebuild, rebuild, "a downloaded module stays bound")
		})
	}
}

// A data directory outside the workdir that holds no downloaded modules adds nothing to the digests.
func TestWorkdirDigestIgnoresAnExternalDataDirWithoutModules(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(t.TempDir(), "absent")
	plan := filepath.Join(root, "reviewed.planfile")
	writeDigestFile(t, root, "main.tf", "module")
	withDefault, err := configurationWorkdirDigest(root, plan, "")
	require.NoError(t, err)
	absent, err := configurationWorkdirDigest(root, plan, dataDir)
	require.NoError(t, err)
	assert.Equal(t, withDefault, absent)
}

// TF_DATA_DIR pointing at the workdir itself, or at one of its parents, must not classify the root
// configuration as mutable Terraform data. Only the established runtime paths are excluded.
func TestWorkdirDigestKeepsConfigurationWhenTheDataDirOverlapsTheWorkdir(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "component")
	plan := filepath.Join(root, "reviewed.planfile")
	for name, dataDir := range map[string]string{"workdir itself": root, "parent of the workdir": parent} {
		t.Run(name, func(t *testing.T) {
			writeDigestFile(t, root, "main.tf", "module v1")
			writeDigestFile(t, root, "nested/child.tf", "child v1")
			baseWorkdir, baseRebuild := bothDigests(t, root, plan, dataDir)

			writeDigestFile(t, root, "main.tf", "module v2")
			workdir, rebuild := bothDigests(t, root, plan, dataDir)
			assert.NotEqual(t, baseWorkdir, workdir, "root configuration stays bound")
			assert.NotEqual(t, baseRebuild, rebuild, "root configuration stays bound")

			writeDigestFile(t, root, "nested/child.tf", "child v2")
			changed, _ := bothDigests(t, root, plan, dataDir)
			assert.NotEqual(t, workdir, changed, "nested configuration stays bound")
		})
	}
}

// With the data directory at the workdir, the established runtime paths of a data directory stay excluded.
func TestWorkdirDigestExcludesRuntimePathsWhenTheDataDirIsTheWorkdir(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "reviewed.planfile")
	writeDigestFile(t, root, "main.tf", "module")
	writeDigestFile(t, root, "modules/vpc/main.tf", "downloaded module")
	baseWorkdir, baseRebuild := bothDigests(t, root, plan, root)

	writeDigestFile(t, root, "environment", "other-workspace")
	writeDigestFile(t, root, "plugin_path", "path")
	writeDigestFile(t, root, "providers/registry/lock.json", "provider")
	workdir, rebuild := bothDigests(t, root, plan, root)
	assert.Equal(t, baseWorkdir, workdir)
	assert.Equal(t, baseRebuild, rebuild)

	writeDigestFile(t, root, "modules/vpc/main.tf", "tampered module")
	workdir, _ = bothDigests(t, root, plan, root)
	assert.NotEqual(t, baseWorkdir, workdir, "downloaded modules stay bound")
}

// TF_DATA_DIR set through extra_arguments is applied to some phases only, so one digest cannot match
// every phase. A reviewed plan is rejected before it is recorded, and a recorded one before it is applied.
func TestReviewedPlanRejectsADataDirFromExtraArguments(t *testing.T) {
	extra := func(environment map[string]string) *schema.ComponentExecution {
		return &schema.ComponentExecution{ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"init"}, Environment: environment}}}
	}
	t.Run("snapshot", func(t *testing.T) {
		config, info, execCtx, _, path := planFixture(t)
		info.ComponentExecution = extra(map[string]string{"TF_DATA_DIR": filepath.Join(t.TempDir(), "data")})
		_, err := takePlanSnapshot(t.Context(), config, info, execCtx, path)
		require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	})
	t.Run("snapshot accepts other environment", func(t *testing.T) {
		config, info, execCtx, _, path := planFixture(t)
		info.ComponentExecution = extra(map[string]string{"TF_LOG": "debug"})
		_, err := takePlanSnapshot(t.Context(), config, info, execCtx, path)
		require.NoError(t, err)
	})
	t.Run("restore", func(t *testing.T) {
		config, info, execCtx, _, path := planFixture(t)
		observeFixturePlan(t, config, info, execCtx, path)
		require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out=" + path}))
		execCtx.preparedConfiguration.Execution = extra(map[string]string{"TF_DATA_DIR": "elsewhere"})
		// Rewrite the record as an older run that accepted the layout would have.
		rewriteRecordExecution(t, path, execCtx.preparedConfiguration.Execution)
		info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path
		prepared, rebuild, err := restoreOrRebuildConfigurationPlan(t.Context(), config, info)
		require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
		require.Nil(t, prepared)
		require.Nil(t, rebuild)
	})
}

// A relative working_directory override takes the retained-path shortcut, which must select the explicit
// source subdirectory the plan ran in, as planning does.
func TestRetainedWorkdirShortcutSelectsTheSourceSubdirectory(t *testing.T) {
	override := filepath.Join("components", "network")
	tests := []struct {
		name     string
		metadata map[string]any
		create   string
		want     func(root string) string
		wantErr  error
	}{
		{name: "subdirectory present", metadata: map[string]any{"source_subdir": "modules/app"}, create: filepath.Join("components", "network", "modules", "app"), want: func(root string) string { return filepath.Join(root, "components", "network", "modules", "app") }},
		{name: "no subdirectory keeps the override", metadata: map[string]any{}, create: filepath.Join("components", "network"), want: func(root string) string { return filepath.Join(root, "components", "network") }},
		{name: "subdirectory absent selects the module path it will have", metadata: map[string]any{"source_subdir": "modules/app"}, create: filepath.Join("components", "network"), want: func(root string) string { return filepath.Join(root, "components", "network", "modules", "app") }},
		{name: "escaping subdirectory is rejected", metadata: map[string]any{"source_subdir": "../escape"}, create: filepath.Join("components", "network"), wantErr: errUtils.ErrWorkdirProvision},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(root, test.create), 0o700))
			config := &schema.AtmosConfiguration{BasePath: root}
			metadata := map[string]any{"working_directory": override}
			for key, value := range test.metadata {
				metadata[key] = value
			}
			info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"metadata": metadata}}

			err := useRetainedWorkdir(config, info)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			want := test.want(root)
			assert.Equal(t, want, ConstructTerraformComponentWorkingDir(config, info))
			if test.name == "subdirectory absent selects the module path it will have" {
				return
			}
			// A later provisioning step resolves the subpath again and must not join it a second time.
			again, err := component.ApplyWorkdirSubpathToSection(info)
			require.NoError(t, err)
			assert.Equal(t, want, again)
		})
	}
}

// A plan reviewed in work/units/app (working_directory work, source_subdir units/app) and applied on a fresh
// checkout where the subdirectory is not on disk yet must reach the rebuild at the recorded module path, in
// the same checkout and in a relocated one, instead of being rejected before the rebuild can happen.
func TestRetainedWorkdirShortcutOfAFreshCheckoutReachesTheRebuildAtTheRecordedModulePath(t *testing.T) {
	tests := []struct {
		name      string
		relocated bool
	}{
		{name: "same checkout"},
		{name: "relocated checkout", relocated: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, info, execCtx, _, _ := planFixture(t)
			override, subdir := "work", filepath.Join("units", "app")
			metadata := func() map[string]any {
				return map[string]any{"metadata": map[string]any{"working_directory": override, "source_subdir": filepath.ToSlash(subdir)}}
			}
			info.ComponentSection = metadata()
			module := filepath.Join(config.BasePath, override, subdir)
			require.NoError(t, os.MkdirAll(module, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(module, "main.tf"), []byte("module"), 0o600))
			require.NoError(t, useRetainedWorkdir(config, info))
			require.Equal(t, module, ConstructTerraformComponentWorkingDir(config, info))
			execCtx.workingDir, execCtx.componentPath = module, module
			plan := filepath.Join(config.BasePath, override, "reviewed.planfile")
			require.NoError(t, os.WriteFile(plan, []byte("reviewed plan"), 0o600))
			observeFixturePlan(t, config, info, execCtx, plan)
			require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out=" + plan}))

			// Fresh checkout: only the restored plan and record exist below the override.
			target, targetPlan := config.BasePath, plan
			if test.relocated {
				target = t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(target, override), 0o700))
				targetPlan = filepath.Join(target, override, "reviewed.planfile")
				for _, suffix := range []string{"", configurationPlanSuffix} {
					data, err := os.ReadFile(plan + suffix)
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(targetPlan+suffix, data, 0o600))
				}
			}
			require.NoError(t, os.RemoveAll(filepath.Join(config.BasePath, override, "units")))
			config.BasePath = target
			fresh := *info
			fresh.ComponentSection = metadata()
			require.NoError(t, useRetainedWorkdir(config, &fresh))
			fresh.SubCommand, fresh.UseTerraformPlan, fresh.PlanFile = "apply", true, targetPlan

			_, rebuild, err := restoreOrRebuildConfigurationPlan(t.Context(), config, &fresh)
			require.NoError(t, err)
			require.NotNil(t, rebuild)
			assert.Equal(t, filepath.Join(target, override, subdir), rebuild.workdir)
		})
	}
}

// rewriteRecordExecution replaces the execution metadata stored in the plan record beside path.
func rewriteRecordExecution(t *testing.T, path string, execution *schema.ComponentExecution) {
	t.Helper()
	data, err := os.ReadFile(path + configurationPlanSuffix)
	require.NoError(t, err)
	var record map[string]any
	require.NoError(t, json.Unmarshal(data, &record))
	encoded, err := json.Marshal(execution)
	require.NoError(t, err)
	var value any
	require.NoError(t, json.Unmarshal(encoded, &value))
	prepared, ok := record["Prepared"].(map[string]any)
	require.True(t, ok)
	prepared["Execution"] = value
	data, err = json.Marshal(record)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path+configurationPlanSuffix, data, 0o600))
}

// writeModuleManifest writes a Terraform module manifest into a data directory whose module records use
// the absolute directory of that data directory, as Terraform does for an absolute TF_DATA_DIR.
func writeModuleManifest(t *testing.T, dataDir, source, script string) {
	t.Helper()
	writeDigestFile(t, dataDir, "modules/vpc/script.sh", script)
	manifest := map[string]any{"Modules": []map[string]string{
		{"Key": "", "Source": "", "Dir": "."},
		{"Key": "vpc", "Source": source, "Dir": filepath.ToSlash(filepath.Join(dataDir, "modules", "vpc"))},
	}}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	writeDigestFile(t, dataDir, "modules/modules.json", string(data))
}

// The module manifest records runner-local absolute directories, so the same layout mounted at another
// root must still verify, while a changed module or a changed manifest source must not.
func TestWorkdirDigestIgnoresTheRunnerRootOfTheModuleManifest(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "reviewed.planfile")
	writeDigestFile(t, root, "main.tf", "module")
	reviewed := t.TempDir()
	writeModuleManifest(t, reviewed, "registry/vpc", "reviewed script")
	baseWorkdir, baseRebuild := bothDigests(t, root, plan, reviewed)

	tests := []struct {
		name   string
		source string
		script string
		same   bool
	}{
		{name: "identical layout on a differently rooted runner", source: "registry/vpc", script: "reviewed script", same: true},
		{name: "changed module content", source: "registry/vpc", script: "tampered script"},
		{name: "changed manifest source", source: "registry/other", script: "reviewed script"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			other := t.TempDir()
			writeModuleManifest(t, other, test.source, test.script)
			workdir, rebuild := bothDigests(t, root, plan, other)
			if test.same {
				assert.Equal(t, baseWorkdir, workdir)
				assert.Equal(t, baseRebuild, rebuild)
				return
			}
			assert.NotEqual(t, baseWorkdir, workdir)
			assert.NotEqual(t, baseRebuild, rebuild)
		})
	}
}

// A manifest that is not the expected shape is hashed as it is, so it can never be dropped from the digest.
func TestWorkdirDigestHashesAnUnparseableModuleManifest(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "reviewed.planfile")
	writeDigestFile(t, root, "main.tf", "module")
	dataDir := t.TempDir()
	writeDigestFile(t, dataDir, "modules/modules.json", "not json")
	base, _ := bothDigests(t, root, plan, dataDir)
	writeDigestFile(t, dataDir, "modules/modules.json", "other bytes")
	changed, _ := bothDigests(t, root, plan, dataDir)
	assert.NotEqual(t, base, changed)
}

// A manifest Dir that was already relative and lies outside the data directory must not collide with a
// Dir rewritten relative to the data directory, or changing the selected module directory goes unnoticed.
func TestWorkdirDigestDistinguishesNormalizedModuleDirsFromRelativeOnes(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "reviewed.planfile")
	writeDigestFile(t, root, "main.tf", "module")
	writeManifestDir := func(dataDir, dir string) {
		t.Helper()
		writeDigestFile(t, dataDir, "modules/vpc/script.sh", "script")
		manifest := map[string]any{"Modules": []map[string]string{{"Key": "vpc", "Source": "registry/vpc", "Dir": dir}}}
		data, err := json.Marshal(manifest)
		require.NoError(t, err)
		writeDigestFile(t, dataDir, "modules/modules.json", string(data))
	}
	inside := t.TempDir()
	writeManifestDir(inside, filepath.ToSlash(filepath.Join(inside, "modules", "vpc")))
	insideWorkdir, insideRebuild := bothDigests(t, root, plan, inside)

	tests := []struct {
		name string
		dir  string
		same bool
	}{
		{name: "relative Dir that spells the normalized path", dir: "modules/vpc"},
		{name: "relative Dir elsewhere", dir: "other/vpc"},
		{name: "Dir below the data directory on another runner root", dir: "", same: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			other := t.TempDir()
			dir := test.dir
			if test.same {
				dir = filepath.ToSlash(filepath.Join(other, "modules", "vpc"))
			}
			writeManifestDir(other, dir)
			workdir, rebuild := bothDigests(t, root, plan, other)
			if test.same {
				assert.Equal(t, insideWorkdir, workdir)
				assert.Equal(t, insideRebuild, rebuild)
				return
			}
			assert.NotEqual(t, insideWorkdir, workdir)
			assert.NotEqual(t, insideRebuild, rebuild)
		})
	}
}

// A custom data directory below the workdir may sit beside local module sources. Only the runtime files
// Terraform writes there stay excluded, so a changed neighbor such as data/child moves the digest.
func TestWorkdirDigestKeepsLocalModulesBesideACustomDataDir(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "reviewed.planfile")
	writeDigestFile(t, root, "main.tf", "module")
	writeDigestFile(t, root, "data/child/main.tf", "reviewed child")
	writeDigestFile(t, root, "data/environment", "default")
	writeDigestFile(t, root, "data/terraform.tfstate", "state")
	writeDigestFile(t, root, "data/providers/registry/lock.json", "provider")
	baseWorkdir, baseRebuild := bothDigests(t, root, plan, "data")

	tests := []struct {
		name    string
		file    string
		content string
		same    bool
	}{
		{name: "workspace marker", file: "data/environment", content: "other", same: true},
		{name: "backend state", file: "data/terraform.tfstate", content: "other", same: true},
		{name: "installed provider", file: "data/providers/registry/lock.json", content: "other", same: true},
		{name: "neighboring local module", file: "data/child/main.tf", content: "tampered child"},
		{name: "new neighboring local module file", file: "data/child/extra.tf", content: "extra"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writeDigestFile(t, root, test.file, test.content)
			t.Cleanup(func() {
				if test.same {
					return
				}
				writeDigestFile(t, root, "data/child/main.tf", "reviewed child")
				require.NoError(t, os.RemoveAll(filepath.Join(root, "data", "child", "extra.tf")))
			})
			workdir, rebuild := bothDigests(t, root, plan, "data")
			if test.same {
				assert.Equal(t, baseWorkdir, workdir)
				assert.Equal(t, baseRebuild, rebuild)
				return
			}
			assert.NotEqual(t, baseWorkdir, workdir)
			assert.NotEqual(t, baseRebuild, rebuild)
		})
	}
}

// A rebuilt workdir must still hold every recorded public file, even where the rebuild digest does not
// look (below a Terragrunt cache). Files absent under a removal policy have no record and stay absent.
func TestVerifyRebuildChecksRecordedPublicFiles(t *testing.T) {
	const cached = ".terragrunt-cache/unit/generated.tf"
	build := func(t *testing.T) (*reviewedRebuild, string) {
		t.Helper()
		workdir := t.TempDir()
		plan := filepath.Join(workdir, "reviewed.planfile")
		writeDigestFile(t, workdir, "main.tf", "module")
		writeDigestFile(t, workdir, cached, "reviewed")
		digest, err := configurationRebuildDigest(workdir, plan, "", nil)
		require.NoError(t, err)
		sum := sha256.Sum256([]byte("reviewed"))
		reviewed := &configurationPlan{
			RebuildDigest: digest,
			Prepared:      &schema.PreparedComponentConfiguration{},
			PublicFiles: map[string]publicFileRecord{
				cached:      {Present: true, Digest: hex.EncodeToString(sum[:])},
				"absent.tf": {},
			},
		}
		return &reviewedRebuild{plan: reviewed, planPath: plan, workdir: workdir}, workdir
	}

	tests := []struct {
		name   string
		mutate func(t *testing.T, workdir string)
		fails  bool
	}{
		{name: "unchanged files verify", mutate: func(*testing.T, string) {}},
		{name: "changed public file", fails: true, mutate: func(t *testing.T, workdir string) {
			writeDigestFile(t, workdir, cached, "tampered")
		}},
		{name: "missing public file", fails: true, mutate: func(t *testing.T, workdir string) {
			require.NoError(t, os.Remove(filepath.Join(workdir, filepath.FromSlash(cached))))
		}},
		{name: "file recorded absent now present", fails: true, mutate: func(t *testing.T, workdir string) {
			writeDigestFile(t, workdir, "absent.tf", "appeared")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rebuild, workdir := build(t)
			test.mutate(t, workdir)
			err := rebuild.verify(workdir)
			if test.fails {
				require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
				return
			}
			require.NoError(t, err)
		})
	}
}

// The failure to resolve the retained base path keeps its cause and is classified as a workdir failure.
func TestUseRetainedWorkdirWrapsABasePathFailure(t *testing.T) {
	cause := errors.New("working directory unreadable")
	original := absolutePath
	absolutePath = func(string) (string, error) { return "", cause }
	t.Cleanup(func() { absolutePath = original })

	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"metadata": map[string]any{"working_directory": filepath.Join("components", "network")}}}
	err := useRetainedWorkdir(config, info)
	require.ErrorIs(t, err, errUtils.ErrWorkdirProvision)
	require.ErrorIs(t, err, cause)
}

func TestStashStalePlanRejectsDirectoryOutputs(t *testing.T) {
	output := filepath.Join(t.TempDir(), "plans")
	require.NoError(t, os.MkdirAll(filepath.Join(output, "keep"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(output, "keep", "file"), []byte("data"), 0o600))

	_, err := stashStalePlan("", "", output)

	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	data, readErr := os.ReadFile(filepath.Join(output, "keep", "file"))
	require.NoError(t, readErr, "the directory must stay where it was")
	require.Equal(t, "data", string(data))
}

func TestStalePlanStashStaysOutOfDownloadedModules(t *testing.T) {
	root := t.TempDir()
	modules := filepath.Join(root, ".terraform", "modules")
	require.NoError(t, os.MkdirAll(modules, 0o755))
	plan := filepath.Join(modules, "reviewed.plan")
	require.NoError(t, os.WriteFile(plan, []byte("earlier"), 0o600))

	settle, err := stashStalePlan(root, "", plan)
	require.NoError(t, err)
	entries, err := os.ReadDir(modules)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the stash must not be created inside the digested module tree")
	require.Equal(t, "reviewed.plan", entries[0].Name())

	require.NoError(t, settle(true))
	data, err := os.ReadFile(plan)
	require.NoError(t, err)
	require.Equal(t, "earlier", string(data))
	rootEntries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, rootEntries, 1, "only .terraform remains once the stash is settled")
	require.Equal(t, ".terraform", rootEntries[0].Name())
}

func TestStalePlanStashIsKeptWhenRestoreFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs file write permissions to be enforced")
	}
	out := t.TempDir()
	plan := filepath.Join(out, "reviewed.plan")
	require.NoError(t, os.WriteFile(plan, []byte("earlier"), 0o600))

	settle, err := stashStalePlan("", "", plan)
	require.NoError(t, err)
	stashes, err := filepath.Glob(filepath.Join(out, stalePlanDirPrefix+"*", "plan"))
	require.NoError(t, err)
	require.Len(t, stashes, 1)
	// A read-only plan file makes writing the earlier contents back fail.
	require.NoError(t, os.Chmod(plan, 0o400))

	err = settle(false)

	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.ErrorContains(t, err, stashes[0])
	data, readErr := os.ReadFile(stashes[0])
	require.NoError(t, readErr, "the stash must be kept for recovery")
	require.Equal(t, "earlier", string(data))
}

func TestStalePlanStashEmptiesThePlanInPlace(t *testing.T) {
	out := t.TempDir()
	target := filepath.Join(out, "target.plan")
	require.NoError(t, os.WriteFile(target, []byte("earlier"), 0o600))
	before, err := os.Stat(target)
	require.NoError(t, err)

	settle, err := stashStalePlan("", "", target)
	require.NoError(t, err)

	after, err := os.Stat(target)
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after), "the plan is emptied in place, not replaced")
	require.Zero(t, after.Size())
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), after.Mode().Perm(), "a private plan keeps its mode for Terraform's write")
	}
	stashes, err := filepath.Glob(filepath.Join(out, stalePlanDirPrefix+"*", "plan"))
	require.NoError(t, err)
	require.Len(t, stashes, 1)
	if runtime.GOOS != "windows" {
		stashInfo, statErr := os.Stat(stashes[0])
		require.NoError(t, statErr)
		require.Equal(t, os.FileMode(0o600), stashInfo.Mode().Perm(), "the stashed copy is private")
	}

	// A run that writes a plan keeps it, and the stash is discarded.
	require.NoError(t, os.WriteFile(target, []byte("fresh"), 0o600))
	require.NoError(t, settle(true))
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "fresh", string(data))
	require.NoDirExists(t, filepath.Dir(stashes[0]))
}

func TestStalePlanStashWritesThroughASymlink(t *testing.T) {
	out := t.TempDir()
	target := filepath.Join(out, "target.plan")
	link := filepath.Join(out, "link.plan")
	require.NoError(t, os.WriteFile(target, []byte("earlier"), 0o600))
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	settle, err := stashStalePlan("", "", link)
	require.NoError(t, err)
	info, err := os.Lstat(link)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSymlink, "the link itself is left in place")
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Empty(t, data, "the target is emptied, as Terraform would rewrite it through the link")

	// A run that writes nothing gets the earlier plan back in the target.
	require.NoError(t, settle(true))
	data, err = os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "earlier", string(data))
}

func TestWrittenPlanDigestRejectsAnEmptyPlan(t *testing.T) {
	plan := filepath.Join(t.TempDir(), "reviewed.plan")
	require.NoError(t, os.WriteFile(plan, nil, 0o600))

	_, err := writtenPlanDigest(plan)

	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.NoError(t, os.WriteFile(plan, []byte("plan"), 0o600))
	digest, err := writtenPlanDigest(plan)
	require.NoError(t, err)
	require.NotEmpty(t, digest)
}

func TestFailedRunReportsTheStashLocation(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs file write permissions to be enforced")
	}
	config, info, execCtx, _, path := planFixture(t)
	snapshotter := &planSnapshotter{ctx: t.Context(), config: config, info: info, execCtx: execCtx, pathFrom: func([]string) string { return path }}
	execCtx.planSnapshots = snapshotter
	terraformFailed := errUtils.ExitCodeError{Code: 1}
	// The run writes nothing and leaves the plan unwritable, so the earlier plan cannot be restored.
	run := func() error {
		require.NoError(t, os.Chmod(path, 0o400))
		return terraformFailed
	}

	err := snapshotter.around([]string{"plan", "-out=" + path}, run, failedOnError)()

	require.ErrorIs(t, err, terraformFailed, "the Terraform result still reaches the caller")
	require.Equal(t, 1, errUtils.GetExitCode(err), "the exit code the hooks classify is unchanged")
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.ErrorContains(t, err, stalePlanDirPrefix, "the operator learns where the earlier plan is kept")
}

func TestStalePlanStashStaysOutOfTheEffectiveModulesDirectory(t *testing.T) {
	t.Run("custom data directory below the workdir", func(t *testing.T) {
		root := t.TempDir()
		modules := filepath.Join(root, "custom-data", modulesDirName)
		require.NoError(t, os.MkdirAll(modules, 0o755))
		plan := filepath.Join(modules, "reviewed.plan")
		require.NoError(t, os.WriteFile(plan, []byte("earlier"), 0o600))

		settle, err := stashStalePlan(root, "custom-data", plan)
		require.NoError(t, err)
		entries, err := os.ReadDir(modules)
		require.NoError(t, err)
		require.Len(t, entries, 1, "the stash must not be created inside the digested custom modules tree")
		require.Equal(t, "reviewed.plan", entries[0].Name())

		require.NoError(t, settle(true))
		data, err := os.ReadFile(plan)
		require.NoError(t, err)
		require.Equal(t, "earlier", string(data))
		rootEntries, err := os.ReadDir(root)
		require.NoError(t, err)
		require.Len(t, rootEntries, 1, "only the data directory remains once the stash is settled")
	})

	t.Run("data directory outside the workdir", func(t *testing.T) {
		root := t.TempDir()
		external := t.TempDir()
		modules := filepath.Join(external, modulesDirName)
		require.NoError(t, os.MkdirAll(modules, 0o755))
		plan := filepath.Join(modules, "reviewed.plan")
		require.NoError(t, os.WriteFile(plan, []byte("earlier"), 0o600))

		settle, err := stashStalePlan(root, external, plan)
		require.NoError(t, err)
		entries, err := os.ReadDir(modules)
		require.NoError(t, err)
		require.Len(t, entries, 1, "the stash must not be created inside the external modules tree")
		stashes, err := filepath.Glob(filepath.Join(root, stalePlanDirPrefix+"*", "plan"))
		require.NoError(t, err)
		require.Len(t, stashes, 1, "the stash moves to the component directory")

		require.NoError(t, settle(true))
		require.NoDirExists(t, filepath.Dir(stashes[0]))
	})

	t.Run("a plan outside every modules directory keeps its stash beside it", func(t *testing.T) {
		root := t.TempDir()
		plan := filepath.Join(root, "reviewed.plan")
		require.NoError(t, os.WriteFile(plan, []byte("earlier"), 0o600))

		settle, err := stashStalePlan(root, "custom-data", plan)
		require.NoError(t, err)
		stashes, err := filepath.Glob(filepath.Join(root, stalePlanDirPrefix+"*", "plan"))
		require.NoError(t, err)
		require.Len(t, stashes, 1)
		require.NoError(t, settle(true))
	})
}

// A workdir that itself lies inside the effective modules tree (TF_DATA_DIR=/project/cache, workdir
// /project/cache/modules/app) is fully digested, so the stash must not be created below it: both snapshots
// would hash it and settlement would delete it, which looks like a configuration change.
func TestStalePlanStashStaysOutsideADigestedWorkdirInsideTheModulesTree(t *testing.T) {
	cache := t.TempDir()
	root := filepath.Join(cache, modulesDirName, "app")
	writeDigestFile(t, root, "main.tf", "module")
	plan := filepath.Join(root, "reviewed.plan")
	require.NoError(t, os.WriteFile(plan, []byte("earlier"), 0o600))
	before, err := configurationWorkdirDigest(root, plan, cache)
	require.NoError(t, err)

	settle, err := stashStalePlan(root, cache, plan)
	require.NoError(t, err)
	during, err := configurationWorkdirDigest(root, plan, cache)
	require.NoError(t, err)
	assert.Equal(t, before, during, "a stash below the workdir would change the digest")
	stashes, err := filepath.Glob(filepath.Join(root, stalePlanDirPrefix+"*"))
	require.NoError(t, err)
	assert.Empty(t, stashes, "the stash must not be created inside the digested workdir")

	require.NoError(t, settle(true))
	data, err := os.ReadFile(plan)
	require.NoError(t, err)
	assert.Equal(t, "earlier", string(data))
	after, err := configurationWorkdirDigest(root, plan, cache)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

// A workdir outside the modules tree whose plan sits in the external modules directory keeps the stash in
// the workdir, as before.
func TestStalePlanStashOfAWorkdirOutsideTheModulesTreeStaysInTheWorkdir(t *testing.T) {
	root := t.TempDir()
	cache := t.TempDir()
	modules := filepath.Join(cache, modulesDirName)
	require.NoError(t, os.MkdirAll(modules, 0o755))
	plan := filepath.Join(modules, "reviewed.plan")
	require.NoError(t, os.WriteFile(plan, []byte("earlier"), 0o600))

	settle, err := stashStalePlan(root, cache, plan)
	require.NoError(t, err)
	stashes, err := filepath.Glob(filepath.Join(root, stalePlanDirPrefix+"*", "plan"))
	require.NoError(t, err)
	assert.Len(t, stashes, 1)
	require.NoError(t, settle(true))
}

// A temporary directory that itself lies inside the digested workdir (TMPDIR=<workdir>/tmp) is no safe
// stash location: both digest snapshots would hash it and settlement would delete it. The stash is refused
// with a hint rather than created there, and the plan is left untouched.
func TestStalePlanStashRefusesATemporaryDirectoryInsideTheDigestedWorkdir(t *testing.T) {
	cache := t.TempDir()
	root := filepath.Join(cache, modulesDirName, "app")
	writeDigestFile(t, root, "main.tf", "module")
	tmp := filepath.Join(root, "tmp")
	require.NoError(t, os.MkdirAll(tmp, 0o755))
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	plan := filepath.Join(root, "reviewed.plan")
	require.NoError(t, os.WriteFile(plan, []byte("earlier"), 0o600))
	before, err := configurationWorkdirDigest(root, plan, cache)
	require.NoError(t, err)

	settle, err := stashStalePlan(root, cache, plan)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.NotNil(t, settle)
	require.NoError(t, settle(true), "the returned settle function is a no-op after a refusal")

	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)
	assert.Empty(t, entries, "no stash may be left in the temporary directory inside the workdir")
	data, err := os.ReadFile(plan)
	require.NoError(t, err)
	assert.Equal(t, "earlier", string(data), "the plan is not emptied when the stash is refused")
	after, err := configurationWorkdirDigest(root, plan, cache)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

// A temporary directory outside the digested workdir stays the stash location for a workdir inside the
// modules tree, and the stash is private and settles away.
func TestStalePlanStashUsesATemporaryDirectoryOutsideTheDigestedWorkdir(t *testing.T) {
	cache := t.TempDir()
	root := filepath.Join(cache, modulesDirName, "app")
	writeDigestFile(t, root, "main.tf", "module")
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	plan := filepath.Join(root, "reviewed.plan")
	require.NoError(t, os.WriteFile(plan, []byte("earlier"), 0o600))

	settle, err := stashStalePlan(root, cache, plan)
	require.NoError(t, err)
	stashes, err := filepath.Glob(filepath.Join(tmp, stalePlanDirPrefix+"*", "plan"))
	require.NoError(t, err)
	require.Len(t, stashes, 1, "the stash lives in the temporary directory outside the workdir")
	require.NoError(t, settle(true))
	require.NoDirExists(t, filepath.Dir(stashes[0]))
}

// Terraform writing the plan inside an external TF_DATA_DIR/modules directory must not look like a module
// change: the plan and its record are excluded from the external modules walk, every other module file stays
// bound.
func TestWorkdirDigestIgnoresAPlanInsideExternalModules(t *testing.T) {
	root := t.TempDir()
	dataDir := t.TempDir()
	plan := filepath.Join(dataDir, modulesDirName, "plans", "reviewed.plan")
	writeDigestFile(t, root, "main.tf", "module")
	writeDigestFile(t, dataDir, "modules/vpc/script.sh", "reviewed script")
	baseWorkdir, baseRebuild := bothDigests(t, root, plan, dataDir)

	writeDigestFile(t, dataDir, "modules/plans/reviewed.plan", "plan bytes")
	writeDigestFile(t, dataDir, "modules/plans/reviewed.plan"+configurationPlanSuffix, "plan record")
	workdir, rebuild := bothDigests(t, root, plan, dataDir)
	assert.Equal(t, baseWorkdir, workdir, "the plan and its record are not configuration")
	assert.Equal(t, baseRebuild, rebuild, "the plan and its record are not configuration")

	writeDigestFile(t, dataDir, "modules/plans/other.tf", "tampered")
	workdir, rebuild = bothDigests(t, root, plan, dataDir)
	assert.NotEqual(t, baseWorkdir, workdir, "another file beside the plan stays bound")
	assert.NotEqual(t, baseRebuild, rebuild, "another file beside the plan stays bound")
}

// A data directory that is a parent of the workdir (TF_DATA_DIR=/project for /project/unit) keeps its
// downloaded modules outside the workdir. Both digests must bind them, while the workdir configuration, the
// runtime paths of the data directory, and unrelated siblings keep their classification.
func TestWorkdirDigestBindsModulesOfADataDirThatIsAParentOfTheWorkdir(t *testing.T) {
	tests := []struct {
		name  string
		depth int
	}{
		{name: "direct parent", depth: 1},
		{name: "grandparent", depth: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			root := dataDir
			for _, name := range []string{"a", "unit"}[2-test.depth:] {
				root = filepath.Join(root, name)
			}
			plan := filepath.Join(root, "reviewed.planfile")
			writeDigestFile(t, root, "main.tf", "module")
			writeDigestFile(t, dataDir, "modules/vpc/script.sh", "reviewed script")
			baseWorkdir, baseRebuild := bothDigests(t, root, plan, dataDir)

			writeDigestFile(t, dataDir, "environment", "other-workspace")
			writeDigestFile(t, dataDir, "terraform.tfstate", "state")
			writeDigestFile(t, dataDir, "providers/registry/lock.json", "provider")
			writeDigestFile(t, dataDir, "sibling/main.tf", "not part of this workdir")
			workdir, rebuild := bothDigests(t, root, plan, dataDir)
			assert.Equal(t, baseWorkdir, workdir, "runtime paths and siblings of the data directory stay excluded")
			assert.Equal(t, baseRebuild, rebuild, "runtime paths and siblings of the data directory stay excluded")

			writeDigestFile(t, root, "main.tf", "module v2")
			workdir, rebuild = bothDigests(t, root, plan, dataDir)
			assert.NotEqual(t, baseWorkdir, workdir, "the workdir configuration stays bound")
			assert.NotEqual(t, baseRebuild, rebuild, "the workdir configuration stays bound")

			writeDigestFile(t, root, "main.tf", "module")
			writeDigestFile(t, dataDir, "modules/vpc/script.sh", "tampered script")
			workdir, rebuild = bothDigests(t, root, plan, dataDir)
			assert.NotEqual(t, baseWorkdir, workdir, "a downloaded module outside the workdir stays bound")
			assert.NotEqual(t, baseRebuild, rebuild, "a downloaded module outside the workdir stays bound")
		})
	}
}

// A runner-local absolute TF_DATA_DIR restores a reviewed plan when it comes from the global environment: the
// downloaded modules are bound by content, not by where they live. A stack environment override is bound
// by value when apply has no execution of its own, so changing it between runners is rejected outright
// instead of being accepted for some values and not others.
func TestRestoredPlanBindsTheDataDirOnlyThroughTheStackEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		viaStack bool
		wantErr  bool
	}{
		{name: "global environment follows the runner", viaStack: false},
		{name: "stack environment override is bound", viaStack: true, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, info, execCtx, _, path := planFixture(t)
			execCtx.preparedConfiguration.Execution = &schema.ComponentExecution{}
			setDataDir := func(dir string) {
				if test.viaStack {
					info.ComponentEnvSection = map[string]any{tfDataDirEnv: dir}
					info.ComponentEnvList = []string{tfDataDirEnv + "=" + dir}
					return
				}
				config.Env = map[string]string{tfDataDirEnv: dir}
			}
			reviewed := t.TempDir()
			writeModuleManifest(t, reviewed, "registry/vpc", "reviewed script")
			setDataDir(reviewed)
			observeFixturePlan(t, config, info, execCtx, path)
			require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out=" + path}))

			info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path
			_, err := restoreConfigurationPlan(t.Context(), config, info)
			require.NoError(t, err, "the reviewed runner restores its own plan")

			other := t.TempDir()
			writeModuleManifest(t, other, "registry/vpc", "reviewed script")
			setDataDir(other)
			_, err = restoreConfigurationPlan(t.Context(), config, info)
			if test.wantErr {
				require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
				require.ErrorContains(t, err, "reviewed stack environment overrides changed")
				return
			}
			require.NoError(t, err, "identical modules at another absolute data directory still verify")

			writeModuleManifest(t, other, "registry/vpc", "tampered script")
			_, err = restoreConfigurationPlan(t.Context(), config, info)
			require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed, "changed modules still fail")
		})
	}
}

// A workdir inside the modules directory of its own data directory is digested by the workdir walk, so the
// external walk must skip it and still bind the sibling modules beside it.
func TestWorkdirDigestBindsSiblingModulesOfAWorkdirInsideItsDataDirModules(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, modulesDirName, "unit")
	plan := filepath.Join(root, "reviewed.planfile")
	writeDigestFile(t, root, "main.tf", "module")
	writeDigestFile(t, dataDir, "modules/sibling/script.sh", "reviewed script")
	baseWorkdir, baseRebuild := bothDigests(t, root, plan, dataDir)

	writeDigestFile(t, root, "reviewed.planfile", "plan bytes")
	workdir, rebuild := bothDigests(t, root, plan, dataDir)
	assert.Equal(t, baseWorkdir, workdir, "the plan is not configuration")
	assert.Equal(t, baseRebuild, rebuild, "the plan is not configuration")

	writeDigestFile(t, root, "main.tf", "module v2")
	workdir, rebuild = bothDigests(t, root, plan, dataDir)
	assert.NotEqual(t, baseWorkdir, workdir, "the workdir configuration stays bound")
	assert.NotEqual(t, baseRebuild, rebuild, "the workdir configuration stays bound")

	writeDigestFile(t, root, "main.tf", "module")
	writeDigestFile(t, dataDir, "modules/sibling/script.sh", "tampered script")
	workdir, rebuild = bothDigests(t, root, plan, dataDir)
	assert.NotEqual(t, baseWorkdir, workdir, "a sibling module stays bound")
	assert.NotEqual(t, baseRebuild, rebuild, "a sibling module stays bound")
}

// A data directory below a directory the digest excludes by name (`.terraform/.atmos/custom`) must keep its
// downloaded modules reachable, while the bookkeeping beside it and the runtime paths stay excluded.
func TestWorkdirDigestBindsModulesOfADataDirBelowAnExcludedNamedDirectory(t *testing.T) {
	tests := []struct {
		name    string
		dataDir string
		prefix  string
		beside  string
	}{
		{name: "below .terraform/.atmos", dataDir: filepath.Join(defaultDataDir, ".atmos", "custom"), prefix: ".terraform/.atmos/custom", beside: ".terraform/.atmos/lock.json"},
		{name: "below .atmos", dataDir: filepath.Join(".atmos", "custom"), prefix: ".atmos/custom", beside: ".atmos/lock.json"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			plan := filepath.Join(root, "reviewed.planfile")
			writeDigestFile(t, root, "main.tf", "module")
			writeDigestFile(t, root, test.prefix+"/modules/vpc/script.sh", "reviewed script")
			writeDigestFile(t, root, test.beside, "bookkeeping")
			baseWorkdir, baseRebuild := bothDigests(t, root, plan, test.dataDir)

			writeDigestFile(t, root, test.beside, "newer bookkeeping")
			writeDigestFile(t, root, test.prefix+"/environment", "other-workspace")
			writeDigestFile(t, root, test.prefix+"/terraform.tfstate", "state")
			writeDigestFile(t, root, test.prefix+"/providers/registry/lock.json", "provider")
			workdir, rebuild := bothDigests(t, root, plan, test.dataDir)
			assert.Equal(t, baseWorkdir, workdir, "bookkeeping and runtime paths stay excluded")
			assert.Equal(t, baseRebuild, rebuild, "bookkeeping and runtime paths stay excluded")

			writeDigestFile(t, root, test.prefix+"/modules/vpc/script.sh", "tampered script")
			workdir, rebuild = bothDigests(t, root, plan, test.dataDir)
			assert.NotEqual(t, baseWorkdir, workdir, "a downloaded module stays bound")
			assert.NotEqual(t, baseRebuild, rebuild, "a downloaded module stays bound")
		})
	}
}

// A plan with a custom name that does not exist yet, reached through a symlinked parent, resolves like the plan Terraform
// creates there. The record beside it stays out of the digest before and after the plan appears, so the
// snapshots taken around the run compare equal.
func TestWorkdirDigestResolvesTheParentOfAPlanThatDoesNotExistYet(t *testing.T) {
	base := t.TempDir()
	workdir := filepath.Join(base, "real")
	require.NoError(t, os.Mkdir(workdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "main.tf"), []byte("module"), 0o600))
	link := filepath.Join(base, "link")
	if err := os.Symlink(workdir, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	plan := filepath.Join(link, "custom.tfplan")
	require.NoError(t, os.WriteFile(plan+configurationPlanSuffix, []byte("record"), 0o600))

	before, err := configurationWorkdirDigest(workdir, plan, "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "custom.tfplan"), []byte("plan"), 0o600))
	after, err := configurationWorkdirDigest(workdir, plan, "")
	require.NoError(t, err)
	assert.Equal(t, before, after, "the record is excluded whether or not the plan exists yet")
}

// A data directory nested in a downloaded module (TF_DATA_DIR=.terraform/modules/vpc/.atmos/data) sits below
// excluded directory names, but the files beside it are the module's own content, executed during apply. They
// stay bound, while the bookkeeping beside an excluded ancestor outside the modules stays excluded.
func TestWorkdirDigestBindsModuleContentBesideAnExcludedAncestorOfTheDataDir(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "reviewed.planfile")
	dataDir := filepath.Join(defaultDataDir, "modules", "vpc", ".atmos", "data")
	writeDigestFile(t, root, "main.tf", "module")
	writeDigestFile(t, root, ".terraform/modules/vpc/.atmos/script.sh", "reviewed script")
	baseWorkdir, baseRebuild := bothDigests(t, root, plan, dataDir)

	writeDigestFile(t, root, ".terraform/modules/vpc/.atmos/script.sh", "tampered script")
	workdir, rebuild := bothDigests(t, root, plan, dataDir)
	assert.NotEqual(t, baseWorkdir, workdir, "module content beside the data directory stays bound")
	assert.NotEqual(t, baseRebuild, rebuild, "module content beside the data directory stays bound")

	outside := filepath.Join(".atmos", "custom")
	writeDigestFile(t, root, ".atmos/lock.json", "bookkeeping")
	baseWorkdir, baseRebuild = bothDigests(t, root, plan, outside)
	writeDigestFile(t, root, ".atmos/lock.json", "newer bookkeeping")
	workdir, rebuild = bothDigests(t, root, plan, outside)
	assert.Equal(t, baseWorkdir, workdir, "bookkeeping beside an excluded ancestor stays excluded")
	assert.Equal(t, baseRebuild, rebuild, "bookkeeping beside an excluded ancestor stays excluded")
}

// A local module whose source sits beside the downloaded modules of a data directory below `.terraform`
// (TF_DATA_DIR=.terraform/custom with source = "./.terraform/custom/child") is executed during apply, so it
// stays bound. The runtime paths, the siblings, and the default layout keep their exclusions.
func TestWorkdirDigestKeepsLocalModulesBesideADataDirBelowTheDefaultCache(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "reviewed.planfile")
	writeDigestFile(t, root, "main.tf", "module")
	writeDigestFile(t, root, ".terraform/custom/child/main.tf", "reviewed child")
	writeDigestFile(t, root, ".terraform/custom/modules/vpc/script.sh", "reviewed script")
	dataDir := filepath.Join(defaultDataDir, "custom")
	baseWorkdir, baseRebuild := bothDigests(t, root, plan, dataDir)

	writeDigestFile(t, root, ".terraform/custom/environment", "other-workspace")
	writeDigestFile(t, root, ".terraform/custom/terraform.tfstate", "state")
	writeDigestFile(t, root, ".terraform/custom/plugin_path", "path")
	writeDigestFile(t, root, ".terraform/custom/providers/registry/lock.json", "provider")
	writeDigestFile(t, root, ".terraform/sibling/cache.json", "sibling cache")
	workdir, rebuild := bothDigests(t, root, plan, dataDir)
	assert.Equal(t, baseWorkdir, workdir, "runtime paths and sibling cache stay excluded")
	assert.Equal(t, baseRebuild, rebuild, "runtime paths and sibling cache stay excluded")

	writeDigestFile(t, root, ".terraform/custom/child/main.tf", "tampered child")
	workdir, rebuild = bothDigests(t, root, plan, dataDir)
	assert.NotEqual(t, baseWorkdir, workdir, "a local module beside the downloaded modules stays bound")
	assert.NotEqual(t, baseRebuild, rebuild, "a local module beside the downloaded modules stays bound")

	// With the default data directory the same files are cache and stay excluded.
	defaultWorkdir, _ := bothDigests(t, root, plan, "")
	writeDigestFile(t, root, ".terraform/custom/child/main.tf", "tampered again")
	again, _ := bothDigests(t, root, plan, "")
	assert.Equal(t, defaultWorkdir, again, "the default data directory keeps excluding its cache")
}

// A relative TF_DATA_DIR that reaches an external directory through a symlink must be resolved like the
// plan path, or the plan inside it is hashed as configuration and the same layout spelled absolutely differs.
func TestWorkdirDigestResolvesASymlinkedRelativeDataDir(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "unit")
	external := filepath.Join(parent, "real")
	require.NoError(t, os.MkdirAll(external, 0o700))
	require.NoError(t, os.MkdirAll(root, 0o700))
	if err := os.Symlink(external, filepath.Join(parent, "link")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	plan := filepath.Join(parent, "link", modulesDirName, "plans", "reviewed.plan")
	writeDigestFile(t, root, "main.tf", "module")
	writeDigestFile(t, external, "modules/vpc/script.sh", "reviewed script")
	baseWorkdir, baseRebuild := bothDigests(t, root, plan, filepath.Join("..", "link"))
	absoluteWorkdir, absoluteRebuild := bothDigests(t, root, plan, external)
	assert.Equal(t, absoluteWorkdir, baseWorkdir, "a symlinked spelling of the data directory is the same layout")
	assert.Equal(t, absoluteRebuild, baseRebuild, "a symlinked spelling of the data directory is the same layout")

	writeDigestFile(t, external, "modules/plans/reviewed.plan", "plan bytes")
	writeDigestFile(t, external, "modules/plans/reviewed.plan"+configurationPlanSuffix, "plan record")
	workdir, rebuild := bothDigests(t, root, plan, filepath.Join("..", "link"))
	assert.Equal(t, baseWorkdir, workdir, "the plan and its record are not configuration")
	assert.Equal(t, baseRebuild, rebuild, "the plan and its record are not configuration")

	writeDigestFile(t, external, "modules/vpc/script.sh", "tampered script")
	workdir, rebuild = bothDigests(t, root, plan, filepath.Join("..", "link"))
	assert.NotEqual(t, baseWorkdir, workdir, "a module behind the symlink stays bound")
	assert.NotEqual(t, baseRebuild, rebuild, "a module behind the symlink stays bound")
}

// Like every other retained file, the module manifest of an external data directory must not be a symlink,
// or its target outside the data directory would be read in place of a retained file.
func TestWorkdirDigestRejectsASymlinkedModuleManifest(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "reviewed.planfile")
	writeDigestFile(t, root, "main.tf", "module")
	dataDir := t.TempDir()
	target := filepath.Join(t.TempDir(), "elsewhere.json")
	require.NoError(t, os.WriteFile(target, []byte(`{"Modules":[]}`), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, modulesDirName), 0o700))
	if err := os.Symlink(target, filepath.Join(dataDir, modulesDirName, moduleManifestName)); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	_, err := configurationWorkdirDigest(root, plan, dataDir)
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	_, err = configurationRebuildDigest(root, plan, dataDir, nil)
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
}

func TestStalePlanSettlementFollowsTheRunOutcome(t *testing.T) {
	tests := []struct {
		name        string
		succeeded   bool
		replacement string
		want        string
	}{
		{name: "a successful run keeps the plan it wrote", succeeded: true, replacement: "fresh", want: "fresh"},
		{name: "a failed run with a partial replacement gets the earlier plan back", succeeded: false, replacement: "partial", want: "earlier"},
		{name: "a failed run that wrote nothing gets the earlier plan back", succeeded: false, replacement: "", want: "earlier"},
		{name: "a successful run that wrote nothing gets the earlier plan back", succeeded: true, replacement: "", want: "earlier"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := t.TempDir()
			plan := filepath.Join(out, "reviewed.plan")
			require.NoError(t, os.WriteFile(plan, []byte("earlier"), 0o600))
			settle, err := stashStalePlan("", "", plan)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(plan, []byte(tt.replacement), 0o600))

			require.NoError(t, settle(tt.succeeded))

			data, err := os.ReadFile(plan)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(data))
			stashes, err := filepath.Glob(filepath.Join(out, stalePlanDirPrefix+"*"))
			require.NoError(t, err)
			require.Empty(t, stashes, "the stash is removed once the plan is settled")
		})
	}
}

func TestPartialPlanFromAFailedRunRestoresTheEarlierPlan(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	snapshotter := &planSnapshotter{ctx: t.Context(), config: config, info: info, execCtx: execCtx, pathFrom: func([]string) string { return path }}
	terraformFailed := errUtils.ExitCodeError{Code: 1}
	run := func() error {
		require.NoError(t, os.WriteFile(path, []byte("partial"), 0o600))
		return terraformFailed
	}

	err := snapshotter.around([]string{"plan", "-out=" + path}, run, failedOnError)()

	require.ErrorIs(t, err, terraformFailed)
	require.NoError(t, snapshotter.err)
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, "reviewed plan", string(data), "a partial replacement never displaces the reviewed plan")
}

func TestChangesPresentStatusKeepsTheNewPlan(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	snapshotter := &planSnapshotter{ctx: t.Context(), config: config, info: info, execCtx: execCtx, pathFrom: func([]string) string { return path }}
	changesPresent := errUtils.ExitCodeError{Code: 2}
	run := func() error {
		require.NoError(t, os.WriteFile(path, []byte("new plan"), 0o600))
		return changesPresent
	}
	acceptsChangesPresent := func(err error) bool { return err != nil && errUtils.GetExitCode(err) != 2 }

	err := snapshotter.around([]string{"plan", "-detailed-exitcode", "-out=" + path}, run, acceptsChangesPresent)()

	require.ErrorIs(t, err, changesPresent)
	require.NoError(t, snapshotter.err)
	require.NotNil(t, snapshotter.result, "the accepted status still binds the plan")
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, "new plan", string(data))
	stashes, globErr := filepath.Glob(filepath.Join(filepath.Dir(path), stalePlanDirPrefix+"*"))
	require.NoError(t, globErr)
	require.Empty(t, stashes, "the earlier copy is discarded once the run succeeded")
}

func TestStalePlanBackupIsCreatedThroughTheSecuredPath(t *testing.T) {
	var created []string
	original := createSensitiveTemp
	createSensitiveTemp = func(directory string) (*os.File, string, error) {
		created = append(created, directory)
		return original(directory)
	}
	t.Cleanup(func() { createSensitiveTemp = original })

	out := t.TempDir()
	plan := filepath.Join(out, "reviewed.plan")
	require.NoError(t, os.WriteFile(plan, []byte("earlier"), 0o600))
	settle, err := stashStalePlan("", "", plan)
	require.NoError(t, err)
	require.Len(t, created, 1, "the backup is written through the restricted creation path")
	stashes, err := filepath.Glob(filepath.Join(out, stalePlanDirPrefix+"*", "plan"))
	require.NoError(t, err)
	require.Len(t, stashes, 1)
	require.Equal(t, filepath.Dir(stashes[0]), created[0])
	data, err := os.ReadFile(stashes[0])
	require.NoError(t, err)
	require.Equal(t, "earlier", string(data))

	// Recovery of a plan Terraform removed recreates it through the same path, with no stray temporary file.
	require.NoError(t, os.Remove(plan))
	require.NoError(t, settle(false))
	require.Len(t, created, 2, "a missing plan is recreated through the restricted creation path")
	data, err = os.ReadFile(plan)
	require.NoError(t, err)
	require.Equal(t, "earlier", string(data))
	leftovers, err := filepath.Glob(filepath.Join(out, ".atmos-plan-*"))
	require.NoError(t, err)
	require.Empty(t, leftovers)
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(plan)
		require.NoError(t, statErr)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}
