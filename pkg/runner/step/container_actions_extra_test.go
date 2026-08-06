package step

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestBuildSpinnerMessage(t *testing.T) {
	assert.Equal(t, "Building image alpine:latest", buildSpinnerMessage("Building image", "alpine:latest"))
	assert.Equal(t, "Building image", buildSpinnerMessage("Building image", "")) // tagless bake build.
}

func TestValidateBuildAction(t *testing.T) {
	// Valid: plain build, no engine.
	require.NoError(t, validateBuildAction(&schema.WorkflowStep{
		Build: &schema.ContainerBuildStep{Context: ".", Tags: []string{"app:local"}},
	}))

	tests := []struct {
		name string
		step *schema.WorkflowStep
	}{
		{"bad runtime", &schema.WorkflowStep{Build: &schema.ContainerBuildStep{Provider: "containerd"}}},
		{"bad engine", &schema.WorkflowStep{Build: &schema.ContainerBuildStep{Engine: "kaniko"}}},
		{"driver requires buildx", &schema.WorkflowStep{Build: &schema.ContainerBuildStep{Provider: "docker", Driver: &schema.ContainerDriverConfig{Provider: "docker-container"}}}},
		{"cache requires buildx", &schema.WorkflowStep{Build: &schema.ContainerBuildStep{Provider: "docker", Cache: &schema.ContainerCacheConfig{From: []map[string]string{{"type": "registry"}}}}}},
		{"buildx requires docker", &schema.WorkflowStep{Build: &schema.ContainerBuildStep{Engine: "buildx", Provider: "podman"}}},
		{"bake requires docker", &schema.WorkflowStep{Build: &schema.ContainerBuildStep{Provider: "podman", Bake: &schema.ContainerBuildBakeStep{File: "x"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, validateBuildAction(tt.step))
		})
	}
}

func TestResolveBuildBake(t *testing.T) {
	h := &ContainerHandler{}
	vars := NewVariables()
	workDir := t.TempDir()
	step := &schema.WorkflowStep{Name: "step", WorkingDirectory: workDir}

	got, err := resolveBuildBake(h, step, vars, nil)
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = resolveBuildBake(h, step, vars, &schema.ContainerBuildBakeStep{
		File:    "docker-bake.hcl",
		Target:  "app",
		Targets: []string{"app", "test"},
		Set:     []string{"app.tags=x"},
		Vars:    map[string]string{"TAG": "v1"},
		Load:    true,
		Push:    true,
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, filepath.Join(workDir, "docker-bake.hcl"), got.File)
	assert.Equal(t, "app", got.Target)
	assert.Equal(t, []string{"app", "test"}, got.Targets)
	assert.Equal(t, "v1", got.Vars["TAG"])
	assert.True(t, got.Load)
	assert.True(t, got.Push)
}

func TestBuildConfigResolutionErrors(t *testing.T) {
	h := &ContainerHandler{}
	vars := NewVariables()

	tests := []struct {
		name string
		step *schema.WorkflowStep
	}{
		{
			name: "context",
			step: &schema.WorkflowStep{
				Name:  "build",
				Build: &schema.ContainerBuildStep{Context: "{{"},
			},
		},
		{
			name: "dockerfile",
			step: &schema.WorkflowStep{
				Name:  "build",
				Build: &schema.ContainerBuildStep{Dockerfile: "{{"},
			},
		},
		{
			name: "target",
			step: &schema.WorkflowStep{
				Name:  "build",
				Build: &schema.ContainerBuildStep{Target: "{{"},
			},
		},
		{
			name: "tags",
			step: &schema.WorkflowStep{
				Name:  "build",
				Build: &schema.ContainerBuildStep{Tags: []string{"{{"}},
			},
		},
		{
			name: "build args",
			step: &schema.WorkflowStep{
				Name:  "build",
				Build: &schema.ContainerBuildStep{BuildArgs: map[string]string{"A": "{{"}},
			},
		},
		{
			name: "driver name",
			step: &schema.WorkflowStep{Name: "build", Build: &schema.ContainerBuildStep{Driver: &schema.ContainerDriverConfig{Name: "{{"}}},
		},
		{
			name: "driver provider",
			step: &schema.WorkflowStep{Name: "build", Build: &schema.ContainerBuildStep{Driver: &schema.ContainerDriverConfig{Provider: "{{"}}},
		},
		{
			name: "driver options",
			step: &schema.WorkflowStep{Name: "build", Build: &schema.ContainerBuildStep{Driver: &schema.ContainerDriverConfig{Opts: map[string]string{"image": "{{"}}}},
		},
		{
			name: "cache from",
			step: &schema.WorkflowStep{Name: "build", Build: &schema.ContainerBuildStep{Cache: &schema.ContainerCacheConfig{From: []map[string]string{{"ref": "{{"}}}}},
		},
		{
			name: "cache to",
			step: &schema.WorkflowStep{Name: "build", Build: &schema.ContainerBuildStep{Cache: &schema.ContainerCacheConfig{To: []map[string]string{{"ref": "{{"}}}}},
		},
		{
			name: "bake file",
			step: &schema.WorkflowStep{
				Name:  "build",
				Build: &schema.ContainerBuildStep{Bake: &schema.ContainerBuildBakeStep{File: "{{"}},
			},
		},
		{
			name: "bake vars",
			step: &schema.WorkflowStep{
				Name:  "build",
				Build: &schema.ContainerBuildStep{Bake: &schema.ContainerBuildBakeStep{Vars: map[string]string{"A": "{{"}}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.buildBuildConfig(tt.step, vars)
			require.Error(t, err)
		})
	}
}

func TestBuildBuildConfigResolvesContextAndDockerfileAgainstWorkingDirectory(t *testing.T) {
	h := &ContainerHandler{}
	workDir := t.TempDir()

	t.Run("relative context and dockerfile", func(t *testing.T) {
		cfg, err := h.buildBuildConfig(&schema.WorkflowStep{
			Name:             "build",
			WorkingDirectory: workDir,
			Build:            &schema.ContainerBuildStep{Context: "docker", Dockerfile: "Dockerfile.prod"},
		}, NewVariables())
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(workDir, "docker"), cfg.Context)
		assert.Equal(t, filepath.Join(workDir, "docker", "Dockerfile.prod"), cfg.Dockerfile)
	})

	t.Run("defaults: dockerfile lands inside context, not working directory directly", func(t *testing.T) {
		cfg, err := h.buildBuildConfig(&schema.WorkflowStep{
			Name:             "build",
			WorkingDirectory: workDir,
			Build:            &schema.ContainerBuildStep{Context: "docker"},
		}, NewVariables())
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(workDir, "docker"), cfg.Context)
		assert.Equal(t, filepath.Join(workDir, "docker", "Dockerfile"), cfg.Dockerfile)
	})

	t.Run("absolute dockerfile is not re-anchored to context", func(t *testing.T) {
		absDockerfile := filepath.Join(t.TempDir(), "Dockerfile.custom")
		cfg, err := h.buildBuildConfig(&schema.WorkflowStep{
			Name:             "build",
			WorkingDirectory: workDir,
			Build:            &schema.ContainerBuildStep{Context: "docker", Dockerfile: absDockerfile},
		}, NewVariables())
		require.NoError(t, err)
		assert.Equal(t, absDockerfile, cfg.Dockerfile)
	})
}

// TestBuildBuildConfigResolvesBakeFilesAgainstWorkingDirectory reproduces a
// bug found while field-testing the working-directory fix: build.context and
// build.dockerfile were anchored to step.WorkingDirectory, but build.bake.file
// and build.bake.files were not, so a relative Buildx Bake file silently
// resolved against the Atmos process's own cwd instead of the step's
// configured (or hook-defaulted) working directory -- reproduced live by
// deleting the correct bake file entirely and observing the build still
// "succeed" against a same-named file elsewhere.
func TestBuildBuildConfigResolvesBakeFilesAgainstWorkingDirectory(t *testing.T) {
	h := &ContainerHandler{}
	workDir := t.TempDir()

	t.Run("relative bake file and files anchor to working directory", func(t *testing.T) {
		cfg, err := h.buildBuildConfig(&schema.WorkflowStep{
			Name:             "build",
			WorkingDirectory: workDir,
			Build: &schema.ContainerBuildStep{
				Provider: "docker",
				Bake: &schema.ContainerBuildBakeStep{
					File:  "docker-bake.hcl",
					Files: []string{"docker-bake.hcl", "override.hcl"},
				},
			},
		}, NewVariables())
		require.NoError(t, err)
		require.NotNil(t, cfg.Bake)
		assert.Equal(t, filepath.Join(workDir, "docker-bake.hcl"), cfg.Bake.File)
		assert.Equal(t, []string{
			filepath.Join(workDir, "docker-bake.hcl"),
			filepath.Join(workDir, "override.hcl"),
		}, cfg.Bake.Files)
	})

	t.Run("absolute bake file and files are not re-anchored", func(t *testing.T) {
		absFile := filepath.Join(t.TempDir(), "docker-bake.hcl")
		cfg, err := h.buildBuildConfig(&schema.WorkflowStep{
			Name:             "build",
			WorkingDirectory: workDir,
			Build: &schema.ContainerBuildStep{
				Provider: "docker",
				Bake: &schema.ContainerBuildBakeStep{
					File:  absFile,
					Files: []string{absFile},
				},
			},
		}, NewVariables())
		require.NoError(t, err)
		require.NotNil(t, cfg.Bake)
		assert.Equal(t, absFile, cfg.Bake.File)
		assert.Equal(t, []string{absFile}, cfg.Bake.Files)
	})
}

// TestResolveBuildCacheAnchorsLocalPaths reproduces the same working-directory
// anchoring gap for Buildx cache entries: `type: local` carries filesystem
// `src`/`dest` paths (per Buildx's own cache attribute set), which must anchor
// to step.WorkingDirectory the same way build.context does. Other cache
// types, such as registry or gha, carry refs/URLs, not paths, and must be
// left untouched.
func TestResolveBuildCacheAnchorsLocalPaths(t *testing.T) {
	h := &ContainerHandler{}
	vars := NewVariables()
	workDir := t.TempDir()
	step := &schema.WorkflowStep{Name: "build", WorkingDirectory: workDir}

	t.Run("type local src and dest anchor to working directory", func(t *testing.T) {
		cache, err := resolveBuildCache(h, step, vars, &schema.ContainerCacheConfig{
			From: []map[string]string{{"type": "local", "src": "cache-in"}},
			To:   []map[string]string{{"type": "local", "dest": "cache-out", "mode": "max"}},
		})
		require.NoError(t, err)
		require.NotNil(t, cache)
		require.Len(t, cache.From, 1)
		require.Len(t, cache.To, 1)
		assert.Equal(t, filepath.Join(workDir, "cache-in"), cache.From[0]["src"])
		assert.Equal(t, filepath.Join(workDir, "cache-out"), cache.To[0]["dest"])
		assert.Equal(t, "max", cache.To[0]["mode"])
	})

	t.Run("non-local types are left untouched", func(t *testing.T) {
		cache, err := resolveBuildCache(h, step, vars, &schema.ContainerCacheConfig{
			From: []map[string]string{{"type": "registry", "ref": "registry.example.com/app:buildcache"}},
		})
		require.NoError(t, err)
		require.NotNil(t, cache)
		require.Len(t, cache.From, 1)
		assert.Equal(t, "registry.example.com/app:buildcache", cache.From[0]["ref"])
	})
}

func TestRunConfigResolutionErrors(t *testing.T) {
	h := &ContainerHandler{}
	vars := NewVariables()

	_, err := h.resolveRunCommand(context.Background(), &schema.WorkflowStep{Name: "s"}, vars, &schema.ContainerRunStep{Command: "{{"})
	require.Error(t, err)

	_, err = h.resolveRunCommand(context.Background(), &schema.WorkflowStep{Name: "s", Command: "{{"}, vars, &schema.ContainerRunStep{})
	require.Error(t, err)

	_, err = resolveRunBasics(vars, &schema.ContainerRunStep{Image: "{{"}, "s")
	require.Error(t, err)

	_, err = resolveContainerEnv(vars, &schema.WorkflowStep{Name: "s", Env: map[string]string{"A": "{{"}})
	require.Error(t, err)

	_, err = convertContainerMounts(vars, []schema.ContainerMount{{Source: "{{", Target: "/target"}})
	require.Error(t, err)

	_, err = convertContainerMounts(vars, []schema.ContainerMount{{Source: "/source", Target: "{{"}})
	require.Error(t, err)
}

func TestDefaultMountType(t *testing.T) {
	assert.Equal(t, "bind", defaultMountType(""))
	assert.Equal(t, "volume", defaultMountType("volume"))
}

func TestEffectiveBuildStep(t *testing.T) {
	assert.Equal(t, schema.ContainerBuildStep{}, effectiveBuildStep(&schema.WorkflowStep{}))
	build := &schema.ContainerBuildStep{Context: ".", Tags: []string{"app:local"}}
	assert.Equal(t, *build, effectiveBuildStep(&schema.WorkflowStep{Build: build}))
}

func TestValidateInspectAction(t *testing.T) {
	h := &ContainerHandler{}

	// Valid via the inspect block.
	require.NoError(t, h.validateInspectAction(&schema.WorkflowStep{Inspect: &schema.ContainerInspectStep{Image: "alpine"}}))

	// Missing image.
	require.Error(t, h.validateInspectAction(&schema.WorkflowStep{}))

	// Bad runtime.
	require.Error(t, h.validateInspectAction(&schema.WorkflowStep{
		Inspect: &schema.ContainerInspectStep{Image: "alpine", Provider: "containerd"},
	}))
}

func TestExecuteInspect_DryRun(t *testing.T) {
	h := &ContainerHandler{}
	vars := NewVariables()

	res, err := h.executeInspect(context.Background(), &schema.WorkflowStep{
		Name:    "inspect",
		Inspect: &schema.ContainerInspectStep{Image: "alpine:latest"},
		DryRun:  true,
	}, vars)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "alpine:latest", res.Metadata["image"])
}

func TestResolveWorkDir(t *testing.T) {
	vars := NewVariables()

	// Explicit working_directory is resolved and made absolute.
	got, err := resolveWorkDir(vars, &schema.WorkflowStep{WorkingDirectory: "subdir"})
	require.NoError(t, err)
	assert.True(t, len(got) > 0)

	// Empty falls back to the current working directory (absolute).
	got, err = resolveWorkDir(vars, &schema.WorkflowStep{})
	require.NoError(t, err)
	assert.NotEmpty(t, got)
}

func TestEffectiveRunStepMergesShorthand(t *testing.T) {
	// Run parameters live under `with:` (step.Run) and are returned as-is.
	run := effectiveRunStep(&schema.WorkflowStep{
		Run: &schema.ContainerRunStep{
			Image:   "alpine",
			Command: "echo hi",
			Shell:   "/bin/bash",
			Mounts:  []schema.ContainerMount{{Source: "/h", Target: "/c"}},
		},
	})
	assert.Equal(t, "alpine", run.Image)
	assert.Equal(t, "echo hi", run.Command)
	assert.Equal(t, "/bin/bash", run.Shell)
	require.Len(t, run.Mounts, 1)
	assert.Equal(t, schema.ContainerMount{Source: "/h", Target: "/c"}, run.Mounts[0])

	// The run block's image is preserved.
	run = effectiveRunStep(&schema.WorkflowStep{
		Run: &schema.ContainerRunStep{Image: "explicit"},
	})
	assert.Equal(t, "explicit", run.Image)
}

func TestEffectiveRunStepHostRuntime(t *testing.T) {
	// Inline step-level runtime.host folds in.
	run := effectiveRunStep(&schema.WorkflowStep{
		Runtime: &schema.ContainerRuntimeConfig{Host: true},
	})
	assert.True(t, runtimeHost(run.Runtime))

	// run.runtime.host folds in.
	run = effectiveRunStep(&schema.WorkflowStep{
		Run: &schema.ContainerRunStep{Runtime: &schema.ContainerRuntimeConfig{Host: true}},
	})
	assert.True(t, runtimeHost(run.Runtime))

	// Merging the inline host must not mutate the shared run.runtime pointer.
	stepRuntime := &schema.ContainerRuntimeConfig{}
	step := &schema.WorkflowStep{
		Runtime: &schema.ContainerRuntimeConfig{Host: true},
		Run:     &schema.ContainerRunStep{Runtime: stepRuntime},
	}
	run = effectiveRunStep(step)
	assert.True(t, runtimeHost(run.Runtime))
	assert.False(t, stepRuntime.Host, "the original run.runtime must stay unmodified")

	// No runtime block → no host access.
	run = effectiveRunStep(&schema.WorkflowStep{})
	assert.False(t, runtimeHost(run.Runtime))
}

func TestConvertContainerPorts(t *testing.T) {
	ports := convertContainerPorts([]schema.ContainerPort{
		{Host: 8080, Container: 80},
		{Host: 53, Container: 53, Protocol: "udp"},
	})
	require.Len(t, ports, 2)
	assert.Equal(t, "tcp", ports[0].Protocol)
	assert.Equal(t, "udp", ports[1].Protocol)
}

func TestEffectiveInspectStepRuntimeShorthand(t *testing.T) {
	got := effectiveInspectStep(&schema.WorkflowStep{Inspect: &schema.ContainerInspectStep{Image: "alpine"}, Provider: "podman", RuntimeAutoStart: true})
	assert.Equal(t, "alpine", got.Image)
	assert.Equal(t, "podman", got.Provider)
	assert.True(t, got.RuntimeAutoStart)

	// Ensure the BakeConfig type is referenced so a field rename fails the build.
	_ = container.BakeConfig{}
}

// TestEffectiveBuildStepProviderFallthrough guards the fix for the bake-build-run
// example: provider/runtime_auto_start are top-level cross-cutting modifiers that
// must fall through into the build/push config (bake requires `provider: docker`).
func TestEffectiveBuildStepProviderFallthrough(t *testing.T) {
	build := effectiveBuildStep(&schema.WorkflowStep{
		Build:            &schema.ContainerBuildStep{Engine: "buildx", Bake: &schema.ContainerBuildBakeStep{File: "docker-bake.hcl"}},
		Provider:         "docker",
		RuntimeAutoStart: true,
	})
	assert.Equal(t, "docker", build.Provider)
	assert.True(t, build.RuntimeAutoStart)

	// An explicit provider under `with:` still wins over the top-level fallthrough.
	build = effectiveBuildStep(&schema.WorkflowStep{
		Build:    &schema.ContainerBuildStep{Provider: "podman"},
		Provider: "docker",
	})
	assert.Equal(t, "podman", build.Provider)

	push := effectivePushStep(&schema.WorkflowStep{
		Push:     &schema.ContainerPushStep{Image: "app:local"},
		Provider: "docker",
	})
	assert.Equal(t, "docker", push.Provider)
}
