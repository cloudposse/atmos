package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/ai/approval"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
)

const defaultChatTimeout = 5 * time.Minute

// chatTerminal hands terminal ownership to the approval form and back to chat.
type chatTerminal interface {
	ReleaseTerminal() error
	RestoreTerminal() error
}

// chatPrompter keeps the chat renderer and approval form from reading the same terminal.
type chatPrompter struct {
	inner    permission.Prompter
	terminal func() chatTerminal
}

func (p *chatPrompter) Prompt(ctx context.Context, tool permission.Tool, params map[string]interface{}) (allowed bool, err error) {
	if cached, ok := p.inner.(interface{ HasCachedDecision(permission.Tool) bool }); ok && cached.HasCachedDecision(tool) {
		return p.inner.Prompt(ctx, tool, params)
	}
	terminal := p.terminal()
	if terminal == nil {
		return false, errUtils.ErrInteractiveNotAvailable
	}
	if err = terminal.ReleaseTerminal(); err != nil {
		return false, fmt.Errorf("%w: release chat terminal: %w", errUtils.ErrAIPermissionPromptFailed, err)
	}
	defer func() {
		if restoreErr := terminal.RestoreTerminal(); restoreErr != nil {
			allowed = false
			err = errors.Join(err, fmt.Errorf("%w: restore chat terminal: %w", errUtils.ErrAIPermissionPromptFailed, restoreErr))
		}
	}()
	return p.inner.Prompt(ctx, tool, params)
}

// configureProviderApproval also runs after provider switching so policy follows the active client.
func (m *ChatModel) configureProviderApproval(client ai.Client) error {
	if provider, ok := client.(approval.Approvable); ok {
		checker, err := permission.NewFromConfig(m.atmosConfig, permission.WithPromptWrapper(func(inner permission.Prompter) permission.Prompter {
			return &chatPrompter{inner: inner, terminal: func() chatTerminal {
				if m.program == nil {
					return nil
				}
				return m.program
			}}
		}))
		if err != nil {
			return err
		}
		provider.SetApprover(approval.NewPermissionApprover(checker))
	}
	if provider, ok := client.(approval.TimeoutManaged); ok {
		provider.SetTimeout(m.requestTimeout())
	}
	return nil
}

func (m *ChatModel) requestTimeout() time.Duration {
	if m.atmosConfig != nil && m.atmosConfig.AI.TimeoutSeconds > 0 {
		return time.Duration(m.atmosConfig.AI.TimeoutSeconds) * time.Second
	}
	return defaultChatTimeout
}

func (m *ChatModel) requestContext() (context.Context, context.CancelFunc) {
	if _, managed := m.client.(approval.TimeoutManaged); managed {
		// The provider excludes time spent waiting for approval from its timeout.
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), m.requestTimeout())
}
