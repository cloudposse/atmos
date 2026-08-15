package step

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests/testhelpers"
)

func installStepFakeDocker(t *testing.T) {
	t.Helper()
	testhelpers.InstallFakeContainerRuntime(t, testhelpers.FakeContainerRuntimeSpec{
		Name: string(container.TypeDocker),
		Mode: testhelpers.FakeContainerRuntimeStep,
	})
}

func installFullFakeDocker(t *testing.T) {
	t.Helper()
	testhelpers.InstallFakeContainerRuntime(t, testhelpers.FakeContainerRuntimeSpec{
		Name: string(container.TypeDocker),
		Mode: testhelpers.FakeContainerRuntimeFull,
	})
}

func fakeRuntimeArgs(t *testing.T, path string) []string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(content)), "\n")
}

func TestContainerHandlerExecuteRunWithFakeDocker(t *testing.T) {
	installStepFakeDocker(t)
	h := &ContainerHandler{}

	res, err := h.executeRun(context.Background(), &schema.WorkflowStep{
		Name: "run",
		Run: &schema.ContainerRunStep{
			Image:    "alpine",
			Command:  "echo hi",
			Provider: string(container.TypeDocker),
		},
	}, NewVariables(), &schema.WorkflowDefinition{Output: "none"})

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "run stdout\n", res.Metadata["stdout"])
	assert.Equal(t, 0, res.Metadata[exitCodeMetadata])
	assert.Equal(t, "container-id", res.Metadata["container_id"])
}

func TestContainerHandlerExecuteRunAttachesSharedNetwork(t *testing.T) {
	installStepFakeDocker(t)
	t.Setenv("ATMOS_EMULATOR_USE_CURRENT_CONTAINER_NETWORK", "false")
	argsPath := filepath.Join(t.TempDir(), "docker-args.log")
	t.Setenv("ATMOS_FAKE_RUNTIME_ARGS_FILE", argsPath)

	h := &ContainerHandler{}
	res, err := h.executeRun(context.Background(), &schema.WorkflowStep{
		Name:  "smoke",
		Stack: "dev",
		Run: &schema.ContainerRunStep{
			Image:    "alpine",
			Command:  "echo hi",
			Provider: string(container.TypeDocker),
		},
	}, NewVariables(), &schema.WorkflowDefinition{Output: "none"})
	require.NoError(t, err)
	require.NotNil(t, res)

	args := fakeRuntimeArgs(t, argsPath)
	assert.Contains(t, args, "network\tcreate\tatmos-dev")

	var createLine string
	for _, line := range args {
		if strings.HasPrefix(line, "create\t") {
			createLine = line
			break
		}
	}
	require.NotEmpty(t, createLine, "expected a create invocation")
	assert.Contains(t, createLine, "--network\tatmos-dev")
	assert.Contains(t, createLine, "--network-alias\tdev-smoke")
}

func TestContainerHandlerExecuteRunHostNetworkOverrideSkipsSharedNetwork(t *testing.T) {
	installStepFakeDocker(t)
	t.Setenv("ATMOS_EMULATOR_USE_CURRENT_CONTAINER_NETWORK", "false")
	argsPath := filepath.Join(t.TempDir(), "docker-args.log")
	t.Setenv("ATMOS_FAKE_RUNTIME_ARGS_FILE", argsPath)

	h := &ContainerHandler{}
	res, err := h.executeRun(context.Background(), &schema.WorkflowStep{
		Name:  "smoke",
		Stack: "dev",
		Run: &schema.ContainerRunStep{
			Image:    "alpine",
			Command:  "echo hi",
			Provider: string(container.TypeDocker),
			RunArgs:  []string{"--network=host"},
		},
	}, NewVariables(), &schema.WorkflowDefinition{Output: "none"})
	require.NoError(t, err)
	require.NotNil(t, res)

	args := fakeRuntimeArgs(t, argsPath)
	for _, line := range args {
		assert.False(t, strings.HasPrefix(line, "network\t"), "explicit --network override must not also get the shared network: %q", line)
		assert.NotContains(t, line, "--network\tatmos-dev", "explicit --network override must not also get the shared network alias")
	}

	var createLine string
	for _, line := range args {
		if strings.HasPrefix(line, "create\t") {
			createLine = line
			break
		}
	}
	require.NotEmpty(t, createLine, "expected a create invocation")
	assert.Contains(t, createLine, "--network=host", "the user's explicit run_args override must still be applied")
}

func TestContainerHandlerExecuteRunNoStackSkipsSharedNetwork(t *testing.T) {
	installStepFakeDocker(t)
	t.Setenv("ATMOS_EMULATOR_USE_CURRENT_CONTAINER_NETWORK", "false")
	argsPath := filepath.Join(t.TempDir(), "docker-args.log")
	t.Setenv("ATMOS_FAKE_RUNTIME_ARGS_FILE", argsPath)

	h := &ContainerHandler{}
	res, err := h.executeRun(context.Background(), &schema.WorkflowStep{
		Name: "smoke",
		Run: &schema.ContainerRunStep{
			Image:    "alpine",
			Command:  "echo hi",
			Provider: string(container.TypeDocker),
		},
	}, NewVariables(), &schema.WorkflowDefinition{Output: "none"})
	require.NoError(t, err)
	require.NotNil(t, res)

	args := fakeRuntimeArgs(t, argsPath)
	assert.NotContains(t, args, "network\tcreate\tatmos-default", "no stack in scope should mean no network attachment")
	for _, line := range args {
		assert.False(t, strings.HasPrefix(line, "network\t"), "no stack in scope should mean no network attachment: %q", line)
	}
}

func TestContainerHandlerExecuteBuildWithFakeDocker(t *testing.T) {
	installStepFakeDocker(t)
	h := &ContainerHandler{}

	res, err := h.executeBuild(context.Background(), &schema.WorkflowStep{
		Name: "build",
		Build: &schema.ContainerBuildStep{
			Provider: string(container.TypeDocker),
			Context:  ".",
			Tags:     []string{"app:local"},
		},
	}, NewVariables())

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "app:local", res.Metadata["image"])
	assert.Equal(t, "sha256:built", res.Metadata["image_id"])
	assert.Equal(t, []string{"app:local"}, res.Metadata["repo_tags"])
}

func TestContainerHandlerExecuteBuildPassesBuildxDriverAndCacheToDocker(t *testing.T) {
	installStepFakeDocker(t)
	argsPath := filepath.Join(t.TempDir(), "docker-args.log")
	t.Setenv("ATMOS_FAKE_RUNTIME_ARGS_FILE", argsPath)

	vars := NewVariables()
	vars.Set("builder", NewStepResult("atmos-test-builder"))
	vars.Set("cache", NewStepResult("registry.example.com/app:buildcache"))
	h := &ContainerHandler{}

	_, err := h.executeBuild(context.Background(), &schema.WorkflowStep{
		Name: "build",
		Build: &schema.ContainerBuildStep{
			Provider: string(container.TypeDocker),
			Engine:   "buildx",
			Context:  ".",
			Tags:     []string{"app:local"},
			Driver: &schema.ContainerDriverConfig{
				Name:     "{{ .steps.builder.value }}",
				Provider: "docker-container",
				Opts:     map[string]string{"image": "mirror.gcr.io/moby/buildkit:buildx-stable-1"},
			},
			Cache: &schema.ContainerCacheConfig{
				From: []map[string]string{{"type": "registry", "ref": "{{ .steps.cache.value }}"}},
				To:   []map[string]string{{"type": "registry", "ref": "{{ .steps.cache.value }}", "mode": "max"}},
			},
		},
	}, vars)
	require.NoError(t, err)

	cwd, err := os.Getwd()
	require.NoError(t, err)

	args := fakeRuntimeArgs(t, argsPath)
	assert.Contains(t, args, "buildx\tcreate\t--name\tatmos-test-builder\t--driver\tdocker-container\t--driver-opt\timage=mirror.gcr.io/moby/buildkit:buildx-stable-1")
	assert.Contains(t, args, "buildx\tbuild\t--builder\tatmos-test-builder\t--cache-from\tref=registry.example.com/app:buildcache,type=registry\t--cache-to\tmode=max,ref=registry.example.com/app:buildcache,type=registry\t-t\tapp:local\t-f\t"+filepath.Join(cwd, "Dockerfile")+"\t"+cwd)
}

func TestContainerHandlerExecutePushPassesResolvedTagsAndRuntimeEnvToDocker(t *testing.T) {
	installFullFakeDocker(t)
	argsPath := filepath.Join(t.TempDir(), "docker-args.log")
	t.Setenv("ATMOS_FAKE_RUNTIME_ARGS_FILE", argsPath)

	vars := NewVariables()
	vars.Env["ATMOS_FAKE_AUTH"] = "present"
	vars.Set("image", NewStepResult("app:local"))
	h := &ContainerHandler{}

	_, err := h.executePush(context.Background(), &schema.WorkflowStep{
		Name: "push",
		Push: &schema.ContainerPushStep{
			Provider: string(container.TypeDocker),
			Image:    "{{ .steps.image.value }}",
			Tags:     []string{"registry.example.com/{{ .steps.image.value }}"},
		},
	}, vars)
	require.NoError(t, err)

	args := fakeRuntimeArgs(t, argsPath)
	assert.Contains(t, args, "tag\tapp:local\tregistry.example.com/app:local")
	assert.Contains(t, args, "push\tregistry.example.com/app:local")
}

func TestContainerHandlerExecuteBuildWritesCISummaryWhenEnabled(t *testing.T) {
	installStepFakeDocker(t)
	h := &ContainerHandler{}
	var summaries []string
	prev := writeStepSummaryFn
	writeStepSummaryFn = func(content string) error {
		summaries = append(summaries, content)
		return nil
	}
	defer func() { writeStepSummaryFn = prev }()
	vars := NewVariables()
	vars.SetAtmosConfig(&schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}})

	_, err := h.executeBuild(context.Background(), &schema.WorkflowStep{
		Name: "build",
		Build: &schema.ContainerBuildStep{
			Provider: string(container.TypeDocker),
			Context:  ".",
			Tags:     []string{"app:local"},
		},
	}, vars)

	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Contains(t, summaries[0], "## 🐳 app:local")
	assert.Contains(t, summaries[0], "| Digest | `sha256:built` |")
}

func TestContainerHandlerExecuteBuildSkipsCISummaryWhenDisabled(t *testing.T) {
	installStepFakeDocker(t)
	h := &ContainerHandler{}
	called := false
	prev := writeStepSummaryFn
	writeStepSummaryFn = func(string) error {
		called = true
		return nil
	}
	defer func() { writeStepSummaryFn = prev }()
	disabled := false
	vars := NewVariables()
	vars.SetAtmosConfig(&schema.AtmosConfiguration{
		CI: schema.CIConfig{
			Enabled: true,
			Summary: schema.CISummaryConfig{
				Enabled: &disabled,
			},
		},
	})

	_, err := h.executeBuild(context.Background(), &schema.WorkflowStep{
		Name: "build",
		Build: &schema.ContainerBuildStep{
			Provider: string(container.TypeDocker),
			Context:  ".",
			Tags:     []string{"app:local"},
		},
	}, vars)

	require.NoError(t, err)
	assert.False(t, called)
}

func TestWriteContainerImageSummarySkipsWhenCIUnavailable(t *testing.T) {
	prev := writeStepSummaryFn
	called := false
	writeStepSummaryFn = func(string) error {
		called = true
		return nil
	}
	defer func() { writeStepSummaryFn = prev }()

	writeContainerImageSummary(nil, &container.ImageInfo{RepoTags: []string{"app:local"}}, container.ImageSummaryOptions{Image: "app:local"})
	writeContainerImageSummary(&schema.AtmosConfiguration{}, &container.ImageInfo{RepoTags: []string{"app:local"}}, container.ImageSummaryOptions{Image: "app:local"})
	writeContainerImageSummary(&schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}, nil, container.ImageSummaryOptions{Image: "app:local"})

	assert.False(t, called)
}

func TestWriteContainerImageSummaryIgnoresWriteError(t *testing.T) {
	prev := writeStepSummaryFn
	writeStepSummaryFn = func(string) error {
		return assert.AnError
	}
	defer func() { writeStepSummaryFn = prev }()

	assert.NotPanics(t, func() {
		writeContainerImageSummary(
			&schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}},
			&container.ImageInfo{RepoTags: []string{"app:local"}},
			container.ImageSummaryOptions{Image: "app:local"},
		)
	})
}

func TestWritePushedImageSummariesSkipsInvalidAndInspectFailures(t *testing.T) {
	var summaries []string
	prev := writeStepSummaryFn
	writeStepSummaryFn = func(content string) error {
		summaries = append(summaries, content)
		return nil
	}
	defer func() { writeStepSummaryFn = prev }()
	runtime := &pushRuntime{
		imageInfos: map[string]*container.ImageInfo{
			"registry.example.com/app:ok": {
				ID:       "sha256:img",
				RepoTags: []string{"registry.example.com/app:ok"},
			},
		},
		inspectErrs: map[string]error{
			"registry.example.com/app:missing": assert.AnError,
		},
	}

	writePushedImageSummaries(context.Background(), runtime, &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}, []*container.PushResult{
		nil,
		{},
		{Image: "registry.example.com/app:missing", Digest: "sha256:missing"},
		{Image: "registry.example.com/app:ok", Digest: "sha256:ok"},
	})

	require.Len(t, summaries, 1)
	assert.Contains(t, summaries[0], "## 🐳 registry.example.com/app:ok")
	assert.Contains(t, summaries[0], "| Digest | `sha256:ok` |")
}

func TestContainerHandlerExecuteInspectWithFakeDocker(t *testing.T) {
	installStepFakeDocker(t)
	h := &ContainerHandler{}

	res, err := h.executeInspect(context.Background(), &schema.WorkflowStep{
		Name: "inspect",
		Inspect: &schema.ContainerInspectStep{
			Image:    "app:local",
			Provider: string(container.TypeDocker),
		},
	}, NewVariables())

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "app:local", res.Metadata["image"])
	assert.Equal(t, "sha256:built", res.Metadata["image_id"])
	assert.Equal(t, int64(1024), res.Metadata["size"])
}
