package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

func boolPtr(b bool) *bool { return &b }

func TestStepContainerDisabledAndOverride(t *testing.T) {
	none := &schema.WorkflowStep{}
	enabled := &schema.WorkflowStep{Container: &schema.WorkflowContainer{Image: "alpine"}}
	disabled := &schema.WorkflowStep{Container: &schema.WorkflowContainer{Enabled: boolPtr(false)}}

	assert.False(t, StepContainerDisabled(none))
	assert.False(t, StepContainerOverride(none))

	assert.True(t, StepContainerOverride(enabled))
	assert.False(t, StepContainerDisabled(enabled))

	assert.True(t, StepContainerDisabled(disabled))
	assert.False(t, StepContainerOverride(disabled))
}

func TestBuildContainerConfig(t *testing.T) {
	params := &ContainerStepParams{
		Workflow:     "deploy",
		WorkflowPath: "workflows/deploy.yaml",
		RuntimeEnv:   []string{"FOO=bar"},
		DryRun:       true,
	}
	cfg := &schema.WorkflowContainer{
		Image:             "alpine:latest",
		Provider:          "podman",
		Pull:              container.PullAlways,
		Cleanup:           container.CleanupOnSuccess,
		Workspace:         "/work",
		WorkspaceReadOnly: true,
		User:              "1000:1000",
		RunArgs:           []string{"--cap-drop=ALL"},
		Env:               map[string]string{"A": "1"},
		Mounts:            []schema.ContainerMount{{Source: "/h", Target: "/c"}},
		Ports:             []schema.ContainerPort{{Host: 8080, Container: 80}},
	}

	got, err := buildContainerConfig(params, cfg, "/repo")
	require.NoError(t, err)
	assert.Equal(t, "alpine:latest", got.Image)
	assert.Equal(t, "podman", got.RuntimeName)
	assert.Equal(t, "/work", got.WorkspaceFolder)
	assert.True(t, got.WorkspaceReadOnly)
	assert.Equal(t, container.PullAlways, got.PullPolicy)
	assert.Equal(t, container.CleanupOnSuccess, got.CleanupPolicy)
	assert.Equal(t, []string{"FOO=bar"}, got.RuntimeEnv)
	assert.Equal(t, []string{"A=1"}, got.Env)
	assert.True(t, got.DryRun)
	require.Len(t, got.Mounts, 1)
	require.Len(t, got.Ports, 1)
}

func TestBuildContainerConfigValidationErrors(t *testing.T) {
	params := &ContainerStepParams{Workflow: "w", WorkflowPath: "p"}
	tests := []struct {
		name string
		cfg  *schema.WorkflowContainer
	}{
		{"missing image", &schema.WorkflowContainer{}},
		{"bad runtime", &schema.WorkflowContainer{Image: "x", Provider: "containerd"}},
		{"bad pull", &schema.WorkflowContainer{Image: "x", Pull: "sometimes"}},
		{"bad cleanup", &schema.WorkflowContainer{Image: "x", Cleanup: "maybe"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildContainerConfig(params, tt.cfg, "/repo")
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)
		})
	}
}

func TestMergeWorkflowContainer_NilCases(t *testing.T) {
	base := &schema.WorkflowContainer{Image: "base"}
	override := &schema.WorkflowContainer{Image: "override"}

	assert.Equal(t, base, mergeWorkflowContainer(base, nil))
	assert.Equal(t, override, mergeWorkflowContainer(nil, override))
}

func TestMergeWorkflowContainer_ScalarsCollectionsAndToggles(t *testing.T) {
	base := &schema.WorkflowContainer{
		Image:    "base",
		Shell:    "/bin/sh",
		Provider: "docker",
		Env:      map[string]string{"BASE": "1"},
		Mounts:   []schema.ContainerMount{{Source: "/base"}},
	}
	override := &schema.WorkflowContainer{
		Image:             "override",
		User:              "root",
		Pull:              container.PullAlways,
		Cleanup:           container.CleanupNever,
		Workspace:         "/w",
		RuntimeAutoStart:  true,
		WorkspaceReadOnly: true,
		Enabled:           boolPtr(true),
		Env:               map[string]string{"OVERRIDE": "2"},
		RunArgs:           []string{"--rm"},
		Ports:             []schema.ContainerPort{{Host: 1}},
	}

	merged := mergeWorkflowContainer(base, override)

	// Scalars: override wins where set, base kept otherwise.
	assert.Equal(t, "override", merged.Image)
	assert.Equal(t, "/bin/sh", merged.Shell) // base kept (override empty).
	assert.Equal(t, "docker", merged.Provider)
	assert.Equal(t, "root", merged.User)
	assert.Equal(t, container.PullAlways, merged.Pull)
	// Toggles OR together.
	assert.True(t, merged.RuntimeAutoStart)
	assert.True(t, merged.WorkspaceReadOnly)
	// Collections: override replaces when non-empty.
	assert.Equal(t, map[string]string{"OVERRIDE": "2"}, merged.Env)
	assert.Equal(t, []schema.ContainerMount{{Source: "/base"}}, merged.Mounts) // base kept (override empty).
	assert.Equal(t, []string{"--rm"}, merged.RunArgs)

	// Isolation: mutating merged must not affect base.
	merged.Image = "mutated"
	assert.Equal(t, "base", base.Image)
}

func TestMapHostWorkDirToContainer(t *testing.T) {
	workspace := filepath.Join(string(filepath.Separator)+"repo", "root")
	sub := filepath.Join(workspace, "services", "api")

	tests := []struct {
		name       string
		hostDir    string
		wantResult string
		wantErr    bool
	}{
		{"empty maps to workspace root", "", "/workspace", false},
		{"dot maps to workspace root", ".", "/workspace", false},
		{"subdir maps under container workspace", sub, "/workspace/services/api", false},
		{"same dir maps to workspace root", workspace, "/workspace", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapHostWorkDirToContainer(tt.hostDir, workspace, "/workspace")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantResult, got)
		})
	}
}

func TestMapHostWorkDirToContainer_OutsideWorkspaceErrors(t *testing.T) {
	workspace := filepath.Join(string(filepath.Separator)+"repo", "root")
	outside := filepath.Join(string(filepath.Separator)+"somewhere", "else")

	_, err := mapHostWorkDirToContainer(outside, workspace, "/workspace")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside workflow container workspace")
}

func TestConvertWorkflowMounts(t *testing.T) {
	mounts := convertWorkflowMounts([]schema.ContainerMount{
		{Source: "/h", Target: "/c"},
		{Type: "volume", Source: "vol", Target: "/data", ReadOnly: true},
	})
	require.Len(t, mounts, 2)
	assert.Equal(t, "bind", mounts[0].Type) // defaulted.
	assert.Equal(t, "/h", mounts[0].Source)
	assert.Equal(t, "volume", mounts[1].Type)
	assert.True(t, mounts[1].ReadOnly)
}

func TestConvertWorkflowPorts(t *testing.T) {
	ports := convertWorkflowPorts([]schema.ContainerPort{
		{Host: 8080, Container: 80},
		{Host: 53, Container: 53, Protocol: "udp"},
	})
	require.Len(t, ports, 2)
	assert.Equal(t, "tcp", ports[0].Protocol) // defaulted.
	assert.Equal(t, 8080, ports[0].HostPort)
	assert.Equal(t, "udp", ports[1].Protocol)
}

func TestEnvMapToSlice(t *testing.T) {
	assert.Nil(t, envMapToSlice(nil))
	assert.Equal(t, []string{"A=1", "B=2", "C=3"}, envMapToSlice(map[string]string{"C": "3", "A": "1", "B": "2"}))
}

func TestMergeEnvSlices(t *testing.T) {
	assert.Equal(t, []string{"A=1"}, mergeEnvSlices(nil, []string{"A=1"}))
	assert.Equal(t, []string{"A=1"}, mergeEnvSlices([]string{"A=1"}, nil))

	// Overlay wins on collisions; first-seen order preserved; malformed entry skipped.
	got := mergeEnvSlices([]string{"A=1", "B=2"}, []string{"B=override", "C=3", "malformed"})
	assert.Equal(t, []string{"A=1", "B=override", "C=3"}, got)
}

func TestExpandHome(t *testing.T) {
	assert.Equal(t, "", expandHome(""))
	assert.Equal(t, "/abs/path", expandHome("/abs/path"))
	assert.Equal(t, "relative", expandHome("relative"))

	home := expandHome("~")
	assert.NotEqual(t, "~", home)
	assert.Equal(t, filepath.Join(home, "sub"), expandHome("~/sub"))
}

func TestDefaultStringAndRuntimePreviewName(t *testing.T) {
	assert.Equal(t, "fallback", defaultString("", "fallback"))
	assert.Equal(t, "value", defaultString("value", "fallback"))
	assert.Equal(t, "docker|podman", runtimePreviewName(""))
	assert.Equal(t, "podman", runtimePreviewName("podman"))
}

func TestValidRuntimePullCleanup(t *testing.T) {
	assert.True(t, validRuntime(""))
	assert.True(t, validRuntime(string(container.TypeDocker)))
	assert.True(t, validRuntime(string(container.TypePodman)))
	assert.False(t, validRuntime("containerd"))

	assert.True(t, validPull(""))
	assert.True(t, validPull(container.PullMissing))
	assert.False(t, validPull("sometimes"))

	assert.True(t, validCleanup(""))
	assert.True(t, validCleanup(container.CleanupOnSuccess))
	assert.False(t, validCleanup("maybe"))
}

func TestResolveStepHostWorkspace(t *testing.T) {
	// Explicit HostWorkDir is made absolute.
	rel := "some-rel-dir"
	got, err := resolveStepHostWorkspace(&ContainerStepParams{HostWorkDir: rel})
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(got))
	assert.True(t, strings.HasSuffix(filepath.ToSlash(got), "some-rel-dir"))

	// Unset falls back to the workflow host workspace (cwd here).
	got, err = resolveStepHostWorkspace(&ContainerStepParams{WorkflowDef: &schema.WorkflowDefinition{}})
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(got))
}

func TestBuildEphemeralStepConfig(t *testing.T) {
	params := &ContainerStepParams{
		Workflow:     "deploy",
		WorkflowPath: "workflows/deploy.yaml",
		Command:      "echo hi",
		StepEnv:      []string{"STEP=1"},
		Step:         &schema.WorkflowStep{Name: "build", Tty: true},
	}
	cfg := &schema.WorkflowContainer{
		Image:   "alpine",
		Shell:   "/bin/bash",
		User:    "root",
		Pull:    container.PullMissing,
		Cleanup: container.CleanupAlways,
		Env:     map[string]string{"C": "9"},
	}

	got := buildEphemeralStepConfig(params, cfg, "/abs/work")
	assert.Equal(t, "atmos-step-build", got.Name)
	assert.Equal(t, "alpine", got.Image)
	assert.Equal(t, []string{"/bin/bash", "-lc", "echo hi"}, got.Command)
	assert.Equal(t, "/abs/work", got.WorkspaceHostPath)
	assert.Equal(t, "/workspace", got.WorkspaceFolder) // defaulted.
	assert.Equal(t, "root", got.User)
	assert.True(t, got.TTY)
	assert.Contains(t, got.Env, "C=9")
	assert.Contains(t, got.Env, "STEP=1")
	assert.Equal(t, container.SandboxTypeWorkflow, got.Labels[container.SandboxLabelType])
	assert.Equal(t, "deploy", got.Labels[container.SandboxLabelWorkflow])
}

func TestWriteEphemeralResult(t *testing.T) {
	params := &ContainerStepParams{
		Step:        &schema.WorkflowStep{Name: "s"},
		WorkflowDef: &schema.WorkflowDefinition{Output: "none"},
	}
	// nil result is a no-op (must not panic).
	assert.NotPanics(t, func() { writeEphemeralResult(params, nil, nil) })
	// A populated result renders without panicking.
	assert.NotPanics(t, func() {
		writeEphemeralResult(params, &container.EphemeralResult{Stdout: "out", Stderr: "err"}, nil)
	})
	// TTY steps skip rendering.
	ttyParams := &ContainerStepParams{
		Step:        &schema.WorkflowStep{Name: "s", Tty: true},
		WorkflowDef: &schema.WorkflowDefinition{Output: "none"},
	}
	assert.NotPanics(t, func() {
		writeEphemeralResult(ttyParams, &container.EphemeralResult{Stdout: "x"}, nil)
	})
}

func TestContainerSessionCleanup(t *testing.T) {
	// nil session and nil backend are safe no-ops.
	var nilSession *ContainerSession
	require.NoError(t, nilSession.Cleanup(true))
	require.NoError(t, (&ContainerSession{}).Cleanup(true))

	fake := &fakeContainer{id: "id", name: "name"}
	session := &ContainerSession{backend: fake}
	require.NoError(t, session.Cleanup(true))
}

func TestStartWorkflowContainer_GuardsAndDisabled(t *testing.T) {
	_, err := StartWorkflowContainer(context.Background(), nil)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)

	// No container config -> nil session, nil error.
	session, err := StartWorkflowContainer(context.Background(), &ContainerStepParams{
		WorkflowDef: &schema.WorkflowDefinition{},
	})
	require.NoError(t, err)
	assert.Nil(t, session)

	// Explicitly disabled container -> nil session, nil error.
	session, err = StartWorkflowContainer(context.Background(), &ContainerStepParams{
		WorkflowDef: &schema.WorkflowDefinition{Container: &schema.WorkflowContainer{Enabled: boolPtr(false)}},
	})
	require.NoError(t, err)
	assert.Nil(t, session)
}

func TestStartWorkflowContainer_DryRun(t *testing.T) {
	// A dry-run config short-circuits the real runtime (container.StartSandbox
	// returns a dry-run sandbox), so the happy path is exercised end-to-end.
	session, err := StartWorkflowContainer(context.Background(), &ContainerStepParams{
		Workflow:     "deploy",
		WorkflowPath: "workflows/deploy.yaml",
		WorkflowDef: &schema.WorkflowDefinition{
			Container: &schema.WorkflowContainer{Image: "alpine:latest"},
		},
		DryRun: true,
	})
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.NotNil(t, session.backend)
}

func TestRunStepContainerOverride_GuardsDisabledAndDryRun(t *testing.T) {
	// nil params / missing required fields -> typed error, no panic.
	assert.ErrorIs(t, RunStepContainerOverride(context.Background(), nil), errUtils.ErrNilParam)
	assert.ErrorIs(
		t,
		RunStepContainerOverride(context.Background(), &ContainerStepParams{Step: &schema.WorkflowStep{}}),
		errUtils.ErrNilParam,
	)

	// Disabled merged container -> no-op success.
	err := RunStepContainerOverride(context.Background(), &ContainerStepParams{
		WorkflowDef: &schema.WorkflowDefinition{},
		Step:        &schema.WorkflowStep{Container: &schema.WorkflowContainer{Enabled: boolPtr(false)}},
	})
	require.NoError(t, err)

	// Enabled but missing image -> typed error.
	err = RunStepContainerOverride(context.Background(), &ContainerStepParams{
		WorkflowDef: &schema.WorkflowDefinition{Container: &schema.WorkflowContainer{Enabled: boolPtr(true)}},
		Step:        &schema.WorkflowStep{Name: "s"},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)

	// Dry-run with a valid image short-circuits before any real runtime call.
	err = RunStepContainerOverride(context.Background(), &ContainerStepParams{
		Workflow:     "deploy",
		WorkflowPath: "workflows/deploy.yaml",
		WorkflowDef:  &schema.WorkflowDefinition{Container: &schema.WorkflowContainer{Image: "alpine"}},
		Step:         &schema.WorkflowStep{Name: "s"},
		DryRun:       true,
	})
	require.NoError(t, err)
}

func TestExecShell_NilGuards(t *testing.T) {
	var nilSession *ContainerSession
	assert.ErrorIs(t, nilSession.ExecShell(context.Background(), &ContainerStepParams{}), errUtils.ErrNilParam)

	session := &ContainerSession{backend: &fakeContainer{id: "id", name: "name"}, config: &schema.WorkflowContainer{}}
	assert.ErrorIs(t, session.ExecShell(context.Background(), nil), errUtils.ErrNilParam)
	assert.ErrorIs(
		t,
		session.ExecShell(context.Background(), &ContainerStepParams{Step: &schema.WorkflowStep{}}),
		errUtils.ErrNilParam,
	)
}

func TestExecShell_DryRunWhenIDEqualsName(t *testing.T) {
	// A dry-run backend reports ID == Name, so ExecShell previews and returns
	// without invoking Exec.
	fake := &fakeContainer{id: "same", name: "same"}
	session := &ContainerSession{
		backend:       fake,
		config:        &schema.WorkflowContainer{Workspace: "/workspace"},
		hostWorkspace: "/repo",
	}
	err := session.ExecShell(context.Background(), &ContainerStepParams{
		Step:        &schema.WorkflowStep{Name: "s"},
		WorkflowDef: &schema.WorkflowDefinition{Output: "none"},
		Command:     "pwd",
	})
	require.NoError(t, err)
	assert.Nil(t, fake.command) // Exec not called.
}

func TestExecutor_CleanupWorkflowContainer(t *testing.T) {
	// nil session is a no-op.
	require.NoError(t, (&Executor{}).cleanupWorkflowContainer(true))

	// With a session, Cleanup runs and the reference is cleared (idempotent teardown).
	e := &Executor{containerSession: &ContainerSession{backend: &fakeContainer{id: "id", name: "name"}}}
	require.NoError(t, e.cleanupWorkflowContainer(true))
	assert.Nil(t, e.containerSession)
}

func TestExecutor_EnsureWorkflowContainerReturnsCached(t *testing.T) {
	existing := &ContainerSession{backend: &fakeContainer{id: "id", name: "name"}}
	e := &Executor{containerSession: existing}

	got, err := e.ensureWorkflowContainer(&WorkflowParams{Ctx: context.Background()}, nil)
	require.NoError(t, err)
	assert.Same(t, existing, got)
}

func TestExecutor_RunShellStepHostPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	runner := NewMockCommandRunner(ctrl)
	runner.EXPECT().RunShell("echo hi", "wf-step-0", "/wd", []string{"E=1"}, false).Return(nil)

	e := &Executor{runner: runner}
	params := &WorkflowParams{
		Ctx:                context.Background(),
		Workflow:           "wf",
		WorkflowDefinition: &schema.WorkflowDefinition{}, // no workflow-level container.
	}
	step := &schema.WorkflowStep{Name: "s"} // no step container override.
	cmdParams := &runCommandParams{command: "echo hi", stepEnv: []string{"E=1"}, workingDirectory: "/wd"}

	require.NoError(t, e.runShellStep(params, step, cmdParams, "/wd"))
}

func TestEnvSliceToMap(t *testing.T) {
	assert.Nil(t, envSliceToMap(nil))
	assert.Equal(
		t,
		map[string]string{"A": "1", "B": "2"},
		envSliceToMap([]string{"A=1", "B=2", "malformed"}), // malformed (no '=') is skipped.
	)
}

func TestExecutor_RunShellStep_ContainerOverrideDryRun(t *testing.T) {
	e := &Executor{}
	params := &WorkflowParams{
		Ctx:                context.Background(),
		AtmosConfig:        &schema.AtmosConfiguration{},
		WorkflowDefinition: &schema.WorkflowDefinition{},
		Opts:               ExecuteOptions{DryRun: true},
	}
	// A step-level container override routes to RunStepContainerOverride; dry-run
	// short-circuits before any real runtime.
	step := &schema.WorkflowStep{Name: "s", Container: &schema.WorkflowContainer{Image: "alpine"}}
	cmdParams := &runCommandParams{command: "echo hi", workingDirectory: "."}

	require.NoError(t, e.runShellStep(params, step, cmdParams, "."))
}

func TestExecutor_RunShellStep_WorkflowContainerExec(t *testing.T) {
	// A cached dry-run session (backend ID == Name) makes ExecShell preview and
	// return without invoking the runtime.
	e := &Executor{containerSession: &ContainerSession{
		backend:       &fakeContainer{id: "same", name: "same"},
		config:        &schema.WorkflowContainer{Workspace: "/workspace"},
		hostWorkspace: "/repo",
	}}
	params := &WorkflowParams{
		Ctx:         context.Background(),
		AtmosConfig: &schema.AtmosConfiguration{},
		WorkflowDefinition: &schema.WorkflowDefinition{
			Container: &schema.WorkflowContainer{Image: "alpine"},
			Output:    "none",
		},
	}
	step := &schema.WorkflowStep{Name: "s"} // not container-disabled.
	cmdParams := &runCommandParams{command: "pwd", workingDirectory: "."}

	require.NoError(t, e.runShellStep(params, step, cmdParams, "."))
}
