package step

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
)

// skipIfInteractiveTTY skips a test when both stdin and stdout are TTYs, because
// the interactive Execute path would then render a huh form and block. The
// non-TTY default behavior under test only triggers when there is no TTY (the
// normal `go test`/CI environment).
func skipIfInteractiveTTY(t *testing.T) {
	t.Helper()
	term := terminal.New()
	if term.IsTTY(terminal.Stdin) && term.IsTTY(terminal.Stdout) {
		t.Skip("stdin and stdout are both TTYs; interactive Execute would block on a prompt")
	}
}

// TestBaseHandler_resolveInteractive verifies the shared decision helper that
// drives non-TTY default fallback for every interactive step handler.
func TestBaseHandler_resolveInteractive(t *testing.T) {
	skipIfInteractiveTTY(t)

	t.Run("non-interactive handler always uses the prompt path", func(t *testing.T) {
		handler := NewBaseHandler("shell", CategoryCommand, false)
		step := &schema.WorkflowStep{Name: "s", Type: "shell"}

		shouldPrompt, err := handler.resolveInteractive(step)
		require.NoError(t, err)
		assert.True(t, shouldPrompt, "non-interactive handlers should not fall back to defaults")
	})

	t.Run("interactive handler with default falls back without error", func(t *testing.T) {
		handler := NewBaseHandler("input", CategoryInteractive, true)
		step := &schema.WorkflowStep{Name: "s", Type: "input", Default: "value"}

		shouldPrompt, err := handler.resolveInteractive(step)
		require.NoError(t, err)
		assert.False(t, shouldPrompt, "a configured default should trigger the non-TTY path")
	})

	t.Run("interactive handler without default returns ErrStepTTYRequired", func(t *testing.T) {
		handler := NewBaseHandler("input", CategoryInteractive, true)
		step := &schema.WorkflowStep{Name: "s", Type: "input"}

		shouldPrompt, err := handler.resolveInteractive(step)
		require.Error(t, err)
		assert.False(t, shouldPrompt)
		assert.ErrorIs(t, err, errUtils.ErrStepTTYRequired)
	})
}

// TestInteractiveHandlers_NonTTYUsesDefault verifies each interactive handler
// returns its configured default (instead of ErrStepTTYRequired) when no TTY is
// available.
func TestInteractiveHandlers_NonTTYUsesDefault(t *testing.T) {
	skipIfInteractiveTTY(t)

	ctx := context.Background()

	tests := []struct {
		name       string
		step       *schema.WorkflowStep
		wantValue  string
		wantValues []string
	}{
		{
			name:      "choose returns default",
			step:      &schema.WorkflowStep{Name: "account", Type: "choose", Prompt: "Account", Options: []string{"dev", "prod"}, Default: "prod"},
			wantValue: "prod",
		},
		{
			name:      "input returns default",
			step:      &schema.WorkflowStep{Name: "tag", Type: "input", Prompt: "Release tag", Default: "latest"},
			wantValue: "latest",
		},
		{
			name:      "confirm yes default returns true",
			step:      &schema.WorkflowStep{Name: "go", Type: "confirm", Prompt: "Proceed?", Default: "yes"},
			wantValue: "true",
		},
		{
			name:      "confirm no default returns false",
			step:      &schema.WorkflowStep{Name: "go", Type: "confirm", Prompt: "Proceed?", Default: "no"},
			wantValue: "false",
		},
		{
			name:      "filter single returns default",
			step:      &schema.WorkflowStep{Name: "region", Type: "filter", Prompt: "Region", Options: []string{"use1", "euc1"}, Default: "euc1"},
			wantValue: "euc1",
		},
		{
			name:       "filter multiple splits default on commas",
			step:       &schema.WorkflowStep{Name: "regions", Type: "filter", Prompt: "Regions", Options: []string{"use1", "euc1", "apse1"}, Multiple: true, Default: "use1, apse1"},
			wantValue:  "use1",
			wantValues: []string{"use1", "apse1"},
		},
		{
			name:      "file returns default path",
			step:      &schema.WorkflowStep{Name: "cfg", Type: "file", Prompt: "Pick a config", Default: "config/prod.yaml"},
			wantValue: "config/prod.yaml",
		},
		{
			name:      "write returns default text",
			step:      &schema.WorkflowStep{Name: "notes", Type: "write", Prompt: "Notes", Default: "line1\nline2"},
			wantValue: "line1\nline2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, ok := Get(tt.step.Type)
			require.True(t, ok, "handler %q should be registered", tt.step.Type)

			result, err := handler.Execute(ctx, tt.step, NewVariables())
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantValue, result.Value)
			if tt.wantValues != nil {
				assert.Equal(t, tt.wantValues, result.Values)
			}
		})
	}
}

// TestInteractiveHandlers_NonTTYDefaultError verifies that when a default is set
// but its template is invalid, the non-TTY path surfaces the resolution error
// (covers the ResolveDefault error branch in each interactive handler).
func TestInteractiveHandlers_NonTTYDefaultError(t *testing.T) {
	skipIfInteractiveTTY(t)

	ctx := context.Background()
	names := []string{"choose", "input", "confirm", "filter", "file", "write"}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			handler, ok := Get(name)
			require.True(t, ok)

			step := &schema.WorkflowStep{
				Name:    name,
				Type:    name,
				Prompt:  "Prompt",
				Options: []string{"a", "b"},   // For choose/filter.
				Default: "{{ .steps.invalid.", // Non-empty default with an invalid template.
			}

			result, err := handler.Execute(ctx, step, NewVariables())
			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
		})
	}
}

// TestInteractiveHandlers_NonTTYDefaultTemplating verifies the default value is
// rendered through step variable templating (e.g. {{ .env.VAR }}) before it is
// returned in the non-TTY path.
func TestInteractiveHandlers_NonTTYDefaultTemplating(t *testing.T) {
	skipIfInteractiveTTY(t)

	vars := NewVariables()
	vars.SetEnv("STACK_ACCOUNT", "prod")

	step := &schema.WorkflowStep{
		Name:    "account",
		Type:    "choose",
		Prompt:  "Account",
		Options: []string{"dev", "prod"},
		Default: "{{ .env.STACK_ACCOUNT }}",
	}

	handler, ok := Get("choose")
	require.True(t, ok)

	result, err := handler.Execute(context.Background(), step, vars)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "prod", result.Value)
}

// TestInteractiveHandlers_TTYBranchPromptError forces a TTY so Execute enters
// the interactive branch, then uses an invalid prompt template so it errors
// during prompt resolution BEFORE rendering a huh form (which would block).
// This exercises the TTY-present branch of every interactive handler without a
// real terminal.
func TestInteractiveHandlers_TTYBranchPromptError(t *testing.T) {
	// terminal.New() reads viper's "force-tty"; forcing it makes IsTTY return
	// true so resolveInteractive selects the prompt path.
	previous := viper.GetBool("force-tty")
	viper.Set("force-tty", true)
	t.Cleanup(func() { viper.Set("force-tty", previous) })

	ctx := context.Background()
	names := []string{"choose", "input", "confirm", "filter", "file", "write"}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			handler, ok := Get(name)
			require.True(t, ok)

			step := &schema.WorkflowStep{
				Name:    name,
				Type:    name,
				Prompt:  "{{ .steps.invalid.value", // Invalid template: errors before any huh form.
				Options: []string{"a", "b"},        // For choose/filter.
			}

			result, err := handler.Execute(ctx, step, NewVariables())
			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
		})
	}
}

// TestSplitFilterDefaults covers the comma-splitting helper used by the filter
// step's multi-select non-TTY default path.
func TestSplitFilterDefaults(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"whitespace only", "   ", nil},
		{"single value", "vpc", []string{"vpc"}},
		{"trimmed and empties skipped", " use1 , ,euc1,", []string{"use1", "euc1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, splitFilterDefaults(tt.input))
		})
	}
}

// TestConfirmHandler_NonTTYDefaultTemplating verifies the confirm handler
// resolves Go templates in its default before parsing the yes/true value, so a
// templated default behaves the same as in choose/input/write.
func TestConfirmHandler_NonTTYDefaultTemplating(t *testing.T) {
	skipIfInteractiveTTY(t)

	handler, ok := Get("confirm")
	require.True(t, ok)

	t.Run("templated default resolving to true returns true", func(t *testing.T) {
		vars := NewVariables()
		vars.SetEnv("CONFIRM", "true")
		step := &schema.WorkflowStep{Name: "go", Type: "confirm", Prompt: "Proceed?", Default: "{{ .env.CONFIRM }}"}

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "true", result.Value)
	})

	t.Run("templated default resolving to no returns false", func(t *testing.T) {
		vars := NewVariables()
		vars.SetEnv("CONFIRM", "no")
		step := &schema.WorkflowStep{Name: "go", Type: "confirm", Prompt: "Proceed?", Default: "{{ .env.CONFIRM }}"}

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "false", result.Value)
	})
}
