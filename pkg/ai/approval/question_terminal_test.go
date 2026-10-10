package approval

import (
	"bytes"
	"errors"
	"io"
	"os"
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

// Compile-time guard for the question fields used below.
var _ = Question{Question: "q", Header: "h", Options: []QuestionOption{{Label: "l", Description: "d"}}, MultiSelect: true}

// formScript drives every huh form through its accessible mode, answering from a script with one line
// per prompt, so the real production prompts run without a terminal.
type formScript struct {
	in io.Reader
	// out collects what the forms print (titles, numbered options, prompts).
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

// captureStderr runs fn and returns what it wrote to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	oldStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	fn()
	require.NoError(t, w.Close())

	var buf strings.Builder
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	return buf.String()
}

// TestTerminalAsker_Available reports availability from the stdin terminal check.
func TestTerminalAsker_Available(t *testing.T) {
	tests := []struct {
		name       string
		isTerminal bool
	}{
		{name: "a terminal on stdin", isTerminal: true},
		{name: "no terminal on stdin", isTerminal: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideStdinIsTerminal(t, tt.isTerminal)

			assert.Equal(t, tt.isTerminal, terminalAsker{}.Available())
		})
	}
}

// TestTerminalAsker_AvailableIsFalseForPipedStdin exercises the real terminal check: a pipe on stdin is never a terminal.
func TestTerminalAsker_AvailableIsFalseForPipedStdin(t *testing.T) {
	// Exercises the real terminal check: a pipe on stdin is never a terminal.
	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })

	assert.False(t, terminalAsker{}.Available())
}

// TestAskOne covers single and multi-select answers, including "Other…" with typed text.
func TestAskOne(t *testing.T) {
	letters := []QuestionOption{{Label: "A", Description: "first"}, {Label: "B"}, {Label: "C"}}

	tests := []struct {
		name      string
		question  Question
		script    string
		want      string
		wantForms int
	}{
		{
			name:      "single select returns the chosen label",
			question:  Question{Question: "Pick one?", Options: letters},
			script:    "2\n",
			want:      "B",
			wantForms: 1,
		},
		{
			name:      "single select returns the last listed label",
			question:  Question{Question: "Pick one?", Options: letters},
			script:    "3\n",
			want:      "C",
			wantForms: 1,
		},
		{
			name:      "other asks for text and uses it",
			question:  Question{Question: "Pick one?", Options: letters},
			script:    "4\nmy own answer\n",
			want:      "my own answer",
			wantForms: 2,
		},
		{
			name:      "typed text is trimmed",
			question:  Question{Question: "Pick one?", Options: letters},
			script:    "4\n   padded   \n",
			want:      "padded",
			wantForms: 2,
		},
		{
			name:      "empty typed text is dropped",
			question:  Question{Question: "Pick one?", Options: letters},
			script:    "4\n\n",
			want:      "",
			wantForms: 2,
		},
		{
			name:      "multi-select joins the labels with a comma",
			question:  Question{Question: "Pick some?", Options: letters, MultiSelect: true},
			script:    "1\n3\n0\n",
			want:      "A, C",
			wantForms: 1,
		},
		{
			name:      "multi-select keeps the option order whatever the pick order",
			question:  Question{Question: "Pick some?", Options: letters, MultiSelect: true},
			script:    "3\n1\n0\n",
			want:      "A, C",
			wantForms: 1,
		},
		{
			name:      "multi-select with nothing picked gives an empty answer",
			question:  Question{Question: "Pick some?", Options: letters, MultiSelect: true},
			script:    "0\n",
			want:      "",
			wantForms: 1,
		},
		{
			name:      "multi-select toggles a pick back off",
			question:  Question{Question: "Pick some?", Options: letters, MultiSelect: true},
			script:    "1\n1\n2\n0\n",
			want:      "B",
			wantForms: 1,
		},
		{
			name:      "multi-select mixes listed labels with typed text",
			question:  Question{Question: "Pick some?", Options: letters, MultiSelect: true},
			script:    "1\n4\n0\nsomething else\n",
			want:      "A, something else",
			wantForms: 2,
		},
		{
			name:      "multi-select with only other uses the typed text",
			question:  Question{Question: "Pick some?", Options: letters, MultiSelect: true},
			script:    "4\n0\nonly typed\n",
			want:      "only typed",
			wantForms: 2,
		},
		{
			name:      "multi-select with other and empty typed text keeps the listed labels",
			question:  Question{Question: "Pick some?", Options: letters, MultiSelect: true},
			script:    "2\n4\n0\n\n",
			want:      "B",
			wantForms: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := scriptForms(t, tt.script)

			got, err := askOne(tt.question)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantForms, script.forms)
		})
	}
}

// TestChoose_ShowsEachOptionAndOther checks the options shown to the user, with descriptions and the free-text option last.
func TestChoose_ShowsEachOptionAndOther(t *testing.T) {
	script := scriptForms(t, "1\n")
	q := Question{
		Question: "Pick one?",
		Header:   "Letter",
		Options:  []QuestionOption{{Label: "A", Description: "first"}, {Label: "B", Description: "  "}},
	}

	chosen, err := choose(q)

	require.NoError(t, err)
	assert.Equal(t, []string{"A"}, chosen)
	shown := ansi.Strip(script.out.String())
	assert.Contains(t, shown, "Pick one?")
	assert.Contains(t, shown, "1. A — first", "a description is shown next to its label")
	assert.Contains(t, shown, "2. B\n", "a blank description adds nothing")
	assert.Contains(t, shown, "3. "+otherLabel, "the free-text option comes last")
}

// TestTypeAnswer_UsesHeaderAsTitle checks that the typed-answer prompt is titled with the question header.
func TestTypeAnswer_UsesHeaderAsTitle(t *testing.T) {
	script := scriptForms(t, "free text\n")

	got, err := typeAnswer(Question{Question: "Which environment should it target?", Header: "Env"})

	require.NoError(t, err)
	assert.Equal(t, "free text", got)
	assert.Contains(t, ansi.Strip(script.out.String()), "Env")
}

// TestAskOne_FormFailures checks that an abort maps to the Atmos abort error and other failures pass through.
func TestAskOne_FormFailures(t *testing.T) {
	boom := errors.New("terminal exploded")
	tests := []struct {
		name string
		// failOn is the 1-based form that fails; earlier forms run from the script.
		failOn      int
		failWith    error
		script      string
		multiSelect bool
		wantAbort   bool
		wantErr     error
	}{
		{name: "abort at the selection", failOn: 1, failWith: huh.ErrUserAborted, wantAbort: true},
		{name: "abort at a multi-select", failOn: 1, failWith: huh.ErrUserAborted, multiSelect: true, wantAbort: true},
		{name: "other failure at a multi-select is returned", failOn: 1, failWith: boom, multiSelect: true, wantErr: boom},
		{name: "abort at the typed answer", failOn: 2, failWith: huh.ErrUserAborted, script: "3\n", wantAbort: true},
		{name: "other failure at the selection is returned", failOn: 1, failWith: boom, wantErr: boom},
		{name: "other failure at the typed answer is returned", failOn: 2, failWith: boom, script: "3\n", wantErr: boom},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			question := Question{
				Question:    "Pick one?",
				Options:     []QuestionOption{{Label: "A"}, {Label: "B"}},
				MultiSelect: tt.multiSelect,
			}
			scripted := &formScript{in: iotest.OneByteReader(strings.NewReader(tt.script))}
			calls := 0
			overrideRunForm(t, func(form *huh.Form) error {
				calls++
				if calls == tt.failOn {
					return tt.failWith
				}
				return scripted.run(form)
			})

			got, err := askOne(question)

			require.Error(t, err)
			assert.Empty(t, got)
			assert.Equal(t, tt.failOn, calls, "no form is shown after the failure")
			if tt.wantAbort {
				require.ErrorIs(t, err, errUtils.ErrUserAborted)
				assert.NotErrorIs(t, err, huh.ErrUserAborted, "the huh error is mapped to the Atmos error")
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
			assert.NotErrorIs(t, err, errUtils.ErrUserAborted, "only an abort maps to ErrUserAborted")
		})
	}
}

// TestTerminalAsker_Ask checks answers keyed by question text, asked in order, with one receipt per question.
func TestTerminalAsker_Ask(t *testing.T) {
	questions := []Question{
		{
			Question: "Which environment?",
			Header:   "Env",
			Options:  []QuestionOption{{Label: "dev"}, {Label: "prod"}},
		},
		{
			// No header: the receipt falls back to the question text.
			Question:    "Which regions?",
			Options:     []QuestionOption{{Label: "us-east-1"}, {Label: "us-west-2"}, {Label: "eu-west-1"}},
			MultiSelect: true,
		},
		{
			Question: "Anything else?",
			Header:   "Notes",
			Options:  []QuestionOption{{Label: "no"}},
		},
	}
	// Question 1: prod. Question 2: us-east-1 and eu-west-1. Question 3: other, typed text.
	script := scriptForms(t, "2\n1\n3\n0\n2\nplease be careful\n")

	var answers map[string]string
	var err error
	stderr := captureStderr(t, func() {
		answers, err = terminalAsker{}.Ask(t.Context(), questions)
	})

	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"Which environment?": "prod",
		"Which regions?":     "us-east-1, eu-west-1",
		"Anything else?":     "please be careful",
	}, answers, "answers are keyed by question text")
	assert.Equal(t, 4, script.forms, "three questions and one typed answer")

	// One receipt per question, in the order asked, labeled by header or by question text.
	receipts := ansi.Strip(stderr)
	env := strings.Index(receipts, "Env: ")
	regions := strings.Index(receipts, "Which regions?: ")
	notes := strings.Index(receipts, "Notes: ")
	require.GreaterOrEqual(t, env, 0, "receipts: %q", receipts)
	require.GreaterOrEqual(t, regions, 0, "receipts: %q", receipts)
	require.GreaterOrEqual(t, notes, 0, "receipts: %q", receipts)
	assert.Less(t, env, regions)
	assert.Less(t, regions, notes)
	assert.Contains(t, receipts, "prod")
	assert.Contains(t, receipts, "us-east-1, eu-west-1")
	assert.Contains(t, receipts, "please be careful")
}

// TestTerminalAsker_Ask_AbortDiscardsAnswers checks that an abort mid-way returns no partial answers.
func TestTerminalAsker_Ask_AbortDiscardsAnswers(t *testing.T) {
	questions := []Question{
		{Question: "First?", Options: []QuestionOption{{Label: "a"}, {Label: "b"}}},
		{Question: "Second?", Options: []QuestionOption{{Label: "c"}, {Label: "d"}}},
	}
	scripted := &formScript{in: iotest.OneByteReader(strings.NewReader("2\n"))}
	calls := 0
	overrideRunForm(t, func(form *huh.Form) error {
		calls++
		if calls == 2 {
			return huh.ErrUserAborted
		}
		return scripted.run(form)
	})

	var answers map[string]string
	var err error
	stderr := captureStderr(t, func() {
		answers, err = terminalAsker{}.Ask(t.Context(), questions)
	})

	require.ErrorIs(t, err, errUtils.ErrUserAborted)
	assert.Nil(t, answers, "nothing is returned for a half-answered set")
	assert.Equal(t, 2, calls)
	assert.Contains(t, ansi.Strip(stderr), "First?", "the first answer was already acknowledged")
	assert.NotContains(t, ansi.Strip(stderr), "Second?")
}

// TestAskQuestions_ThroughTheTerminalAsker runs question requests end to end through the real terminal asker.
func TestAskQuestions_ThroughTheTerminalAsker(t *testing.T) {
	t.Run("answers reach the model in the tool input", func(t *testing.T) {
		overrideStdinIsTerminal(t, true)
		script := scriptForms(t, "2\n")
		approver, prompter := questionApprover(t, &schema.AIToolSettings{}, terminalAsker{})
		req := questionRequest()

		var decision Decision
		var err error
		captureStderr(t, func() {
			decision, err = approver.Approve(t.Context(), req)
		})

		require.NoError(t, err)
		assert.True(t, decision.Allow)
		assert.Equal(t, map[string]string{"Count only stacks that have components?": "No"}, decision.UpdatedInput["answers"])
		assert.Equal(t, 1, script.forms)
		assert.Zero(t, prompter.callCount(), "a question is not a permission request")
		assert.NotContains(t, req.Input, "answers", "the caller's request is not modified")
	})

	t.Run("without a terminal the model is told and no form is shown", func(t *testing.T) {
		overrideStdinIsTerminal(t, false)
		formsRun := 0
		overrideRunForm(t, func(*huh.Form) error {
			formsRun++
			return nil
		})
		approver, _ := questionApprover(t, &schema.AIToolSettings{}, terminalAsker{})

		decision, err := approver.Approve(t.Context(), questionRequest())

		require.NoError(t, err)
		assert.False(t, decision.Allow)
		assert.False(t, decision.Interrupt)
		assert.Equal(t, noTerminalMessage, decision.Message)
		assert.Zero(t, formsRun)
	})

	t.Run("an abort interrupts the whole run", func(t *testing.T) {
		overrideStdinIsTerminal(t, true)
		overrideRunForm(t, func(*huh.Form) error { return huh.ErrUserAborted })
		approver, _ := questionApprover(t, &schema.AIToolSettings{}, terminalAsker{})

		decision, err := approver.Approve(t.Context(), questionRequest())

		require.NoError(t, err)
		assert.False(t, decision.Allow)
		assert.True(t, decision.Interrupt)
		assert.Equal(t, abortedMessage, decision.Message)
		assert.Nil(t, decision.UpdatedInput)
	})

	t.Run("a form failure is returned as an error", func(t *testing.T) {
		boom := errors.New("terminal exploded")
		overrideStdinIsTerminal(t, true)
		overrideRunForm(t, func(*huh.Form) error { return boom })
		approver, _ := questionApprover(t, &schema.AIToolSettings{}, terminalAsker{})

		decision, err := approver.Approve(t.Context(), questionRequest())

		require.ErrorIs(t, err, boom)
		assert.False(t, decision.Allow)
		assert.False(t, decision.Interrupt)
	})
}
