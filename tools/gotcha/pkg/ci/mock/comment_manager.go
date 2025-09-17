package mock

import (
	"context"
	"sync"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/gotcha/pkg/ci"
)

// MockCommentManager implements ci.CommentManager for testing.
type MockCommentManager struct {
	config *MockConfig
	logger *log.Logger
	mu     sync.Mutex
}

// PostOrUpdateComment simulates posting or updating a comment.
func (m *MockCommentManager) PostOrUpdateComment(ctx context.Context, ciCtx ci.Context, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.config.ShouldFailComment {
		if m.config.CommentError != nil {
			return m.config.CommentError
		}
		return ci.ErrCommentCreateFailed
	}

	// Extract UUID from context
	uuid := ciCtx.GetCommentUUID()

	// Store or update the comment
	if m.config.Comments == nil {
		m.config.Comments = make(map[string]string)
	}

	// Check if comment exists (update) or new (create)
	if _, exists := m.config.Comments[uuid]; exists {
		m.logger.Debug("Updating mock comment", "uuid", uuid, "size", len(content))
	} else {
		m.logger.Debug("Creating new mock comment", "uuid", uuid, "size", len(content))
	}

	m.config.Comments[uuid] = content

	m.logger.Info("Mock comment posted successfully",
		"provider", "mock",
		"owner", ciCtx.GetOwner(),
		"repo", ciCtx.GetRepo(),
		"pr", ciCtx.GetPRNumber(),
		"uuid", uuid,
	)

	return nil
}

// FindExistingComment simulates finding an existing comment.
func (m *MockCommentManager) FindExistingComment(ctx context.Context, ciCtx ci.Context, uuid string) (interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.config.Comments == nil {
		return nil, nil
	}

	if content, exists := m.config.Comments[uuid]; exists {
		// Return a mock comment structure
		return &MockComment{
			UUID:    uuid,
			Content: content,
		}, nil
	}

	return nil, nil
}

// MockComment represents a mock comment for testing.
type MockComment struct {
	UUID    string
	Content string
}
