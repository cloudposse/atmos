package tui

import (
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/approval"
)

// chatQuestionApprover gives model questions the same terminal handoff as tool
// approvals. Questions bypass the permission prompter and its handoff wrapper.
type chatQuestionApprover struct {
	inner    approval.Approver
	terminal func() chatTerminal
}

// Approve releases chat's terminal only for questions; tool approvals already
// release it through chatPrompter when an uncached decision needs a prompt.
func (a *chatQuestionApprover) Approve(ctx context.Context, req approval.Request) (decision approval.Decision, err error) {
	if req.ToolName != approval.AskUserQuestionTool {
		return a.inner.Approve(ctx, req)
	}
	terminal := a.terminal()
	if terminal == nil {
		return approval.Decision{}, errUtils.ErrInteractiveNotAvailable
	}
	if err = terminal.ReleaseTerminal(); err != nil {
		return approval.Decision{}, fmt.Errorf("%w: release chat terminal: %w", errUtils.ErrAIPermissionPromptFailed, err)
	}
	defer func() {
		if restoreErr := terminal.RestoreTerminal(); restoreErr != nil {
			decision = approval.Decision{}
			err = errors.Join(err, fmt.Errorf("%w: restore chat terminal: %w", errUtils.ErrAIPermissionPromptFailed, restoreErr))
		}
	}()
	return a.inner.Approve(ctx, req)
}
