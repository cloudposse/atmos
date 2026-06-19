package step

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestContainerHandlerBuildConfig(t *testing.T) {
	tmpDir := t.TempDir()
	credFile := filepath.Join(tmpDir, "credentials")
	require.NoError(t, os.WriteFile(credFile, []byte("secret"), 0o600))

	handler := &ContainerHandler{}
	vars := NewVariables()
	vars.Set("image", NewStepResult("alpine:latest"))

	cfg, err := handler.buildConfig(context.Background(), &schema.WorkflowStep{
		Name:             "scan",
		Type:             "container",
		Image:            "{{ .steps.image.value }}",
		Command:          "echo $FOO",
		WorkingDirectory: tmpDir,
		Runtime:          "docker",
		Pull:             container.PullAlways,
		Cleanup:          container.CleanupOnSuccess,
		Env: map[string]string{
			"FOO":                         "bar",
			"AWS_SHARED_CREDENTIALS_FILE": credFile,
		},
		Mounts: []schema.ContainerMount{
			{Type: "bind", Source: tmpDir, Target: "/cache", ReadOnly: true},
		},
		Ports: []schema.ContainerPort{{Host: 8080, Container: 8080}},
	}, vars)

	require.NoError(t, err)
	assert.Equal(t, "alpine:latest", cfg.Image)
	assert.Equal(t, []string{defaultContainerShell, "-lc", "echo $FOO"}, cfg.Command)
	assert.Equal(t, tmpDir, cfg.WorkspaceHostPath)
	assert.Equal(t, defaultContainerWorkdir, cfg.WorkspaceFolder)
	assert.Equal(t, container.PullAlways, cfg.PullPolicy)
	assert.Equal(t, container.CleanupOnSuccess, cfg.CleanupPolicy)
	assert.Contains(t, cfg.Env, "FOO=bar")
	assert.Contains(t, cfg.Env, "AWS_SHARED_CREDENTIALS_FILE="+credFile)
	require.Len(t, cfg.Mounts, 2)
	assert.Equal(t, "/cache", cfg.Mounts[0].Target)
	assert.True(t, cfg.Mounts[0].ReadOnly)
	assert.Equal(t, credFile, cfg.Mounts[1].Source)
	assert.Equal(t, credFile, cfg.Mounts[1].Target)
	assert.True(t, cfg.Mounts[1].ReadOnly)
	require.Len(t, cfg.Ports, 1)
	assert.Equal(t, "tcp", cfg.Ports[0].Protocol)
}

// TestCredentialFileMounts pins the allowlist behavior that the original code
// lacked: the in-container env is the full host environment, so only credential
// files referenced by allowlisted variables may be bind-mounted. Path-valued
// non-credential variables (SHELL, SSH_AUTH_SOCK, …) and non-regular files
// (directories, sockets) must never be mounted — mounting SHELL=/bin/zsh or a
// 1Password agent socket broke `podman create` with "statfs … operation not
// supported".
func TestCredentialFileMounts(t *testing.T) {
	dir := t.TempDir()

	credFile := filepath.Join(dir, "credentials")
	require.NoError(t, os.WriteFile(credFile, []byte("creds"), 0o600))

	// A real regular file referenced by a NON-allowlisted var (mimics SHELL=/bin/zsh).
	shellFile := filepath.Join(dir, "zsh")
	require.NoError(t, os.WriteFile(shellFile, []byte("bin"), 0o600))

	// An allowlisted var pointing at a directory: non-regular, must be skipped
	// (the same MODE check that skips a unix socket like SSH_AUTH_SOCK).
	credDir := filepath.Join(dir, "config-dir")
	require.NoError(t, os.Mkdir(credDir, 0o700))

	env := []string{
		"AWS_SHARED_CREDENTIALS_FILE=" + credFile,           // allowlisted regular file → mounted.
		"SHELL=" + shellFile,                                // non-allowlisted regular file → skipped.
		"KUBECONFIG=" + credDir,                             // allowlisted but a directory → skipped.
		"AWS_CONFIG_FILE=" + filepath.Join(dir, "absent"),   // allowlisted but missing → skipped.
		"NOT_A_PATH=hello",                                  // not a path → skipped.
		"SSH_AUTH_SOCK=" + filepath.Join(dir, "agent.sock"), // non-allowlisted, missing socket → skipped.
	}

	mounts := credentialFileMounts(env)

	require.Len(t, mounts, 1)
	assert.Equal(t, credFile, mounts[0].Source)
	assert.Equal(t, credFile, mounts[0].Target)
	assert.Equal(t, "bind", mounts[0].Type)
	assert.True(t, mounts[0].ReadOnly)

	for _, m := range mounts {
		assert.NotEqual(t, shellFile, m.Source, "non-allowlisted SHELL file must not be mounted")
		assert.NotEqual(t, credDir, m.Source, "directory must not be mounted")
	}
}

func TestContainerHandlerActionBlocks(t *testing.T) {
	handler := &ContainerHandler{}
	vars := NewVariables()
	vars.Set("tag", NewStepResult("app:test"))

	buildCfg, err := handler.buildBuildConfig(&schema.WorkflowStep{
		Name:   "build",
		Type:   "container",
		Action: "build",
		Build: &schema.ContainerBuildStep{
			Engine:     "buildx",
			Context:    ".",
			Dockerfile: "Dockerfile",
			Tags:       []string{"{{ .steps.tag.value }}"},
			BuildArgs:  map[string]string{"VERSION": "1.0.0"},
			Target:     "runtime",
			NoCache:    true,
			Pull:       true,
			Bake: &schema.ContainerBuildBakeStep{
				File:    "docker-bake.hcl",
				Files:   []string{"docker-bake.override.hcl"},
				Target:  "{{ .steps.tag.value }}",
				Targets: []string{"worker"},
				Set:     []string{"*.tags={{ .steps.tag.value }}"},
				Vars:    map[string]string{"VERSION": "{{ .steps.tag.value }}"},
				Load:    true,
				Push:    true,
				Print:   true,
			},
		},
	}, vars)
	require.NoError(t, err)
	assert.Equal(t, ".", buildCfg.Context)
	assert.Equal(t, "buildx", buildCfg.Engine)
	assert.Equal(t, "Dockerfile", buildCfg.Dockerfile)
	assert.Equal(t, []string{"app:test"}, buildCfg.Tags)
	assert.Equal(t, map[string]string{"VERSION": "1.0.0"}, buildCfg.Args)
	assert.Equal(t, "runtime", buildCfg.Target)
	assert.True(t, buildCfg.NoCache)
	assert.True(t, buildCfg.Pull)
	require.NotNil(t, buildCfg.Bake)
	assert.Equal(t, "docker-bake.hcl", buildCfg.Bake.File)
	assert.Equal(t, []string{"docker-bake.override.hcl"}, buildCfg.Bake.Files)
	assert.Equal(t, "app:test", buildCfg.Bake.Target)
	assert.Equal(t, []string{"worker"}, buildCfg.Bake.Targets)
	assert.Equal(t, []string{"*.tags=app:test"}, buildCfg.Bake.Set)
	assert.Equal(t, map[string]string{"VERSION": "app:test"}, buildCfg.Bake.Vars)
	assert.True(t, buildCfg.Bake.Load)
	assert.True(t, buildCfg.Bake.Push)
	assert.True(t, buildCfg.Bake.Print)

	pushCfg, tags, err := handler.buildPushConfig(&schema.WorkflowStep{
		Name:   "push",
		Type:   "container",
		Action: "push",
		Push: &schema.ContainerPushStep{
			Image: "{{ .steps.tag.value }}",
			Tags:  []string{"registry.example.com/{{ .steps.tag.value }}"},
		},
	}, vars)
	require.NoError(t, err)
	assert.Equal(t, "app:test", pushCfg.Image)
	assert.Equal(t, []string{"registry.example.com/app:test"}, tags)
}

func TestContainerHandlerValidateActionBlocks(t *testing.T) {
	handler := &ContainerHandler{}

	assert.NoError(t, handler.Validate(&schema.WorkflowStep{
		Name:    "flat-run",
		Type:    "container",
		Image:   "alpine",
		Command: "echo ok",
	}))
	assert.NoError(t, handler.Validate(&schema.WorkflowStep{
		Name:   "block-run",
		Type:   "container",
		Action: "run",
		Run: &schema.ContainerRunStep{
			Image:   "alpine",
			Command: "echo ok",
		},
	}))
	assert.Error(t, handler.Validate(&schema.WorkflowStep{
		Name:   "missing-push-image",
		Type:   "container",
		Action: "push",
		Push:   &schema.ContainerPushStep{},
	}))
	assert.Error(t, handler.Validate(&schema.WorkflowStep{
		Name:   "bad-action",
		Type:   "container",
		Action: "publish",
	}))
	assert.Error(t, handler.Validate(&schema.WorkflowStep{
		Name:   "podman-bake",
		Type:   "container",
		Action: "build",
		Build: &schema.ContainerBuildStep{
			Runtime: "podman",
			Engine:  "buildx",
			Bake: &schema.ContainerBuildBakeStep{
				File: "docker-bake.hcl",
			},
		},
	}))
	assert.Error(t, handler.Validate(&schema.WorkflowStep{
		Name:   "buildx-without-docker-runtime",
		Type:   "container",
		Action: "build",
		Build: &schema.ContainerBuildStep{
			Engine: "buildx",
		},
	}))
	assert.NoError(t, handler.Validate(&schema.WorkflowStep{
		Name:   "docker-bake",
		Type:   "container",
		Action: "build",
		Build: &schema.ContainerBuildStep{
			Runtime: "docker",
			Bake: &schema.ContainerBuildBakeStep{
				File: "docker-bake.hcl",
			},
		},
	}))
	assert.Error(t, handler.Validate(&schema.WorkflowStep{
		Name:   "bad-build-engine",
		Type:   "container",
		Action: "build",
		Build: &schema.ContainerBuildStep{
			Engine: "buildkit",
		},
	}))
}
