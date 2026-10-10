package ai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/executor"
	"github.com/cloudposse/atmos/pkg/ai/formatter"
	"github.com/cloudposse/atmos/pkg/ai/interactive"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/data"
)

// askTurn runs the questions of one `ask` invocation: the first question and any follow-ups.
type askTurn struct {
	exec         *executor.Executor
	interactive  *interactive.Session
	sess         *execSession
	sessionID    string
	timeout      time.Duration
	toolsEnabled bool
	stackContext string
}

// run sends one question, shows progress while the AI works, prints the answer, and returns its text.
// Prompt is what the AI receives; question is the plain text recorded in a persisted session.
func (t *askTurn) run(prompt, question string, history []types.Message) (string, error) {
	// Reuse the gathered project context on every turn while persisting plain questions.
	if t.stackContext != "" {
		prompt = t.stackContext + "\n\n" + prompt
	}

	// Providers that enforce the timeout themselves exclude the time spent waiting at an
	// approval prompt, so they get a context without a deadline.
	ctx, cancel := t.interactive.NewContext(t.timeout)
	defer cancel()

	// Raw-mode terminals turn ctrl+c into a keypress, so the spinner reports it.
	t.interactive.Progress.OnInterrupt(cancel)

	t.interactive.Progress.Start()
	defer t.interactive.Progress.Stop()
	result := t.exec.Execute(ctx, executor.Options{
		Prompt:       prompt,
		ToolsEnabled: t.toolsEnabled,
		SessionID:    t.sessionID,
		History:      history,
	})
	t.interactive.Progress.Stop()

	// Persist this turn (the plain question, not any context-augmented prompt)
	// so a subsequent `--session` invocation sees it.
	if result.Success {
		t.sess.recordTurn(ctx, question, result.Response)
	}

	// The spinner cancels the context when the user presses ctrl+c.
	if !result.Success && errors.Is(ctx.Err(), context.Canceled) {
		return "", errUtils.ErrUserAborted
	}
	if err := executor.ResultError(result); err != nil {
		return "", err
	}

	// Render response with tool execution details as Markdown.
	var buf bytes.Buffer
	if err := formatter.NewFormatter(formatter.FormatMarkdown).Format(&buf, result); err != nil {
		return "", fmt.Errorf("failed to format response: %w", err)
	}
	if err := data.Markdownf("%s", buf.String()); err != nil {
		return "", err
	}

	return result.Response, nil
}
