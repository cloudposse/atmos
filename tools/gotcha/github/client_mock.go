package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v59/github"
)

// MockClient for testing GitHub operations.
type MockClient struct {
	Comments   []*github.IssueComment
	CallCount  map[string]int
	ListFunc   func(ctx context.Context, owner, repo string, issueNumber int, opts *github.IssueListCommentsOptions) ([]*github.IssueComment, *github.Response, error)
	CreateFunc func(ctx context.Context, owner, repo string, issueNumber int, comment *github.IssueComment) (*github.IssueComment, *github.Response, error)
	UpdateFunc func(ctx context.Context, owner, repo string, commentID int64, comment *github.IssueComment) (*github.IssueComment, *github.Response, error)

	// State tracking
	CreatedComments []*github.IssueComment
	UpdatedComments []*github.IssueComment
}

// NewMockClient creates a new mock client for testing.
func NewMockClient() *MockClient {
	return &MockClient{
		Comments:        make([]*github.IssueComment, 0),
		CallCount:       make(map[string]int),
		CreatedComments: make([]*github.IssueComment, 0),
		UpdatedComments: make([]*github.IssueComment, 0),
	}
}

// ListIssueComments mocks listing comments with pagination support.
func (m *MockClient) ListIssueComments(ctx context.Context, owner, repo string, issueNumber int, opts *github.IssueListCommentsOptions) ([]*github.IssueComment, *github.Response, error) {
	m.CallCount["ListIssueComments"]++

	if m.ListFunc != nil {
		return m.ListFunc(ctx, owner, repo, issueNumber, opts)
	}

	// Default pagination behavior
	perPage := 30
	page := 1

	if opts != nil {
		if opts.PerPage > 0 {
			perPage = opts.PerPage
		}
		if opts.Page > 0 {
			page = opts.Page
		}
	}

	start := (page - 1) * perPage
	end := start + perPage

	if start >= len(m.Comments) {
		// Return empty results for pages beyond available comments
		return []*github.IssueComment{}, &github.Response{
			NextPage: 0,
			LastPage: (len(m.Comments) + perPage - 1) / perPage,
		}, nil
	}

	if end > len(m.Comments) {
		end = len(m.Comments)
	}

	result := m.Comments[start:end]

	// Calculate pagination info
	lastPage := (len(m.Comments) + perPage - 1) / perPage
	nextPage := 0
	if page < lastPage {
		nextPage = page + 1
	}

	resp := &github.Response{
		NextPage: nextPage,
		LastPage: lastPage,
	}

	return result, resp, nil
}

// CreateComment mocks creating a new comment.
func (m *MockClient) CreateComment(ctx context.Context, owner, repo string, issueNumber int, comment *github.IssueComment) (*github.IssueComment, *github.Response, error) {
	m.CallCount["CreateComment"]++

	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, owner, repo, issueNumber, comment)
	}

	// Create a new comment with generated ID
	newID := int64(len(m.Comments) + len(m.CreatedComments) + 1)
	newComment := &github.IssueComment{
		ID:   github.Int64(newID),
		Body: comment.Body,
	}

	m.CreatedComments = append(m.CreatedComments, newComment)

	return newComment, &github.Response{}, nil
}

// UpdateComment mocks updating an existing comment.
func (m *MockClient) UpdateComment(ctx context.Context, owner, repo string, commentID int64, comment *github.IssueComment) (*github.IssueComment, *github.Response, error) {
	m.CallCount["UpdateComment"]++

	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, owner, repo, commentID, comment)
	}

	// Find and update existing comment
	for i, existingComment := range m.Comments {
		if existingComment.ID != nil && *existingComment.ID == commentID {
			updatedComment := &github.IssueComment{
				ID:   github.Int64(commentID),
				Body: comment.Body,
			}
			m.Comments[i] = updatedComment
			m.UpdatedComments = append(m.UpdatedComments, updatedComment)
			return updatedComment, &github.Response{}, nil
		}
	}

	return nil, &github.Response{}, fmt.Errorf("comment with ID %d not found", commentID)
}

// AddComment adds a comment to the mock for testing.
func (m *MockClient) AddComment(id int64, body string) {
	comment := &github.IssueComment{
		ID:   github.Int64(id),
		Body: github.String(body),
	}
	m.Comments = append(m.Comments, comment)
}

// GenerateMockComments creates a specified number of mock comments for pagination testing.
func (m *MockClient) GenerateMockComments(count int, uuidMarker string, targetIndex int) {
	m.Comments = make([]*github.IssueComment, count)

	for i := 0; i < count; i++ {
		body := fmt.Sprintf("Mock comment #%d content", i+1)

		// Insert the UUID marker at the specified index
		if targetIndex >= 0 && i == targetIndex {
			body = fmt.Sprintf("<!-- test-summary-uuid: %s -->\n\n# Test Results\n\nThis is a test summary comment.", uuidMarker)
		}

		m.Comments[i] = &github.IssueComment{
			ID:   github.Int64(int64(i + 1)),
			Body: github.String(body),
		}
	}
}
