package helmfile

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractTargetFlag(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantTarget string
		wantArgs   []string
	}{
		{
			name:       "no target",
			args:       []string{"--state-values-file", "vars.yaml", "template"},
			wantTarget: "",
			wantArgs:   []string{"--state-values-file", "vars.yaml", "template"},
		},
		{
			name:       "space form",
			args:       []string{"template", "--target", "deployment-repo", "--skip-deps"},
			wantTarget: "deployment-repo",
			wantArgs:   []string{"template", "--skip-deps"},
		},
		{
			name:       "equals form",
			args:       []string{"--target=git-repo", "template"},
			wantTarget: "git-repo",
			wantArgs:   []string{"template"},
		},
		{
			name:       "trailing target without value",
			args:       []string{"template", "--target"},
			wantTarget: "",
			wantArgs:   []string{"template"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, args := ExtractTargetFlag(tt.args)
			assert.Equal(t, tt.wantTarget, target)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

type fakeProvisioner struct {
	input *target.DeliverInput
	err   error
}

func (f *fakeProvisioner) Deliver(_ context.Context, in *target.DeliverInput) error {
	f.input = in
	return f.err
}

func writeHelmfileProgram(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	name := "helmfile"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	sourcePath := filepath.Join(dir, "main.go")
	source := "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nfunc main() {\n\t_ = os.Args\n" + body + "\n}\n"
	require.NoError(t, os.WriteFile(sourcePath, []byte(source), 0o600))
	cmd := exec.Command("go", "build", "-o", path, sourcePath)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return path
}

func TestRenderAndDeliverExternalTarget(t *testing.T) {
	provisioner := &fakeProvisioner{}
	target.Register("test-helmfile", provisioner)

	cmd := writeHelmfileProgram(t, `fmt.Print("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: app\n  namespace: demo\ndata:\n  key: value\n")`)

	rendered, err := RenderAndDeliver(context.Background(), &RenderDeliverInput{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info:        &schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "dev"},
		Command:     cmd,
		Args:        []string{"template"},
		WorkingDir:  t.TempDir(),
		ProvisionSection: map[string]any{
			"default": "repo",
			"targets": map[string]any{
				"repo": map[string]any{"kind": "test-helmfile", "path": "clusters/dev"},
			},
		},
	})
	require.NoError(t, err)
	assert.Contains(t, rendered, "kind: ConfigMap")
	require.NotNil(t, provisioner.input)
	assert.Equal(t, "repo", provisioner.input.TargetName)
	assert.Equal(t, "app", provisioner.input.Artifact.Metadata.Component)
	assert.Equal(t, "dev", provisioner.input.Artifact.Metadata.Stack)
	assert.NotEmpty(t, provisioner.input.Artifact.Files)
}

func TestRenderAndDeliverErrors(t *testing.T) {
	t.Run("unknown target", func(t *testing.T) {
		_, err := RenderAndDeliver(context.Background(), &RenderDeliverInput{
			AtmosConfig:      &schema.AtmosConfiguration{},
			Info:             &schema.ConfigAndStacksInfo{},
			Command:          writeHelmfileProgram(t, `fmt.Print("ok\n")`),
			WorkingDir:       t.TempDir(),
			ProvisionSection: map[string]any{"default": "missing"},
		})
		assert.ErrorIs(t, err, errUtils.ErrProvisionTargetNotFound)
	})

	t.Run("command failure includes stderr", func(t *testing.T) {
		cmd := writeHelmfileProgram(t, `fmt.Fprint(os.Stderr, "bad\n")
	os.Exit(7)`)
		rendered, err := RenderAndDeliver(context.Background(), &RenderDeliverInput{
			AtmosConfig: &schema.AtmosConfiguration{},
			Info:        &schema.ConfigAndStacksInfo{},
			Command:     cmd,
			WorkingDir:  t.TempDir(),
		})
		assert.Empty(t, rendered)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "helmfile template failed")
		assert.Contains(t, err.Error(), "bad")
	})

	t.Run("target deliver error", func(t *testing.T) {
		sentinel := errors.New("deliver failed")
		provisioner := &fakeProvisioner{err: sentinel}
		target.Register("test-helmfile-error", provisioner)
		cmd := writeHelmfileProgram(t, `fmt.Print("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: app\n")`)

		_, err := RenderAndDeliver(context.Background(), &RenderDeliverInput{
			AtmosConfig: &schema.AtmosConfiguration{},
			Info:        &schema.ConfigAndStacksInfo{},
			Command:     cmd,
			Args:        []string{"template"},
			WorkingDir:  t.TempDir(),
			ProvisionSection: map[string]any{
				"default": "repo",
				"targets": map[string]any{
					"repo": map[string]any{"kind": "test-helmfile-error"},
				},
			},
		})
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestCaptureCommandEnvironmentAndWorkingDir(t *testing.T) {
	dir := t.TempDir()
	cmd := writeHelmfileProgram(t, `wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s:%s", os.Getenv("CUSTOM_ENV"), wd)`)
	out, err := captureCommand(context.Background(), &RenderDeliverInput{
		Command:    cmd,
		WorkingDir: dir,
		EnvVars:    []string{"CUSTOM_ENV=value"},
	})
	require.NoError(t, err)
	expectedDir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.Equal(t, "value:"+expectedDir, out)
}
