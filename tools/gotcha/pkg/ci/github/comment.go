package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	log "github.com/charmbracelet/log"
	"github.com/google/go-github/v59/github"
)

const (
	// MaxCommentSize is GitHub's limit for comment body size.
	MaxCommentSize = 65536
	// CommentsPerPage is the number of comments to fetch per API call.
	CommentsPerPage = 100
	// MaxRetries is the maximum number of API retries.
	MaxRetries = 3
	// RetryDelay is the base delay between retries.
	RetryDelay = time.Second
)

// CommentManager handles GitHub PR comment operations.
type CommentManager struct {
	client Client
	logger *log.Logger
}

// NewCommentManager creates a new comment manager.
func NewCommentManager(client Client, logger *log.Logger) *CommentManager {
	return &CommentManager{
		client: client,
		logger: logger,
	}
}

// FindExistingComment searches for a comment with the specified UUID marker.
// It handles pagination to search through all comments on the PR.
func (m *CommentManager) FindExistingComment(ctx context.Context, owner, repo string, prNumber int, uuid string) (*github.IssueComment, error) {
	if uuid == "" {
		return nil, ErrUUIDCannotBeEmpty
	}

	marker := fmt.Sprintf("<!-- test-summary-uuid: %s -->", uuid)
	page := 1

	m.logger.Debug("Searching for existing comment",
		"owner", owner,
		"repo", repo,
		"pr", prNumber,
		"uuid", uuid)

	for {
		opts := &github.IssueListCommentsOptions{
			ListOptions: github.ListOptions{
				Page:    page,
				PerPage: CommentsPerPage,
			},
		}

		comments, resp, err := m.retryAPICallList(func() ([]*github.IssueComment, *github.Response, error) {
			return m.client.ListIssueComments(ctx, owner, repo, prNumber, opts)
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list comments (page %d): %w", page, err)
		}

		m.logger.Debug("Retrieved comments page",
			"page", page,
			"count", len(comments),
			"total_pages", resp.LastPage)

		// Search for UUID marker in comment bodies
		for _, comment := range comments {
			if comment.Body != nil && strings.Contains(*comment.Body, marker) {
				m.logger.Debug("Found existing comment",
					"comment_id", *comment.ID,
					"page", page)
				return comment, nil
			}
		}

		// Check if we have more pages
		if resp.NextPage == 0 {
			break
		}

		page = resp.NextPage

		// Safety check to avoid infinite loops
		if page > 1000 { // Reasonable upper limit
			m.logger.Warn("Reached maximum page limit while searching for comment", "page", page)
			break
		}
	}

	m.logger.Debug("No existing comment found with UUID", "uuid", uuid)
	return nil, nil
}

// PostOrUpdateComment creates a new comment or updates an existing one.
func (m *CommentManager) PostOrUpdateComment(ctx context.Context, ghCtx *Context, markdown string) error {
	if ghCtx == nil {
		return ErrGitHubContextIsNil
	}

	if !ghCtx.IsSupported() {
		return fmt.Errorf("%w: '%s'", ErrUnsupportedEventType, ghCtx.EventName)
	}

	// Log comment size for debugging
	if len(markdown) > MaxCommentSize {
		m.logger.Warn("Comment content exceeds GitHub limit but should be pre-sized",
			"size", len(markdown),
			"limit", MaxCommentSize)
		// The generateCommentContent function should have handled sizing,
		// but if we're still over the limit, fall back to simple truncation.
		markdown = truncateComment(markdown, MaxCommentSize)
	} else {
		m.logger.Debug("Comment size within limits",
			"size", len(markdown),
			"limit", MaxCommentSize)
	}

	// Search for existing comment
	existingComment, err := m.FindExistingComment(ctx, ghCtx.Owner, ghCtx.Repo, ghCtx.PRNumber, ghCtx.CommentUUID)
	if err != nil {
		return fmt.Errorf("failed to search for existing comment: %w", err)
	}

	if existingComment != nil {
		// Update existing comment
		m.logger.Info("Updating existing GitHub comment",
			"comment_id", *existingComment.ID,
			"pr", ghCtx.PRNumber)

		updateComment := &github.IssueComment{
			Body: github.String(markdown),
		}

		_, _, err := m.retryAPICall(func() (*github.IssueComment, *github.Response, error) {
			return m.client.UpdateComment(ctx, ghCtx.Owner, ghCtx.Repo, *existingComment.ID, updateComment)
		})
		if err != nil {
			return fmt.Errorf("failed to update comment: %w", err)
		}

		m.logger.Info("Successfully updated GitHub comment", "comment_id", *existingComment.ID)
	} else {
		// Create new comment
		m.logger.Info("Creating new GitHub comment", "pr", ghCtx.PRNumber)

		newComment := &github.IssueComment{
			Body: github.String(markdown),
		}

		createdComment, _, err := m.retryAPICall(func() (*github.IssueComment, *github.Response, error) {
			return m.client.CreateComment(ctx, ghCtx.Owner, ghCtx.Repo, ghCtx.PRNumber, newComment)
		})
		if err != nil {
			return fmt.Errorf("failed to create comment: %w", err)
		}

		m.logger.Info("Successfully created GitHub comment", "comment_id", *createdComment.ID)
	}

	return nil
}

// retryAPICall performs API calls with exponential backoff retry logic.
func (m *CommentManager) retryAPICall(apiCall func() (*github.IssueComment, *github.Response, error)) (*github.IssueComment, *github.Response, error) {
	var lastErr error

	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * RetryDelay
			m.logger.Debug("Retrying API call", "attempt", attempt+1, "delay", delay)
			time.Sleep(delay)
		}

		comment, resp, err := apiCall()
		if err == nil {
			return comment, resp, nil
		}

		lastErr = err

		// Check if this is a retryable error
		if !isRetryableError(err) {
			m.logger.Debug("Non-retryable error encountered", "error", err)
			break
		}

		m.logger.Debug("Retryable error encountered", "attempt", attempt+1, "error", err)
	}

	return nil, nil, fmt.Errorf("API call failed after %d attempts: %w", MaxRetries, lastErr)
}

// retryAPICallList performs list API calls with retry logic.
func (m *CommentManager) retryAPICallList(apiCall func() ([]*github.IssueComment, *github.Response, error)) ([]*github.IssueComment, *github.Response, error) {
	var lastErr error

	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * RetryDelay
			m.logger.Debug("Retrying API call", "attempt", attempt+1, "delay", delay)
			time.Sleep(delay)
		}

		comments, resp, err := apiCall()
		if err == nil {
			return comments, resp, nil
		}

		lastErr = err

		// Check if this is a retryable error
		if !isRetryableError(err) {
			m.logger.Debug("Non-retryable error encountered", "error", err)
			break
		}

		m.logger.Debug("Retryable error encountered", "attempt", attempt+1, "error", err)
	}

	return nil, nil, fmt.Errorf("API call failed after %d attempts: %w", MaxRetries, lastErr)
}

// isRetryableError determines if an error should be retried.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Retry on rate limiting, temporary network issues, and server errors
	retryableErrors := []string{
		"rate limit",
		"too many requests",
		"timeout",
		"temporary failure",
		"server error",
		"502 bad gateway",
		"503 service unavailable",
		"504 gateway timeout",
	}

	for _, retryable := range retryableErrors {
		if strings.Contains(errStr, retryable) {
			return true
		}
	}

	return false
}

// truncateComment truncates a comment to fit within GitHub's size limits.
func truncateComment(content string, maxSize int) string {
	if len(content) <= maxSize {
		return content
	}

	// Reserve space for truncation message
	truncationMsg := "\n\n---\n*Comment truncated due to size limits*"
	
	// If the truncation message itself is too long for maxSize,
	// return as much of it as possible
	if len(truncationMsg) >= maxSize {
		return truncationMsg[:maxSize]
	}
	
	availableSize := maxSize - len(truncationMsg)

	if availableSize <= 0 {
		return truncationMsg
	}

	// Try to truncate at a reasonable boundary (line break)
	truncated := content[:availableSize]
	if lastNewline := strings.LastIndex(truncated, "\n"); lastNewline > availableSize/2 {
		truncated = truncated[:lastNewline]
	}

	return truncated + truncationMsg
}
