package permission

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/charmbracelet/huh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/schema"
)

// The prompt tests decide on one command and check that a decision for it does not leak to another.
const (
	promptCommandKey = "Bash(atmos list stacks)"
	otherCommandKey  = "Bash(atmos destroy)"
)

// promptTool is the tool the prompt tests ask about.
var promptTool = scopedFakeTool{name: "Bash", key: promptCommandKey}

// promptParams are the parameters shown for promptTool.
var promptParams = map[string]interface{}{"command": "atmos list stacks"}

// formScript drives every huh form through its accessible mode, answering from a script with one line
// per prompt, so the real production prompts run without a terminal.
type formScript struct {
	in io.Reader
	// out collects what the forms print (the request, titles, numbered options).
	out bytes.Buffer
	// forms counts the forms that ran.
	forms int
}

// run is the runForm seam.
func (s *formScript) run(form *huh.Form) error {
	s.forms++
	return form.WithAccessible(true).WithInput(s.in).WithOutput(&s.out).Run()
}

// scriptForms replaces the runForm seam with a scripted accessible-mode run and restores it afterwards.
// It also fails the test when the script is not fully consumed, so a prompt that silently falls back to
// its default cannot go unnoticed.
func scriptForms(t *testing.T, script string) *formScript {
	t.Helper()

	// Every accessible prompt builds its own line scanner, which would swallow the whole script on the
	// first prompt; handing the text out one byte at a time leaves the following lines for later prompts.
	s := &formScript{in: iotest.OneByteReader(strings.NewReader(script))}
	overrideRunForm(t, s.run)
	t.Cleanup(func() {
		rest, err := io.ReadAll(s.in)
		require.NoError(t, err)
		assert.Empty(t, string(rest), "the scripted answers must all be used")
	})
	return s
}

// overrideRunForm swaps the runForm seam for the duration of a test.
func overrideRunForm(t *testing.T, fn func(*huh.Form) error) {
	t.Helper()

	orig := runForm
	runForm = fn
	t.Cleanup(func() { runForm = orig })
}

// overrideStdinIsTerminal swaps the stdinIsTerminal seam for the duration of a test.
func overrideStdinIsTerminal(t *testing.T, isTerminal bool) {
	t.Helper()

	orig := stdinIsTerminal
	stdinIsTerminal = func() bool { return isTerminal }
	t.Cleanup(func() { stdinIsTerminal = orig })
}

// failIfFormRuns makes the test fail when any form is shown.
func failIfFormRuns(t *testing.T) {
	t.Helper()

	overrideRunForm(t, func(*huh.Form) error {
		t.Error("no form may be shown")
		return nil
	})
}

// newCacheAt opens the decision cache stored under basePath.
func newCacheAt(t *testing.T, basePath string) *PermissionCache {
	t.Helper()

	cache, err := NewPermissionCache(basePath)
	require.NoError(t, err)
	return cache
}

// TestCLIPrompter_Prompt_WithCache covers the four choices of the cached prompt and what each one saves.
func TestCLIPrompter_Prompt_WithCache(t *testing.T) {
	tests := []struct {
		name        string
		script      string
		wantAllowed bool
		wantSaved   string // "allow", "deny", or "" for nothing saved.
		wantReceipt string
	}{
		{name: "allow once", script: "1\n", wantAllowed: true, wantReceipt: "Allowed Bash: atmos list stacks"},
		{name: "always allow", script: "2\n", wantAllowed: true, wantSaved: "allow", wantReceipt: "Allowed Bash: atmos list stacks (saved to " + settingsPathHint},
		{name: "deny once", script: "3\n", wantAllowed: false, wantReceipt: "Denied Bash: atmos list stacks"},
		{name: "always deny", script: "4\n", wantAllowed: false, wantSaved: "deny", wantReceipt: "Denied Bash: atmos list stacks (saved to " + settingsPathHint},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideStdinIsTerminal(t, true)
			script := scriptForms(t, tt.script)
			basePath := filepath.Join(t.TempDir(), "base")
			cache := newCacheAt(t, basePath)
			p := NewCLIPrompterWithCache(cache)

			var allowed bool
			var err error
			stderr := captureStderr(t, func() {
				allowed, err = p.Prompt(context.Background(), promptTool, promptParams)
			})

			require.NoError(t, err)
			assert.Equal(t, tt.wantAllowed, allowed)
			assert.Equal(t, 1, script.forms)

			// What was saved, and that it is scoped to this command only.
			reloaded := newCacheAt(t, basePath)
			switch tt.wantSaved {
			case "allow":
				assert.True(t, reloaded.IsAllowedExact(promptCommandKey), "the decision is persisted")
				assert.False(t, reloaded.IsAllowedExact(otherCommandKey), "another command is not allowed")
				assert.False(t, reloaded.IsDeniedExact(promptCommandKey))
				assert.Equal(t, []string{promptCommandKey}, reloaded.GetAllowList())
				assert.Empty(t, reloaded.GetDenyList())
			case "deny":
				assert.True(t, reloaded.IsDeniedExact(promptCommandKey), "the decision is persisted")
				assert.False(t, reloaded.IsDeniedExact(otherCommandKey), "another command is not denied")
				assert.False(t, reloaded.IsAllowedExact(promptCommandKey))
				assert.Equal(t, []string{promptCommandKey}, reloaded.GetDenyList())
				assert.Empty(t, reloaded.GetAllowList())
			default:
				assert.Empty(t, reloaded.GetAllowList(), "a one-time answer is not remembered")
				assert.Empty(t, reloaded.GetDenyList(), "a one-time answer is not remembered")
			}

			receipt := ansi.Strip(stderr)
			assert.Contains(t, receipt, tt.wantReceipt)
			assert.Equal(t, tt.wantSaved != "", strings.Contains(receipt, "saved to"))
		})
	}
}

// TestCLIPrompter_Prompt_WithCacheShowsTheRequestAndChoices checks the request and the choices shown in the cached prompt.
func TestCLIPrompter_Prompt_WithCacheShowsTheRequestAndChoices(t *testing.T) {
	tests := []struct {
		name       string
		tool       Tool
		params     map[string]interface{}
		wantShown  []string
		wantAlways []string
	}{
		{
			name:       "command-scoped tool",
			tool:       promptTool,
			params:     promptParams,
			wantShown:  []string{requestTitle, "Tool", "Bash", "atmos list stacks", "Allow execution?"},
			wantAlways: []string{"2. Always allow this command", "4. Always deny this command"},
		},
		{
			name:       "tool-wide decisions name the tool",
			tool:       plainFakeTool{name: "mcp__atmos__list_stacks"},
			params:     nil,
			wantShown:  []string{requestTitle, "atmos → list_stacks"},
			wantAlways: []string{"2. Always allow atmos → list_stacks", "4. Always deny atmos → list_stacks"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideStdinIsTerminal(t, true)
			script := scriptForms(t, "3\n")
			p := NewCLIPrompterWithCache(newTestCache(t))

			captureStderr(t, func() {
				_, err := p.Prompt(context.Background(), tt.tool, tt.params)
				require.NoError(t, err)
			})

			shown := ansi.Strip(script.out.String())
			for _, want := range tt.wantShown {
				assert.Contains(t, shown, want)
			}
			assert.Contains(t, shown, "1. Allow once")
			assert.Contains(t, shown, "3. Deny once")
			for _, want := range tt.wantAlways {
				assert.Contains(t, shown, want)
			}
		})
	}
}

// TestCLIPrompter_Prompt_WithoutCache covers the allow or deny confirmation used when there is no cache.
func TestCLIPrompter_Prompt_WithoutCache(t *testing.T) {
	tests := []struct {
		name        string
		script      string
		wantAllowed bool
		wantReceipt string
	}{
		{name: "allow", script: "y\n", wantAllowed: true, wantReceipt: "Allowed Bash: atmos list stacks"},
		{name: "enter takes the default, which is allow", script: "\n", wantAllowed: true, wantReceipt: "Allowed Bash: atmos list stacks"},
		{name: "deny", script: "n\n", wantAllowed: false, wantReceipt: "Denied Bash: atmos list stacks"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideStdinIsTerminal(t, true)
			script := scriptForms(t, tt.script)
			p := NewCLIPrompter()

			var allowed bool
			var err error
			stderr := captureStderr(t, func() {
				allowed, err = p.Prompt(context.Background(), promptTool, promptParams)
			})

			require.NoError(t, err)
			assert.Equal(t, tt.wantAllowed, allowed)
			assert.Equal(t, 1, script.forms)

			receipt := ansi.Strip(stderr)
			assert.Contains(t, receipt, tt.wantReceipt)
			assert.NotContains(t, receipt, "saved to", "without a cache nothing is remembered")
			assert.Contains(t, ansi.Strip(script.out.String()), "Allow execution?")
		})
	}
}

// TestCLIPrompter_Prompt_CachedDecisionSkipsTheForm checks that a saved decision answers before any terminal check or form.
func TestCLIPrompter_Prompt_CachedDecisionSkipsTheForm(t *testing.T) {
	// A remembered decision answers before the terminal check and before any form, so it also works in CI.
	overrideStdinIsTerminal(t, false)
	failIfFormRuns(t)

	cache := newTestCache(t)
	require.NoError(t, cache.AddAllow(promptCommandKey))
	require.NoError(t, cache.AddDeny(otherCommandKey))
	p := NewCLIPrompterWithCache(cache)

	var allowed, denied bool
	var allowErr, denyErr error
	stderr := captureStderr(t, func() {
		allowed, allowErr = p.Prompt(context.Background(), promptTool, promptParams)
		denied, denyErr = p.Prompt(context.Background(), scopedFakeTool{name: "Bash", key: otherCommandKey}, nil)
	})

	require.NoError(t, allowErr)
	require.NoError(t, denyErr)
	assert.True(t, allowed)
	assert.False(t, denied)
	assert.Empty(t, stderr, "a remembered decision prints nothing")
}

// TestCLIPrompter_Prompt_RememberedDecisionIsUsedNextTime checks that "always" answers the next identical request without a form.
func TestCLIPrompter_Prompt_RememberedDecisionIsUsedNextTime(t *testing.T) {
	overrideStdinIsTerminal(t, true)
	script := scriptForms(t, "2\n")
	p := NewCLIPrompterWithCache(newTestCache(t))

	var first, second bool
	captureStderr(t, func() {
		var err error
		first, err = p.Prompt(context.Background(), promptTool, promptParams)
		require.NoError(t, err)
		second, err = p.Prompt(context.Background(), promptTool, promptParams)
		require.NoError(t, err)
	})

	assert.True(t, first)
	assert.True(t, second)
	assert.Equal(t, 1, script.forms, "the second request is answered from the saved decision")
}

// TestCLIPrompter_Prompt_NonInteractive checks the error and silence when no terminal is available.
func TestCLIPrompter_Prompt_NonInteractive(t *testing.T) {
	tests := []struct {
		name string
		p    *CLIPrompter
	}{
		{name: "with cache", p: NewCLIPrompterWithCache(newTestCache(t))},
		{name: "without cache", p: NewCLIPrompter()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideStdinIsTerminal(t, false)
			failIfFormRuns(t)

			var allowed bool
			var err error
			stderr := captureStderr(t, func() {
				allowed, err = tt.p.Prompt(context.Background(), promptTool, promptParams)
			})

			require.ErrorIs(t, err, errUtils.ErrInteractiveNotAvailable)
			assert.False(t, allowed)
			assert.Empty(t, stderr, "non-interactive logs stay free of a dangling request")
		})
	}
}

// TestCLIPrompter_Prompt_FormFailures checks abort and failure mapping, and that nothing is saved or printed.
func TestCLIPrompter_Prompt_FormFailures(t *testing.T) {
	boom := errors.New("terminal exploded")

	tests := []struct {
		name      string
		cache     bool
		failWith  error
		wantAbort bool
	}{
		{name: "abort with cache", cache: true, failWith: huh.ErrUserAborted, wantAbort: true},
		{name: "abort without cache", cache: false, failWith: huh.ErrUserAborted, wantAbort: true},
		{name: "failure with cache", cache: true, failWith: boom},
		{name: "failure without cache", cache: false, failWith: boom},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideStdinIsTerminal(t, true)
			overrideRunForm(t, func(*huh.Form) error { return tt.failWith })
			basePath := filepath.Join(t.TempDir(), "base")
			p := NewCLIPrompter()
			if tt.cache {
				p = NewCLIPrompterWithCache(newCacheAt(t, basePath))
			}

			var allowed bool
			var err error
			stderr := captureStderr(t, func() {
				allowed, err = p.Prompt(context.Background(), promptTool, promptParams)
			})

			require.Error(t, err)
			assert.False(t, allowed, "an unanswered prompt never allows")
			assert.Empty(t, stderr, "no receipt is printed when nothing was decided")
			if tt.wantAbort {
				require.ErrorIs(t, err, errUtils.ErrUserAborted)
				assert.NotErrorIs(t, err, errUtils.ErrAIPermissionPromptFailed)
			} else {
				require.ErrorIs(t, err, errUtils.ErrAIPermissionPromptFailed)
				require.ErrorIs(t, err, boom)
				assert.NotErrorIs(t, err, errUtils.ErrUserAborted)
			}
			if tt.cache {
				reloaded := newCacheAt(t, basePath)
				assert.Empty(t, reloaded.GetAllowList())
				assert.Empty(t, reloaded.GetDenyList())
			}
		})
	}
}

// TestCLIPrompter_CheckCachedForTool_ScopedToolWithoutCache checks that a command-scoped tool is never cached without a cache.
func TestCLIPrompter_CheckCachedForTool_ScopedToolWithoutCache(t *testing.T) {
	p := NewCLIPrompter()

	decision, found := p.checkCachedForTool(promptTool)

	assert.False(t, decision)
	assert.False(t, found)
	assert.False(t, p.HasCachedDecision(promptTool))
}

// TestRunRequestForm_MapsErrors checks how form errors map onto Atmos errors.
func TestRunRequestForm_MapsErrors(t *testing.T) {
	boom := errors.New("boom")

	t.Run("success", func(t *testing.T) {
		overrideRunForm(t, func(*huh.Form) error { return nil })

		require.NoError(t, runRequestForm(huh.NewForm()))
	})

	t.Run("abort is the Atmos abort error", func(t *testing.T) {
		overrideRunForm(t, func(*huh.Form) error { return huh.ErrUserAborted })

		err := runRequestForm(huh.NewForm())

		require.ErrorIs(t, err, errUtils.ErrUserAborted)
	})

	t.Run("other errors keep both the prompt error and the cause", func(t *testing.T) {
		overrideRunForm(t, func(*huh.Form) error { return boom })

		err := runRequestForm(huh.NewForm())

		require.ErrorIs(t, err, errUtils.ErrAIPermissionPromptFailed)
		require.ErrorIs(t, err, boom)
	})
}

// TestRequestTheme_TightensTheRequestBlock checks the spacing overrides of the request theme.
func TestRequestTheme_TightensTheRequestBlock(t *testing.T) {
	theme := requestTheme()

	for name, styles := range map[string]*huh.FieldStyles{"focused": &theme.Focused, "blurred": &theme.Blurred} {
		assert.Zero(t, styles.NoteTitle.GetMarginBottom(), "%s: the note title sits flush above its body", name)
		assert.Equal(t, 1, styles.Card.GetMarginTop(), "%s: the note card owns the space above the block", name)
		assert.Zero(t, styles.Base.GetMarginTop(), "%s: the choices do not double the space", name)
	}
}

// TestPromptHooks_BracketOnlyRealPrompts runs the real CLI prompter behind a checker with prompt hooks
// (the spinner pause/resume in production): the hooks wrap a prompt that is shown and stay silent when a
// saved decision answers.
func TestPromptHooks_BracketOnlyRealPrompts(t *testing.T) {
	overrideStdinIsTerminal(t, true)

	var events []string
	script := scriptForms(t, "2\n")
	overrideRunForm(t, func(form *huh.Form) error {
		events = append(events, "form")
		return script.run(form)
	})

	cfg := &schema.AtmosConfiguration{BasePath: filepath.Join(t.TempDir(), "base")}
	checker, err := NewFromConfig(cfg, WithPromptHooks(
		func() { events = append(events, "before") },
		func() { events = append(events, "after") },
	))
	require.NoError(t, err)

	var first, second bool
	captureStderr(t, func() {
		first, err = checker.CheckPermission(context.Background(), promptTool, promptParams)
		require.NoError(t, err)
		events = append(events, "--")
		second, err = checker.CheckPermission(context.Background(), promptTool, promptParams)
		require.NoError(t, err)
	})

	assert.True(t, first)
	assert.True(t, second)
	assert.Equal(t, []string{"before", "form", "after", "--"}, events,
		"the hooks wrap the prompt and do not fire for the remembered decision")
}
