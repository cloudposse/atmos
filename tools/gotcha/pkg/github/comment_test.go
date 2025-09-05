package github

import (
	"context"
	"fmt"
	"strings"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommentManager(t *testing.T) {
	client := NewMockClient()
	logger := log.New(nil)

	manager := NewCommentManager(client, logger)

	assert.NotNil(t, manager)
	assert.Equal(t, client, manager.client)
	assert.Equal(t, logger, manager.logger)
}

func TestFindExistingCommentWithPagination(t *testing.T) {
	tests := []struct {
		name          string
		totalComments int
		targetIndex   int // Where to place the target comment (-1 for no target)
		uuid          string
		expectedFound bool
		expectedID    int64
	}{
		{
			name:          "comment on first page",
			totalComments: 50,
			targetIndex:   10,
			uuid:          "test-uuid-1",
			expectedFound: true,
			expectedID:    11, // targetIndex + 1 (1-based ID)
		},
		{
			name:          "comment on second page",
			totalComments: 150,
			targetIndex:   120, // Will be on page 2 (120/100 = 1.2, so page 2)
			uuid:          "test-uuid-2",
			expectedFound: true,
			expectedID:    121, // targetIndex + 1
		},
		{
			name:          "comment on third page",
			totalComments: 250,
			targetIndex:   220,
			uuid:          "test-uuid-3",
			expectedFound: true,
			expectedID:    221,
		},
		{
			name:          "no matching comment",
			totalComments: 150,
			targetIndex:   -1, // No target comment
			uuid:          "non-existent-uuid",
			expectedFound: false,
		},
		{
			name:          "empty comments",
			totalComments: 0,
			targetIndex:   -1,
			uuid:          "any-uuid",
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockClient()
			client.GenerateMockComments(tt.totalComments, tt.uuid, tt.targetIndex)

			manager := NewCommentManager(client, log.New(nil))
			ctx := context.Background()

			comment, err := manager.FindExistingComment(ctx, "owner", "repo", 123, tt.uuid)

			assert.NoError(t, err)

			if tt.expectedFound {
				require.NotNil(t, comment)
				assert.Equal(t, tt.expectedID, *comment.ID)
				assert.Contains(t, *comment.Body, fmt.Sprintf("test-summary-uuid: %s", tt.uuid))

				// Verify pagination was used correctly
				assert.Greater(t, client.CallCount["ListIssueComments"], 0)

				// For comments beyond first page, verify multiple API calls
				expectedPages := (tt.targetIndex / CommentsPerPage) + 1
				assert.GreaterOrEqual(t, client.CallCount["ListIssueComments"], expectedPages)
			} else {
				assert.Nil(t, comment)
			}
		})
	}
}

func TestFindExistingCommentErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		uuid        string
		mockError   error
		expectError bool
	}{
		{
			name:        "empty UUID",
			uuid:        "",
			expectError: true,
		},
		{
			name:        "API error",
			uuid:        "test-uuid",
			mockError:   fmt.Errorf("API rate limited"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockClient()

			if tt.mockError != nil {
				client.ListFunc = func(ctx context.Context, owner, repo string, issueNumber int, opts *github.IssueListCommentsOptions) ([]*github.IssueComment, *github.Response, error) {
					return nil, nil, tt.mockError
				}
			}

			manager := NewCommentManager(client, log.New(nil))
			ctx := context.Background()

			comment, err := manager.FindExistingComment(ctx, "owner", "repo", 123, tt.uuid)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, comment)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPostOrUpdateComment(t *testing.T) {
	tests := []struct {
		name               string
		existingCommentID  int64
		setupExisting      bool
		markdown           string
		expectedCreateCall bool
		expectedUpdateCall bool
		expectError        bool
	}{
		{
			name:               "create new comment",
			setupExisting:      false,
			markdown:           "<!-- test-summary-uuid: uuid1 -->\n\n# Test Results\n\nNew comment",
			expectedCreateCall: true,
			expectedUpdateCall: false,
		},
		{
			name:               "update existing comment",
			existingCommentID:  123,
			setupExisting:      true,
			markdown:           "<!-- test-summary-uuid: uuid1 -->\n\n# Test Results\n\nUpdated comment",
			expectedCreateCall: false,
			expectedUpdateCall: true,
		},
		{
			name:        "nil context",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockClient()
			manager := NewCommentManager(client, log.New(nil))
			ctx := context.Background()

			var ghCtx *Context
			if !tt.expectError || tt.name != "nil context" {
				ghCtx = &Context{
					Owner:       "owner",
					Repo:        "repo",
					PRNumber:    123,
					CommentUUID: "uuid1",
					EventName:   "pull_request",
					IsActions:   true,
				}
			}

			// Setup existing comment if needed
			if tt.setupExisting {
				client.AddComment(tt.existingCommentID, fmt.Sprintf("<!-- test-summary-uuid: %s -->\n\nExisting comment", "uuid1"))
			}

			err := manager.PostOrUpdateComment(ctx, ghCtx, tt.markdown)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				if tt.expectedCreateCall {
					assert.Equal(t, 1, client.CallCount["CreateComment"])
					assert.Equal(t, 0, client.CallCount["UpdateComment"])
					assert.Len(t, client.CreatedComments, 1)
					assert.Equal(t, tt.markdown, *client.CreatedComments[0].Body)
				}

				if tt.expectedUpdateCall {
					assert.Equal(t, 0, client.CallCount["CreateComment"])
					assert.Equal(t, 1, client.CallCount["UpdateComment"])
					assert.Len(t, client.UpdatedComments, 1)
					assert.Equal(t, tt.markdown, *client.UpdatedComments[0].Body)
				}
			}
		})
	}
}

func TestPostOrUpdateCommentUnsupportedEvent(t *testing.T) {
	client := NewMockClient()
	manager := NewCommentManager(client, log.New(nil))
	ctx := context.Background()

	ghCtx := &Context{
		Owner:     "owner",
		Repo:      "repo",
		PRNumber:  123,
		EventName: "push", // Unsupported event
		IsActions: true,
	}

	err := manager.PostOrUpdateComment(ctx, ghCtx, "test markdown")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "event type 'push' is not supported")
}

func TestPostOrUpdateCommentLargeContent(t *testing.T) {
	client := NewMockClient()
	manager := NewCommentManager(client, log.New(nil))
	ctx := context.Background()

	ghCtx := &Context{
		Owner:       "owner",
		Repo:        "repo",
		PRNumber:    123,
		CommentUUID: "uuid1",
		EventName:   "pull_request",
		IsActions:   true,
	}

	// Create content larger than GitHub's limit
	largeContent := strings.Repeat("a", MaxCommentSize+1000)

	err := manager.PostOrUpdateComment(ctx, ghCtx, largeContent)

	assert.NoError(t, err)
	assert.Equal(t, 1, client.CallCount["CreateComment"])

	// Verify content was truncated
	createdComment := client.CreatedComments[0]
	assert.Less(t, len(*createdComment.Body), MaxCommentSize)
	assert.Contains(t, *createdComment.Body, "Comment truncated due to size limits")
}

func TestTruncateComment(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		maxSize  int
		expected string
	}{
		{
			name:     "content within limit",
			content:  "short content",
			maxSize:  100,
			expected: "short content",
		},
		{
			name:     "content exceeds limit",
			content:  strings.Repeat("a", 100),
			maxSize:  50,
			expected: strings.Repeat("a", 50-len("\n\n---\n*Comment truncated due to size limits*")) + "\n\n---\n*Comment truncated due to size limits*",
		},
		{
			name:     "content with line breaks",
			content:  strings.Repeat("line\n", 20),
			maxSize:  50,
			expected: strings.Repeat("line\n", 8) + "\n\n---\n*Comment truncated due to size limits*",
		},
		{
			name:     "max size too small",
			content:  "any content",
			maxSize:  10,
			expected: "\n\n---\n*Comment truncated due to size limits*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateComment(tt.content, tt.maxSize)
			assert.LessOrEqual(t, len(result), tt.maxSize)

			if len(tt.content) > tt.maxSize {
				assert.Contains(t, result, "Comment truncated due to size limits")
			}
		})
	}
}

func TestRetryLogic(t *testing.T) {
	tests := []struct {
		name          string
		errors        []error
		expectSuccess bool
		expectedCalls int
	}{
		{
			name:          "success on first try",
			errors:        []error{nil},
			expectSuccess: true,
			expectedCalls: 1,
		},
		{
			name: "success after retry",
			errors: []error{
				fmt.Errorf("rate limit exceeded"),
				nil,
			},
			expectSuccess: true,
			expectedCalls: 2,
		},
		{
			name: "failure after all retries",
			errors: []error{
				fmt.Errorf("rate limit exceeded"),
				fmt.Errorf("timeout"),
				fmt.Errorf("server error"),
				fmt.Errorf("still failing"),
			},
			expectSuccess: false,
			expectedCalls: MaxRetries,
		},
		{
			name: "non-retryable error",
			errors: []error{
				fmt.Errorf("not found"),
			},
			expectSuccess: false,
			expectedCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockClient()
			manager := NewCommentManager(client, log.New(nil))
			ctx := context.Background()

			callCount := 0
			client.CreateFunc = func(ctx context.Context, owner, repo string, issueNumber int, comment *github.IssueComment) (*github.IssueComment, *github.Response, error) {
				if callCount < len(tt.errors) {
					err := tt.errors[callCount]
					callCount++
					if err != nil {
						return nil, nil, err
					}
				}
				return &github.IssueComment{ID: github.Int64(123), Body: comment.Body}, &github.Response{}, nil
			}

			ghCtx := &Context{
				Owner:       "owner",
				Repo:        "repo",
				PRNumber:    123,
				CommentUUID: "uuid1",
				EventName:   "pull_request",
				IsActions:   true,
			}

			err := manager.PostOrUpdateComment(ctx, ghCtx, "test markdown")

			if tt.expectSuccess {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}

			assert.Equal(t, tt.expectedCalls, callCount)
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name:      "nil error",
			err:       nil,
			retryable: false,
		},
		{
			name:      "rate limit error",
			err:       fmt.Errorf("rate limit exceeded"),
			retryable: true,
		},
		{
			name:      "too many requests",
			err:       fmt.Errorf("too many requests"),
			retryable: true,
		},
		{
			name:      "timeout error",
			err:       fmt.Errorf("timeout occurred"),
			retryable: true,
		},
		{
			name:      "server error",
			err:       fmt.Errorf("502 bad gateway"),
			retryable: true,
		},
		{
			name:      "not found error",
			err:       fmt.Errorf("not found"),
			retryable: false,
		},
		{
			name:      "permission denied",
			err:       fmt.Errorf("permission denied"),
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.err)
			assert.Equal(t, tt.retryable, result)
		})
	}
}
