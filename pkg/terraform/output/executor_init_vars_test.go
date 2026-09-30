package output

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/terraform/autoinit"
	tfplugin "github.com/cloudposse/atmos/pkg/terraform/plugin"
)

// realRuleSetEnv mimics tfexec.Terraform.SetEnv: it rejects any prohibited key, exactly like the
// real library does, so a mock runner can no longer accept an environment the real one refuses.
func realRuleSetEnv(env map[string]string) error {
	if prohibited := tfexec.ProhibitedEnv(env); len(prohibited) > 0 {
		return &tfexec.ErrManualEnvVar{Name: prohibited[0]}
	}
	return nil
}

// TestExecutor_ExecuteWithSections_PassVars_RunnerAcceptsRealTfexecRule is a regression test for
// issue #3231: with init.pass_vars enabled the runner's SetEnv (which rejects TF_VAR_* in real
// tfexec) must succeed, i.e. execute() must not hand TF_VAR_* to runner.SetEnv.
func TestExecutor_ExecuteWithSections_PassVars_RunnerAcceptsRealTfexecRule(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)
	mockRunner.EXPECT().SetEnv(gomock.Any()).DoAndReturn(realRuleSetEnv).AnyTimes()
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil)
	mockRunner.EXPECT().Output(gomock.Any()).Return(nil, nil)

	var initEnv map[string]string
	exec := NewExecutor(
		mockDescriber,
		WithRunnerFactory(func(workdir, executable string) (TerraformRunner, error) { return mockRunner, nil }),
		WithInitWithVars(func(_ context.Context, req *InitWithVarsRequest) error {
			initEnv = req.Env
			return nil
		}),
	)

	atmosConfig := validAtmosConfig(t)
	atmosConfig.Components.Terraform.Init.PassVars = true
	sections := validSections()
	sections[cfg.VarsSectionName] = map[string]any{"aks_version": "9.4.1"}

	_, err := exec.ExecuteWithSections(atmosConfig, "test-component", "test-stack", sections, nil)
	require.NoError(t, err)
	assert.Equal(t, "9.4.1", initEnv["TF_VAR_aks_version"])
}

// TestBuildAutoInitInputs_TFVarExtrasReachFingerprint verifies the smart-init fingerprint still
// includes TF_VAR_* values (issue #1412) when the full environment map is passed, and that a changed
// var value changes the fingerprint.
func TestBuildAutoInitInputs_TFVarExtrasReachFingerprint(t *testing.T) {
	dir := t.TempDir()
	config := &ComponentConfig{ComponentPath: dir, PassVars: true}

	inA := buildAutoInitInputs(config, map[string]string{"TF_VAR_aks_version": "9.4.1", "PATH": "/x"})
	inB := buildAutoInitInputs(config, map[string]string{"TF_VAR_aks_version": "9.5.0", "PATH": "/x"})

	assert.Equal(t, map[string]string{"TF_VAR_aks_version": "9.4.1"}, inA.Extra)
	assert.Equal(t, map[string]string{"TF_VAR_aks_version": "9.5.0"}, inB.Extra)

	fpA, err := autoinit.Compute(inA)
	require.NoError(t, err)
	fpB, err := autoinit.Compute(inB)
	require.NoError(t, err)
	assert.NotEqual(t, fpA.Hash, fpB.Hash, "changing a TF_VAR_* value must change the init fingerprint")

	// Without pass_vars the extras are not part of the fingerprint.
	noPass := buildAutoInitInputs(&ComponentConfig{ComponentPath: dir}, map[string]string{"TF_VAR_aks_version": "9.4.1"})
	assert.Empty(t, noPass.Extra)
}

// TestSetupEnvironment_PassVarsEnvAcceptedByRealTfexecOnlyWhenFiltered is the reproducer for issue
// #3231 against the real terraform-exec library: the environment produced for pass_vars is rejected by
// tfexec.Terraform.SetEnv unless TF_VAR_* is stripped first, which is what the executor now does.
func TestSetupEnvironment_PassVarsEnvAcceptedByRealTfexecOnlyWhenFiltered(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	tf, err := tfexec.NewTerraform(t.TempDir(), exe)
	require.NoError(t, err)

	config := &ComponentConfig{
		PassVars: true,
		Vars:     map[string]any{"aks_version": "9.4.1", "tags": map[string]any{"a": "b"}},
	}
	env, err := (&defaultEnvironmentSetup{}).SetupEnvironment(config, nil)
	require.NoError(t, err)
	require.Equal(t, "9.4.1", env["TF_VAR_aks_version"])

	// Documents why the filtering exists: the unfiltered map is refused by the real library.
	var manual *tfexec.ErrManualEnvVar
	require.ErrorAs(t, tf.SetEnv(env), &manual)
	assert.True(t, strings.HasPrefix(manual.Name, "TF_VAR_"), "unexpected rejected key %q", manual.Name)
	assert.NotEmpty(t, tfexec.ProhibitedEnv(env))

	filtered := withoutTerraformVarEnv(env)
	assert.Empty(t, tfexec.ProhibitedEnv(filtered))
	require.NoError(t, tf.SetEnv(filtered))
	assert.Equal(t, env["PATH"], filtered["PATH"], "non-TF_VAR entries must be preserved")
}

func TestWithoutTerraformVarEnv(t *testing.T) {
	src := map[string]string{"TF_VAR_a": "1", "TF_VAR_": "2", "PATH": "/bin", "TF_PLUGIN_CACHE_DIR": "/c", "MY_TF_VAR_x": "3"}
	orig := map[string]string{}
	for k, v := range src {
		orig[k] = v
	}

	got := withoutTerraformVarEnv(src)
	assert.Equal(t, map[string]string{"PATH": "/bin", "TF_PLUGIN_CACHE_DIR": "/c", "MY_TF_VAR_x": "3"}, got)
	assert.Equal(t, orig, src, "input map must not be mutated")

	// Result -> source isolation.
	got["PATH"] = "changed"
	assert.Equal(t, "/bin", src["PATH"])
	// Source -> result isolation.
	src["NEW"] = "x"
	_, leaked := got["NEW"]
	assert.False(t, leaked)

	assert.Empty(t, withoutTerraformVarEnv(nil))
}

func TestBuildInitSubprocessArgs(t *testing.T) {
	tests := []struct {
		name                 string
		reconfigure, upgrade bool
		want                 []string
	}{
		{"plain", false, false, []string{"init", "-input=false", "-no-color"}},
		{"upgrade", false, true, []string{"init", "-input=false", "-no-color", "-upgrade"}},
		{"reconfigure", true, false, []string{"init", "-input=false", "-no-color", "-reconfigure"}},
		{"both", true, true, []string{"init", "-input=false", "-no-color", "-upgrade", "-reconfigure"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, buildInitSubprocessArgs(tt.reconfigure, tt.upgrade))
		})
	}
}

func TestBuildInitSubprocessEnv(t *testing.T) {
	list := buildInitSubprocessEnv(map[string]string{
		"TF_VAR_x":     "1",
		"TF_WORKSPACE": "leak",
		"TF_LOG":       "TRACE",
		"PATH":         "/bin",
	})
	assert.Contains(t, list, "TF_VAR_x=1")
	assert.Contains(t, list, "PATH=/bin")
	assert.Contains(t, list, "TF_IN_AUTOMATION=1")
	assert.Contains(t, list, "TF_LOG=", "logging must be disabled so it cannot pollute stderr")
	for _, kv := range list {
		assert.False(t, strings.HasPrefix(kv, "TF_WORKSPACE="), "TF_WORKSPACE must be removed")
	}
	assert.IsIncreasing(t, list, "environment must be rendered deterministically")
}

// helperInitEnv builds the environment for a run of the test binary acting as fake terraform.
func helperInitEnv(t *testing.T, outFile string, extra map[string]string) map[string]string {
	t.Helper()
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	env[envInitHelper] = "1"
	env[envInitHelperOut] = outFile
	for k, v := range extra {
		env[k] = v
	}
	return env
}

func readHelperObservation(t *testing.T, outFile string) initHelperObservation {
	t.Helper()
	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	var obs initHelperObservation
	require.NoError(t, json.Unmarshal(data, &obs))
	return obs
}

func TestRunInitSubprocess_PassesArgsEnvAndDir(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	tests := []struct {
		name                 string
		reconfigure, upgrade bool
		wantArgs             []string
	}{
		{"plain", false, false, []string{"init", "-input=false", "-no-color"}},
		{"upgrade", false, true, []string{"init", "-input=false", "-no-color", "-upgrade"}},
		{"reconfigure", true, false, []string{"init", "-input=false", "-no-color", "-reconfigure"}},
		{"both", true, true, []string{"init", "-input=false", "-no-color", "-upgrade", "-reconfigure"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			outFile := filepath.Join(t.TempDir(), "observed.json")
			env := helperInitEnv(t, outFile, map[string]string{"TF_VAR_x": "hello", "TF_WORKSPACE": "leak"})

			err := runInitSubprocess(context.Background(), &InitWithVarsRequest{
				Dir: dir, Executable: exe, Env: env, Reconfigure: tt.reconfigure, Upgrade: tt.upgrade,
			})
			require.NoError(t, err)

			obs := readHelperObservation(t, outFile)
			assert.Equal(t, tt.wantArgs, obs.Args)
			assert.Equal(t, "hello", obs.Env["TF_VAR_x"], "TF_VAR_* must reach the init subprocess")
			assert.Equal(t, "1", obs.Env["TF_IN_AUTOMATION"])
			_, hasWorkspace := obs.Env["TF_WORKSPACE"]
			assert.False(t, hasWorkspace)

			wantDir, err := filepath.EvalSymlinks(dir)
			require.NoError(t, err)
			gotDir, err := filepath.EvalSymlinks(obs.Cwd)
			require.NoError(t, err)
			assert.Equal(t, wantDir, gotDir, "init must run in the component directory")
		})
	}
}

func TestRunInitSubprocess_FailureCarriesStderrForClassifier(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	diagnostic := "Error: Inconsistent dependency lock file\n\nmust use terraform init -upgrade"

	tests := []struct {
		name    string
		capture *quietModeWriter
	}{
		{"without stderr capture", nil},
		{"with stderr capture", newQuietModeWriter()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outFile := filepath.Join(t.TempDir(), "observed.json")
			env := helperInitEnv(t, outFile, map[string]string{envInitHelperExit: "1", envInitHelperStderr: diagnostic})

			req := &InitWithVarsRequest{Dir: t.TempDir(), Executable: exe, Env: env}
			if tt.capture != nil {
				req.Stderr = tt.capture
			}
			err := runInitSubprocess(context.Background(), req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must use terraform init -upgrade")

			diag := autoinit.Classify(diagnosticText(err, tt.capture))
			assert.True(t, diag.UpgradeRequired, "the classifier must see the subprocess diagnostic")
			if tt.capture != nil {
				assert.Contains(t, tt.capture.String(), "must use terraform init -upgrade")
			}
		})
	}
}

func TestRunInitSubprocess_MissingExecutable(t *testing.T) {
	err := runInitSubprocess(context.Background(), &InitWithVarsRequest{
		Dir: t.TempDir(), Executable: filepath.Join(t.TempDir(), "does-not-exist"), Env: map[string]string{},
	})
	require.Error(t, err)
}

// TestRunInit_VarsInitRunnerRecoversWithUpgrade verifies the existing upgrade/reconfigure recovery in
// runInitOnce keeps working when init runs through the vars-aware subprocess path, and that
// runner.Init is never used in that mode.
func TestRunInit_VarsInitRunnerRecoversWithUpgrade(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRunner := NewMockTerraformRunner(ctrl) // No Init expectation: any runner.Init call fails the test.

	var calls []InitWithVarsRequest
	executor := &Executor{initWithVars: func(_ context.Context, req *InitWithVarsRequest) error {
		calls = append(calls, *req)
		if len(calls) == 1 {
			return errors.New("Error: Inconsistent dependency lock file\n\nmust use terraform init -upgrade")
		}
		return nil
	}}
	config := &ComponentConfig{ComponentPath: t.TempDir(), Executable: "tofu", PassVars: true, Vars: map[string]any{"a": "b"}}
	env := map[string]string{"TF_VAR_a": "b"}
	runner := executor.withVarsInit(mockRunner, config, env, nil)

	require.NoError(t, executor.runInit(context.Background(), runner, config, "c", "s", nil, tfplugin.Cache{}, false, false))

	require.Len(t, calls, 2)
	assert.False(t, calls[0].Upgrade)
	assert.True(t, calls[1].Upgrade, "the retry must add -upgrade")
	assert.False(t, calls[1].Reconfigure)
	assert.Equal(t, "b", calls[1].Env["TF_VAR_a"])
	assert.Equal(t, "tofu", calls[1].Executable)
	assert.Equal(t, config.ComponentPath, calls[1].Dir)
}

// Negative path: an init failure that carries no upgrade/reconfigure diagnostic is not retried.
func TestRunInit_VarsInitRunnerDoesNotRetryPlainFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRunner := NewMockTerraformRunner(ctrl)

	calls := 0
	executor := &Executor{initWithVars: func(context.Context, *InitWithVarsRequest) error {
		calls++
		return errors.New("permission denied")
	}}
	config := &ComponentConfig{ComponentPath: t.TempDir(), PassVars: true, Vars: map[string]any{"a": "b"}}
	runner := executor.withVarsInit(mockRunner, config, map[string]string{"TF_VAR_a": "b"}, nil)

	err := executor.runInit(context.Background(), runner, config, "c", "s", nil, tfplugin.Cache{}, false, false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrTerraformInit))
	assert.Equal(t, 1, calls)
}

func TestWithVarsInit_OnlyWrapsWhenPassVarsWithVars(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRunner := NewMockTerraformRunner(ctrl)
	executor := &Executor{}
	env := map[string]string{"TF_VAR_a": "b"}

	tests := []struct {
		name     string
		config   *ComponentConfig
		wantWrap bool
	}{
		{"pass_vars with vars", &ComponentConfig{PassVars: true, Vars: map[string]any{"a": "b"}}, true},
		{"pass_vars without vars", &ComponentConfig{PassVars: true}, false},
		{"vars without pass_vars", &ComponentConfig{Vars: map[string]any{"a": "b"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := executor.withVarsInit(mockRunner, tt.config, env, nil)
			if tt.wantWrap {
				assert.IsType(t, &varsInitRunner{}, got)
			} else {
				assert.Same(t, TerraformRunner(mockRunner), got)
			}
		})
	}
}

func TestSetRunnerEnv(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("empty env skips SetEnv", func(t *testing.T) {
		mockRunner := NewMockTerraformRunner(ctrl)
		require.NoError(t, setRunnerEnv(mockRunner, map[string]string{}))
	})

	t.Run("strips TF_VAR_ and does not mutate input", func(t *testing.T) {
		mockRunner := NewMockTerraformRunner(ctrl)
		var got map[string]string
		mockRunner.EXPECT().SetEnv(gomock.Any()).DoAndReturn(func(env map[string]string) error {
			got = env
			return realRuleSetEnv(env)
		})
		in := map[string]string{"PATH": "/bin", "TF_VAR_a": "b"}
		require.NoError(t, setRunnerEnv(mockRunner, in))
		assert.Equal(t, map[string]string{"PATH": "/bin"}, got)
		assert.Equal(t, "b", in["TF_VAR_a"])
	})

	t.Run("propagates SetEnv error", func(t *testing.T) {
		mockRunner := NewMockTerraformRunner(ctrl)
		boom := errors.New("boom")
		mockRunner.EXPECT().SetEnv(gomock.Any()).Return(boom)
		require.ErrorIs(t, setRunnerEnv(mockRunner, map[string]string{"PATH": "/bin"}), boom)
	})
}
