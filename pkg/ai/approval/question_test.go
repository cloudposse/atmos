package approval

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeAsker answers questions from a script and records what it was asked.
type fakeAsker struct {
	unavailable bool
	answers     map[string]string
	err         error

	asked [][]Question
	log   *[]string
}

func (f *fakeAsker) Available() bool { return !f.unavailable }

func (f *fakeAsker) Ask(_ context.Context, questions []Question) (map[string]string, error) {
	f.asked = append(f.asked, questions)
	if f.log != nil {
		*f.log = append(*f.log, "ask")
	}
	return f.answers, f.err
}

var _ Asker = (*fakeAsker)(nil)

func questionRequest() Request {
	return Request{
		ToolName:  AskUserQuestionTool,
		ToolUseID: "toolu_q",
		Input: map[string]any{
			"questions": []any{
				map[string]any{
					"question": "Count only stacks that have components?",
					"header":   "Count",
					"options": []any{
						map[string]any{"label": "Yes", "description": "Count them"},
						map[string]any{"label": "No", "description": "Keep all"},
					},
					"multiSelect": false,
				},
			},
		},
	}
}

// questionApprover builds an approver whose permission prompter must never be used for questions.
func questionApprover(t *testing.T, tools *schema.AIToolSettings, asker Asker, opts ...ApproverOption) (*PermissionApprover, *stubPrompter) {
	t.Helper()
	prompter := &stubPrompter{}
	cfg := &schema.AtmosConfiguration{
		BasePath: filepath.Join(t.TempDir(), "base"),
		AI:       schema.AISettings{Tools: *tools},
	}
	checker, err := permission.NewFromConfig(cfg, permission.WithPrompter(prompter))
	require.NoError(t, err)
	return NewPermissionApprover(checker, append([]ApproverOption{WithAsker(asker)}, opts...)...), prompter
}

func TestParseQuestions(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]any
		want    []Question
		wantErr bool
	}{
		{
			name:  "one question with options",
			input: questionRequest().Input,
			want: []Question{{
				Question: "Count only stacks that have components?",
				Header:   "Count",
				Options:  []QuestionOption{{Label: "Yes", Description: "Count them"}, {Label: "No", Description: "Keep all"}},
			}},
		},
		{
			name: "multi-select is kept",
			input: map[string]any{"questions": []any{
				map[string]any{"question": "Which?", "multiSelect": true, "options": []any{map[string]any{"label": "a"}}},
			}},
			want: []Question{{Question: "Which?", MultiSelect: true, Options: []QuestionOption{{Label: "a"}}}},
		},
		{
			name: "questions without text are dropped",
			input: map[string]any{"questions": []any{
				map[string]any{"question": "  "},
				map[string]any{"question": "Keep me?"},
			}},
			want: []Question{{Question: "Keep me?"}},
		},
		{name: "no questions", input: map[string]any{}, want: nil},
		{name: "questions of the wrong type", input: map[string]any{"questions": "nope"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseQuestions(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAskQuestions_AnswersAreReturnedInTheToolInput(t *testing.T) {
	asker := &fakeAsker{answers: map[string]string{"Count only stacks that have components?": "Yes"}}
	approver, prompter := questionApprover(t, &schema.AIToolSettings{}, asker)
	req := questionRequest()

	decision, err := approver.Approve(t.Context(), req)

	require.NoError(t, err)
	assert.True(t, decision.Allow)
	assert.Equal(t, map[string]string{"Count only stacks that have components?": "Yes"}, decision.UpdatedInput["answers"])
	assert.Contains(t, decision.UpdatedInput, "questions", "the original questions stay in the input")
	require.Len(t, asker.asked, 1)
	assert.Equal(t, "Count only stacks that have components?", asker.asked[0][0].Question)
	assert.Zero(t, prompter.callCount(), "a question is not a permission request")
	assert.NotContains(t, req.Input, "answers", "the caller's request is not modified")
}

func TestAskQuestions_IgnoresPermissionSettings(t *testing.T) {
	// A question to the user is never a tool-permission matter, even when the tool is listed as blocked.
	tests := []struct {
		name  string
		tools schema.AIToolSettings
	}{
		{name: "blocked list", tools: schema.AIToolSettings{Blocked: []string{AskUserQuestionTool}}},
		{name: "prompt mode", tools: schema.AIToolSettings{Mode: "require_confirmation"}},
		{name: "yolo mode", tools: schema.AIToolSettings{Mode: "yolo"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asker := &fakeAsker{answers: map[string]string{"Count only stacks that have components?": "No"}}
			approver, prompter := questionApprover(t, &tt.tools, asker)

			decision, err := approver.Approve(t.Context(), questionRequest())

			require.NoError(t, err)
			assert.True(t, decision.Allow)
			assert.Len(t, asker.asked, 1)
			assert.Zero(t, prompter.callCount())
		})
	}
}

func TestAskQuestions_Failures(t *testing.T) {
	boom := errors.New("boom")

	tests := []struct {
		name         string
		asker        *fakeAsker
		request      Request
		wantAllow    bool
		wantInterupt bool
		wantErr      error
		wantMessage  string
		wantAsked    bool
	}{
		{
			name:        "no terminal: the model is told and carries on",
			asker:       &fakeAsker{unavailable: true},
			request:     questionRequest(),
			wantMessage: "no terminal is available",
		},
		{
			name:         "ctrl+c aborts the whole run",
			asker:        &fakeAsker{err: fmt.Errorf("form: %w", errUtils.ErrUserAborted)},
			request:      questionRequest(),
			wantInterupt: true,
			wantMessage:  abortedMessage,
			wantAsked:    true,
		},
		{
			name:      "any other failure is returned",
			asker:     &fakeAsker{err: boom},
			request:   questionRequest(),
			wantErr:   boom,
			wantAsked: true,
		},
		{
			name:        "a question that cannot be read is refused without asking",
			asker:       &fakeAsker{},
			request:     Request{ToolName: AskUserQuestionTool, Input: map[string]any{"questions": "nope"}},
			wantMessage: malformedQuestionMessage,
		},
		{
			name:        "a request with no questions is refused without asking",
			asker:       &fakeAsker{},
			request:     Request{ToolName: AskUserQuestionTool, Input: map[string]any{}},
			wantMessage: malformedQuestionMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approver, _ := questionApprover(t, &schema.AIToolSettings{}, tt.asker)

			decision, err := approver.Approve(t.Context(), tt.request)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantAllow, decision.Allow)
			assert.Equal(t, tt.wantInterupt, decision.Interrupt)
			assert.False(t, decision.NonInteractive, "a missing terminal must not be reported as a permission problem")
			assert.Nil(t, decision.UpdatedInput)
			assert.Contains(t, decision.Message, tt.wantMessage)
			assert.Equal(t, tt.wantAsked, len(tt.asker.asked) > 0)
		})
	}
}

func TestAskQuestions_HooksBracketTheQuestion(t *testing.T) {
	t.Run("called around the question", func(t *testing.T) {
		var log []string
		asker := &fakeAsker{answers: map[string]string{"q": "a"}, log: &log}
		approver, _ := questionApprover(t, &schema.AIToolSettings{}, asker,
			WithPromptHooks(func() { log = append(log, "before") }, func() { log = append(log, "after") }))

		_, err := approver.Approve(t.Context(), questionRequest())

		require.NoError(t, err)
		assert.Equal(t, []string{"before", "ask", "after"}, log)
	})

	t.Run("after still runs when the question fails", func(t *testing.T) {
		var log []string
		asker := &fakeAsker{err: errors.New("boom"), log: &log}
		approver, _ := questionApprover(t, &schema.AIToolSettings{}, asker,
			WithPromptHooks(func() { log = append(log, "before") }, func() { log = append(log, "after") }))

		_, err := approver.Approve(t.Context(), questionRequest())

		require.Error(t, err)
		assert.Equal(t, []string{"before", "ask", "after"}, log)
	})

	t.Run("not called when no question can be shown", func(t *testing.T) {
		var log []string
		asker := &fakeAsker{unavailable: true}
		approver, _ := questionApprover(t, &schema.AIToolSettings{}, asker,
			WithPromptHooks(func() { log = append(log, "before") }, func() { log = append(log, "after") }))

		_, err := approver.Approve(t.Context(), questionRequest())

		require.NoError(t, err)
		assert.Empty(t, log, "the spinner keeps running when there is nothing to show")
	})
}

func TestQuestionLabel(t *testing.T) {
	assert.Equal(t, "Count", questionLabel(Question{Header: "Count", Question: "Count only stacks?"}))
	assert.Equal(t, "Count only stacks?", questionLabel(Question{Header: "  ", Question: "Count only stacks?"}))
}
