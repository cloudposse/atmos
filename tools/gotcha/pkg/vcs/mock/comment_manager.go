package mock

import (
	"context"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/vcs"
)

// MockCommentManager implements vcs.CommentManager for testing.
type MockCommentManager struct {
	config *MockConfig
	logger *log.Logger
	mu     sync.Mutex
}

// PostOrUpdateComment simulates posting or updating a comment.
func (m *MockCommentManager) PostOrUpdateComment(ctx context.Context, vcsCtx vcs.Context, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.config.ShouldFailComment {
		if m.config.CommentError != nil {
			return m.config.CommentError
		}
		return vcs.ErrCommentCreateFailed
	}

	// Extract UUID from context
	uuid := vcsCtx.GetCommentUUID()

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
		"platform", "mock",
		"owner", vcsCtx.GetOwner(),
		"repo", vcsCtx.GetRepo(),
		"pr", vcsCtx.GetPRNumber(),
		"uuid", uuid,
	)

	return nil
}

// FindExistingComment simulates finding an existing comment.
func (m *MockCommentManager) FindExistingComment(ctx context.Context, vcsCtx vcs.Context, uuid string) (interface{}, error) {
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
