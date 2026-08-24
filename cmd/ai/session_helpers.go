package ai

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/session"
	"github.com/cloudposse/atmos/pkg/ai/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// execSession bundles the session manager, storage, resolved session, and
// prior conversation history needed to thread a persisted AI session through
// the non-interactive exec/ask commands (chat.go has its own, longer-lived
// equivalent wired directly into the TUI). Callers must Close it when done
// (e.g. via defer); a nil *execSession is safe to call Close/History/
// recordTurn on, so call sites don't need a separate "sessions in play" check.
type execSession struct {
	manager *session.Manager
	storage session.Storage
	session *session.Session
	history []types.Message
}

// prepareSession loads or creates a persisted session for exec/ask when
// sessions are enabled and a --session value was given. It returns (nil, nil)
// — not an error — when sessions aren't in play, so the default (no session
// storage opened, no perf cost, behavior unchanged) stays the common case.
// When --session was given but sessions are disabled, that's surfaced as a
// warning rather than a silent no-op: without it, a user could run
// `--session foo` across multiple invocations believing conversation context
// was being carried over, with nothing persisted and no indication why.
// Model should be the actually-resolved model from the constructed AI client
// (client.GetModel()); see getModelFromConfig's doc comment (chat.go) for why
// that matters more than an independent provider-config lookup when a new
// session has to be created.
func prepareSession(ctx context.Context, atmosConfig *schema.AtmosConfiguration, sessionID, model string) (*execSession, error) {
	if sessionID == "" {
		return nil, nil
	}
	if !atmosConfig.AI.Sessions.Enabled {
		ui.Warning(fmt.Sprintf("--session %q was ignored: enable sessions with `ai.sessions.enabled: true` in atmos.yaml", sessionID))
		return nil, nil
	}
	return loadOrCreateSession(ctx, atmosConfig, sessionID, model)
}

// loadOrCreateSession opens session storage, resolves (or creates) the named
// session, and loads its prior messages as conversation history.
func loadOrCreateSession(ctx context.Context, atmosConfig *schema.AtmosConfiguration, sessionID, model string) (*execSession, error) {
	storagePath := getSessionStoragePath(atmosConfig)
	storage, err := session.NewSQLiteStorage(storagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize session storage: %w", err)
	}

	manager := session.NewManager(storage, atmosConfig.BasePath, atmosConfig.AI.Sessions.MaxSessions, atmosConfig)

	sess, err := manager.GetSessionByName(ctx, sessionID)
	if err != nil {
		log.Debugf("Session '%s' not found, creating new session", sessionID)
		sess, err = manager.CreateSession(ctx, session.CreateSessionParams{
			Name:     sessionID,
			Model:    model,
			Provider: getProviderFromConfig(atmosConfig),
		})
		if err != nil {
			_ = storage.Close()
			return nil, fmt.Errorf("failed to create session: %w", err)
		}
	}

	messages, err := manager.GetMessagesWithCompaction(ctx, sess.ID, 0)
	if err != nil {
		_ = storage.Close()
		return nil, fmt.Errorf("failed to load session history: %w", err)
	}

	return &execSession{
		manager: manager,
		storage: storage,
		session: sess,
		history: sessionMessagesToTypesMessages(messages),
	}, nil
}

// Close closes the underlying session storage. Safe to call on a nil receiver.
func (es *execSession) Close() error {
	if es == nil || es.storage == nil {
		return nil
	}
	return es.storage.Close()
}

// History returns prior session messages for use as executor.Options.History.
// Safe to call on a nil receiver (returns nil: no history to prepend).
func (es *execSession) History() []types.Message {
	if es == nil {
		return nil
	}
	return es.history
}

// recordTurn persists the user's prompt and the assistant's response for the
// session. Failures are logged rather than returned: a persistence hiccup
// shouldn't turn an otherwise-successful AI execution into a failed command.
// Safe to call on a nil receiver (no-op: no session to record to).
func (es *execSession) recordTurn(ctx context.Context, prompt, response string) {
	if es == nil || es.manager == nil || es.session == nil {
		return
	}
	if err := es.manager.AddMessage(ctx, es.session.ID, session.RoleUser, prompt); err != nil {
		log.Warn("Failed to persist user message to session", "session", es.session.Name, "error", err)
	}
	if err := es.manager.AddMessage(ctx, es.session.ID, session.RoleAssistant, response); err != nil {
		log.Warn("Failed to persist assistant message to session", "session", es.session.Name, "error", err)
	}
}

// sessionMessagesToTypesMessages converts stored session messages into the
// executor's message type for use as executor.Options.History.
func sessionMessagesToTypesMessages(messages []*session.Message) []types.Message {
	converted := make([]types.Message, 0, len(messages))
	for _, m := range messages {
		converted = append(converted, types.Message{Role: m.Role, Content: m.Content})
	}
	return converted
}
