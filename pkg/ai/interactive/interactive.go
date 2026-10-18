// Package interactive prepares an AI client for a terminal session.
//
// It connects the pieces that make a one-shot request feel alive: a spinner that
// follows the provider's tool activity, and an approval prompt (backed by the
// Atmos permission system) for providers that run their own tools.
package interactive

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudposse/atmos/pkg/ai/approval"
	"github.com/cloudposse/atmos/pkg/ai/progress"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Session is the interactive state for one request.
type Session struct {
	// Progress is the spinner for the request. Start it before sending and Stop it afterwards.
	Progress *progress.Progress
	// TimeoutManaged is true when the provider enforces the timeout itself, excluding time
	// spent at approval prompts. The caller then needs a cancel-only context.
	TimeoutManaged bool
}

// Attach wires client for interactive use and returns the session.
//
// Providers opt in through the interfaces in the approval package; a client that
// implements none of them gets only the spinner. The timeout is handed to providers that
// enforce it themselves.
func Attach(atmosConfig *schema.AtmosConfiguration, client any, message string, timeout time.Duration) (*Session, error) {
	defer perf.Track(atmosConfig, "interactive.Attach")()

	session := &Session{Progress: progress.New(message)}
	session.Progress.Attach(client)

	if approvable, ok := client.(approval.Approvable); ok {
		before, after := session.Progress.PromptHooks()
		checker, err := permission.NewFromConfig(atmosConfig, permission.WithPromptHooks(before, after))
		if err != nil {
			return nil, fmt.Errorf("failed to set up tool approval: %w", err)
		}
		approvable.SetApprover(approval.NewPermissionApprover(checker, approval.WithPromptHooks(before, after)))
	}

	if managed, ok := client.(approval.TimeoutManaged); ok {
		managed.SetTimeout(timeout)
		session.TimeoutManaged = true
	}

	return session, nil
}

// NewContext returns the context for the request. Providers that enforce the timeout
// themselves get a context without a deadline, so time spent waiting at an approval
// prompt does not count against it; every other provider gets a deadline.
func (s *Session) NewContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	defer perf.Track(nil, "interactive.Session.NewContext")()

	if s.TimeoutManaged {
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), timeout)
}
