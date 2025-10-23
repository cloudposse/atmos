package session

import (
	"context"
	"time"
)

// Storage defines the interface for session persistence.
type Storage interface {
	// Session operations
	CreateSession(ctx context.Context, session *Session) error
	GetSession(ctx context.Context, id string) (*Session, error)
	GetSessionByName(ctx context.Context, projectPath, name string) (*Session, error)
	ListSessions(ctx context.Context, projectPath string, limit int) ([]*Session, error)
	UpdateSession(ctx context.Context, session *Session) error
	DeleteSession(ctx context.Context, id string) error
	DeleteOldSessions(ctx context.Context, olderThan time.Time) (int, error)

	// Message operations
	AddMessage(ctx context.Context, message *Message) error
	GetMessages(ctx context.Context, sessionID string, limit int) ([]*Message, error)
	GetMessageCount(ctx context.Context, sessionID string) (int, error)

	// Context operations
	AddContext(ctx context.Context, item *ContextItem) error
	GetContext(ctx context.Context, sessionID string) ([]*ContextItem, error)
	DeleteContext(ctx context.Context, sessionID string) error

	// Database management
	Close() error
	Migrate() error
}
