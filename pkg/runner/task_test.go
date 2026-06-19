package runner

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTasks_UnmarshalYAML_SimpleStrings(t *testing.T) {
	input := `
- "echo hello"
- "echo world"
- ls -la
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	assert.Len(t, tasks, 3)

	assert.Equal(t, "echo hello", tasks[0].Command)
	assert.Equal(t, "shell", tasks[0].Type)
	assert.Empty(t, tasks[0].Name)

	assert.Equal(t, "echo world", tasks[1].Command)
	assert.Equal(t, "shell", tasks[1].Type)

	assert.Equal(t, "ls -la", tasks[2].Command)
	assert.Equal(t, "shell", tasks[2].Type)
}

func TestTasks_UnmarshalYAML_StructuredSyntax(t *testing.T) {
	input := `
- name: validate
  command: terraform validate
  type: shell
  timeout: 30s
- name: plan
  command: terraform plan vpc
  type: atmos
  stack: dev-us-east-1
  timeout: 5m
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	assert.Len(t, tasks, 2)

	assert.Equal(t, "validate", tasks[0].Name)
	assert.Equal(t, "terraform validate", tasks[0].Command)
	assert.Equal(t, "shell", tasks[0].Type)
	assert.Equal(t, 30*time.Second, tasks[0].Timeout)

	assert.Equal(t, "plan", tasks[1].Name)
	assert.Equal(t, "terraform plan vpc", tasks[1].Command)
	assert.Equal(t, "atmos", tasks[1].Type)
	assert.Equal(t, "dev-us-east-1", tasks[1].Stack)
	assert.Equal(t, 5*time.Minute, tasks[1].Timeout)
}

func TestTasks_UnmarshalYAML_MixedSyntax(t *testing.T) {
	input := `
- "echo simple string"
- name: structured
  command: echo with timeout
  timeout: 10s
- another simple command
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	assert.Len(t, tasks, 3)

	// First: simple string.
	assert.Equal(t, "echo simple string", tasks[0].Command)
	assert.Equal(t, "shell", tasks[0].Type)
	assert.Empty(t, tasks[0].Name)
	assert.Zero(t, tasks[0].Timeout)

	// Second: structured.
	assert.Equal(t, "structured", tasks[1].Name)
	assert.Equal(t, "echo with timeout", tasks[1].Command)
	assert.Equal(t, "shell", tasks[1].Type) // defaults to shell.
	assert.Equal(t, 10*time.Second, tasks[1].Timeout)

	// Third: simple string.
	assert.Equal(t, "another simple command", tasks[2].Command)
	assert.Equal(t, "shell", tasks[2].Type)
}

func TestTasks_UnmarshalYAML_DefaultsTypeToShell(t *testing.T) {
	input := `
- name: no-type-specified
  command: echo hello
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	assert.Len(t, tasks, 1)
	assert.Equal(t, "shell", tasks[0].Type)
}

func TestTasks_UnmarshalYAML_WithRetry(t *testing.T) {
	input := `
- name: flaky-task
  command: curl http://example.com
  retry:
    max_attempts: 3
    initial_delay: 1s
    max_delay: 10s
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	assert.Len(t, tasks, 1)
	require.NotNil(t, tasks[0].Retry)
	require.NotNil(t, tasks[0].Retry.MaxAttempts)
	assert.Equal(t, 3, *tasks[0].Retry.MaxAttempts)
	require.NotNil(t, tasks[0].Retry.InitialDelay)
	assert.Equal(t, time.Second, *tasks[0].Retry.InitialDelay)
	require.NotNil(t, tasks[0].Retry.MaxDelay)
	assert.Equal(t, 10*time.Second, *tasks[0].Retry.MaxDelay)
}

func TestTasks_UnmarshalYAML_WithWorkingDirectory(t *testing.T) {
	input := `
- name: build
  command: make build
  working_directory: /app/src
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	assert.Len(t, tasks, 1)
	assert.Equal(t, "/app/src", tasks[0].WorkingDirectory)
}

func TestTasks_UnmarshalYAML_WithIdentity(t *testing.T) {
	input := `
- name: deploy
  command: terraform apply
  type: atmos
  identity: production-deployer
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	assert.Len(t, tasks, 1)
	assert.Equal(t, "production-deployer", tasks[0].Identity)
}

func TestTasks_UnmarshalYAML_WithContainerFields(t *testing.T) {
	input := `
- name: scan
  type: container
  image: alpine:latest
  command: echo hello
  shell: /bin/sh
  provider: docker
  pull: always
  workspace: /workspace
  workspace_read_only: true
  cleanup: on_success
  user: "1000:1000"
  run_args:
    - --network=none
  mounts:
    - type: bind
      source: ./cache
      target: /cache
      read_only: true
  ports:
    - host: 8080
      container: 8080
      protocol: tcp
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	require.Len(t, tasks, 1)
	task := tasks[0]
	assert.Equal(t, "container", task.Type)
	assert.Equal(t, "alpine:latest", task.Image)
	assert.Equal(t, "/bin/sh", task.Shell)
	assert.Equal(t, "docker", task.Provider)
	assert.Equal(t, "always", task.Pull)
	assert.Equal(t, "/workspace", task.Workspace)
	assert.True(t, task.WorkspaceReadOnly)
	assert.Equal(t, "on_success", task.Cleanup)
	assert.Equal(t, "1000:1000", task.User)
	assert.Equal(t, []string{"--network=none"}, task.RunArgs)
	require.Len(t, task.Mounts, 1)
	assert.Equal(t, "./cache", task.Mounts[0].Source)
	assert.Equal(t, "/cache", task.Mounts[0].Target)
	assert.True(t, task.Mounts[0].ReadOnly)
	require.Len(t, task.Ports, 1)
	assert.Equal(t, 8080, task.Ports[0].Host)
	assert.Equal(t, 8080, task.Ports[0].Container)
	assert.Equal(t, "tcp", task.Ports[0].Protocol)
}

func TestTasks_UnmarshalYAML_WithContainerActionBlocksAndOutputs(t *testing.T) {
	input := `
- name: build
  type: container
  action: build
  build:
    provider: docker
    runtime_auto_start: true
    engine: buildx
    context: .
    dockerfile: Dockerfile
    tags:
      - app:local
    build_args:
      VERSION: "1.0.0"
    target: runtime
    no_cache: true
    pull: true
    bake:
      file: docker-bake.hcl
      files:
        - docker-bake.override.hcl
      target: app
      targets:
        - worker
      set:
        - "*.platform=linux/amd64"
      vars:
        VERSION: "1.0.0"
      load: true
      push: true
      print: true
  outputs:
    image: "{{ .metadata.image }}"
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	require.Len(t, tasks, 1)
	task := tasks[0]
	assert.Equal(t, "container", task.Type)
	assert.Equal(t, "build", task.Action)
	require.NotNil(t, task.Build)
	assert.Equal(t, "docker", task.Build.Provider)
	assert.True(t, task.Build.RuntimeAutoStart)
	assert.Equal(t, "buildx", task.Build.Engine)
	assert.Equal(t, ".", task.Build.Context)
	assert.Equal(t, "Dockerfile", task.Build.Dockerfile)
	assert.Equal(t, []string{"app:local"}, task.Build.Tags)
	assert.Equal(t, map[string]string{"VERSION": "1.0.0"}, task.Build.BuildArgs)
	assert.Equal(t, "runtime", task.Build.Target)
	assert.True(t, task.Build.NoCache)
	assert.True(t, task.Build.Pull)
	require.NotNil(t, task.Build.Bake)
	assert.Equal(t, "docker-bake.hcl", task.Build.Bake.File)
	assert.Equal(t, []string{"docker-bake.override.hcl"}, task.Build.Bake.Files)
	assert.Equal(t, "app", task.Build.Bake.Target)
	assert.Equal(t, []string{"worker"}, task.Build.Bake.Targets)
	assert.Equal(t, []string{"*.platform=linux/amd64"}, task.Build.Bake.Set)
	assert.Equal(t, map[string]string{"VERSION": "1.0.0"}, task.Build.Bake.Vars)
	assert.True(t, task.Build.Bake.Load)
	assert.True(t, task.Build.Bake.Push)
	assert.True(t, task.Build.Bake.Print)
	assert.Equal(t, map[string]string{"image": "{{ .metadata.image }}"}, task.Outputs)
}

func TestTasks_UnmarshalYAML_EmptyList(t *testing.T) {
	input := `[]`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	assert.Len(t, tasks, 0)
}

func TestTasks_UnmarshalYAML_InvalidNotSequence(t *testing.T) {
	input := `command: echo hello`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.Error(t, err)
	assert.ErrorIs(t, err, schema.ErrTaskInvalidFormat)
}

func TestTasks_UnmarshalYAML_InvalidNestedSequence(t *testing.T) {
	input := `
- - nested
  - sequence
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.Error(t, err)
	assert.ErrorIs(t, err, schema.ErrTaskUnexpectedNodeKind)
}

func TestTask_ToWorkflowStep(t *testing.T) {
	maxAttempts := 3
	task := Task{
		Name:             "test-task",
		Command:          "echo hello",
		Type:             "shell",
		Stack:            "dev",
		WorkingDirectory: "/app",
		Identity:         "test-identity",
		Retry: &schema.RetryConfig{
			MaxAttempts: &maxAttempts,
		},
		Timeout:           30 * time.Second,
		Image:             "alpine:latest",
		Shell:             "/bin/sh",
		Provider:          "docker",
		Pull:              "always",
		Workspace:         "/workspace",
		WorkspaceReadOnly: true,
		Cleanup:           "on_success",
		User:              "1000:1000",
		RunArgs:           []string{"--network=none"},
		Mounts:            []schema.ContainerMount{{Type: "bind", Source: "./cache", Target: "/cache", ReadOnly: true}},
		Ports:             []schema.ContainerPort{{Host: 8080, Container: 8080, Protocol: "tcp"}},
		Action:            "run",
		Build: &schema.ContainerBuildStep{
			Engine:  "buildx",
			Context: ".",
			Tags:    []string{"app:local"},
			Bake: &schema.ContainerBuildBakeStep{
				File:   "docker-bake.hcl",
				Target: "app",
			},
		},
		Push: &schema.ContainerPushStep{
			Image: "app:local",
			Tags:  []string{"registry.example.com/app:local"},
		},
		Run: &schema.ContainerRunStep{
			Image:   "app:local",
			Command: "echo ok",
		},
		Outputs: map[string]string{"image": "{{ .metadata.image }}"},
	}

	step := task.ToWorkflowStep()

	assert.Equal(t, task.Name, step.Name)
	assert.Equal(t, task.Command, step.Command)
	assert.Equal(t, task.Type, step.Type)
	assert.Equal(t, task.Stack, step.Stack)
	assert.Equal(t, task.WorkingDirectory, step.WorkingDirectory)
	assert.Equal(t, task.Identity, step.Identity)
	assert.Equal(t, task.Retry, step.Retry)
	assert.Equal(t, task.Image, step.Image)
	assert.Equal(t, task.Shell, step.Shell)
	assert.Equal(t, task.Provider, step.Provider)
	assert.Equal(t, task.Pull, step.Pull)
	assert.Equal(t, task.Workspace, step.Workspace)
	assert.Equal(t, task.WorkspaceReadOnly, step.WorkspaceReadOnly)
	assert.Equal(t, task.Cleanup, step.Cleanup)
	assert.Equal(t, task.User, step.User)
	assert.Equal(t, task.RunArgs, step.RunArgs)
	assert.Equal(t, task.Mounts, step.Mounts)
	assert.Equal(t, task.Ports, step.Ports)
	assert.Equal(t, task.Action, step.Action)
	assert.Equal(t, task.Build, step.Build)
	assert.Equal(t, task.Push, step.Push)
	assert.Equal(t, task.Run, step.Run)
	assert.Equal(t, task.Outputs, step.Outputs)
	// Note: Timeout is not in WorkflowStep.
}

func TestTaskFromWorkflowStep(t *testing.T) {
	maxAttempts := 5
	step := schema.WorkflowStep{
		Name:             "workflow-step",
		Command:          "terraform apply",
		Type:             "atmos",
		Stack:            "prod",
		WorkingDirectory: "/infra",
		Identity:         "prod-identity",
		Retry: &schema.RetryConfig{
			MaxAttempts: &maxAttempts,
		},
		Image:             "alpine:latest",
		Shell:             "/bin/sh",
		Provider:          "docker",
		Pull:              "always",
		Workspace:         "/workspace",
		WorkspaceReadOnly: true,
		Cleanup:           "on_success",
		User:              "1000:1000",
		RunArgs:           []string{"--network=none"},
		Mounts:            []schema.ContainerMount{{Type: "bind", Source: "./cache", Target: "/cache", ReadOnly: true}},
		Ports:             []schema.ContainerPort{{Host: 8080, Container: 8080, Protocol: "tcp"}},
		Action:            "run",
		Build: &schema.ContainerBuildStep{
			Engine:  "buildx",
			Context: ".",
			Tags:    []string{"app:local"},
			Bake: &schema.ContainerBuildBakeStep{
				File:   "docker-bake.hcl",
				Target: "app",
			},
		},
		Push: &schema.ContainerPushStep{
			Image: "app:local",
			Tags:  []string{"registry.example.com/app:local"},
		},
		Run: &schema.ContainerRunStep{
			Image:   "app:local",
			Command: "echo ok",
		},
		Outputs: map[string]string{"image": "{{ .metadata.image }}"},
	}

	task := schema.TaskFromWorkflowStep(&step)

	assert.Equal(t, step.Name, task.Name)
	assert.Equal(t, step.Command, task.Command)
	assert.Equal(t, step.Type, task.Type)
	assert.Equal(t, step.Stack, task.Stack)
	assert.Equal(t, step.WorkingDirectory, task.WorkingDirectory)
	assert.Equal(t, step.Identity, task.Identity)
	assert.Equal(t, step.Retry, task.Retry)
	assert.Equal(t, step.Image, task.Image)
	assert.Equal(t, step.Shell, task.Shell)
	assert.Equal(t, step.Provider, task.Provider)
	assert.Equal(t, step.Pull, task.Pull)
	assert.Equal(t, step.Workspace, task.Workspace)
	assert.Equal(t, step.WorkspaceReadOnly, task.WorkspaceReadOnly)
	assert.Equal(t, step.Cleanup, task.Cleanup)
	assert.Equal(t, step.User, task.User)
	assert.Equal(t, step.RunArgs, task.RunArgs)
	assert.Equal(t, step.Mounts, task.Mounts)
	assert.Equal(t, step.Ports, task.Ports)
	assert.Equal(t, step.Action, task.Action)
	assert.Equal(t, step.Build, task.Build)
	assert.Equal(t, step.Push, task.Push)
	assert.Equal(t, step.Run, task.Run)
	assert.Equal(t, step.Outputs, task.Outputs)
	assert.Zero(t, task.Timeout) // WorkflowStep doesn't have Timeout.
}
