package stack

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/charmbracelet/huh"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	project "github.com/cloudposse/atmos/pkg/project/config"
)

func scriptedCreationForm(t *testing.T, input string) {
	t.Helper()
	previous := runCreationForm
	runCreationForm = func(ctx context.Context, form *huh.Form) error {
		return form.WithAccessible(true).WithInput(iotest.OneByteReader(strings.NewReader(input))).WithOutput(io.Discard).RunWithContext(ctx)
	}
	t.Cleanup(func() { runCreationForm = previous })
}

func TestCreationDestinationPrompt(t *testing.T) {
	for _, tc := range []struct{ name, input, file, target string }{
		{"single file", "1\nstacks/dev.yaml\n", "stacks/dev.yaml", ""},
		{"directory", "2\nstacks/dev\n", "", "stacks/dev"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scriptedCreationForm(t, tc.input)
			file, target, err := promptCreationDestination(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tc.file, file)
			assert.Equal(t, tc.target, target)
		})
	}
}

func TestCreationDestinationCancellation(t *testing.T) {
	previous := runCreationForm
	runCreationForm = func(context.Context, *huh.Form) error { return huh.ErrUserAborted }
	t.Cleanup(func() { runCreationForm = previous })
	_, _, err := promptCreationDestination(context.Background())
	require.ErrorIs(t, err, errUtils.ErrUserAborted)
}

func TestCreationDestinationPromptPolicy(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		interactive, defaults, tty bool
	}{
		{"defaults", true, true, true},
		{"disabled", false, false, true},
		{"nonterminal", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := viper.Get("force-tty")
			viper.Set("force-tty", tc.tty)
			t.Cleanup(func() { viper.Set("force-tty", previous) })
			v := viper.New()
			v.Set("interactive", tc.interactive)
			v.Set("defaults", tc.defaults)
			_, _, err := creationDestination(context.Background(), v)
			require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
			assert.Contains(t, err.Error(), "--file or --target")
		})
	}
}

func TestCreateInteractiveDryRunCollectsAnswersOnce(t *testing.T) {
	output := createProject(t)
	previous := viper.Get("force-tty")
	viper.Set("force-tty", true)
	t.Cleanup(func() { viper.Set("force-tty", previous) })
	scriptedCreationForm(t, "2\nstacks/dev\n")
	writeCreateRecipe(t, `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: prompted
spec:
  fields:
    - name: environment
      type: input
      required: true
    - name: optional
      type: input
      default: preset
    - name: inactive
      type: input
      required: true
      when: "answers.environment == 'prod'"
  files:
    - path: stack.yaml
`)
	require.NoError(t, os.WriteFile("recipe/stack.yaml", []byte("# atmos:template\nvars:\n  stage: {{ .Config.environment | quote }}\n  label: {{ .Config.optional | quote }}\n"), 0o644))
	calls := 0
	restore := project.SetFormRunnerForTest(func(form *huh.Form) error {
		calls++
		return form.WithAccessible(true).WithInput(iotest.OneByteReader(strings.NewReader("dev\n\n"))).WithOutput(io.Discard).Run()
	})
	t.Cleanup(restore)
	require.NoError(t, runCreate(t, "--template", "./recipe", "--set", "optional=supplied", "--dry-run"))
	assert.Equal(t, 1, calls)
	assert.Contains(t, output.String(), `stage: "dev"`)
	assert.Contains(t, output.String(), `label: "supplied"`)
	assert.Contains(t, output.String(), "stacks/dev/stack.yaml")
	assert.NoDirExists(t, "stacks/dev")
	assert.NoDirExists(t, ".atmos")
}

func TestCreateDestinationConflictsAndNoninteractiveAnswers(t *testing.T) {
	createProject(t)
	err := runCreate(t, "--file", "stacks/dev.yaml", "--target", "stacks/dev", "--defaults")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
	writeCreateRecipe(t, `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: required
spec:
  fields:
    - name: environment
      type: input
      required: true
  files:
    - path: stack.yaml
`)
	err = runCreate(t, "--target", "stacks/dev", "--template", "./recipe", "--defaults")
	require.ErrorIs(t, err, errUtils.ErrGeneratorFieldRequired)
	require.NoError(t, runCreate(t, "--target", "stacks/dev", "--template", "./recipe", "--set", "environment=dev", "--defaults", "--dry-run"))
	assert.NoDirExists(t, "stacks/dev")
}
