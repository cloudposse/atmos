package mock

import (
	"context"
	"errors"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
)

func TestMockIntegration(t *testing.T) {
	logger := log.New(nil)

	t.Run("default configuration", func(t *testing.T) {
		integration := NewMockIntegration(logger).(*MockIntegration)

		// Test IsAvailable
		assert.True(t, integration.IsAvailable())

		// Test Provider
		assert.Equal(t, "mock", integration.Provider())

		// Test DetectContext
		ctx, err := integration.DetectContext()
		assert.NoError(t, err)
		assert.NotNil(t, ctx)
		assert.Equal(t, "mock-owner", ctx.GetOwner())
		assert.Equal(t, "mock-repo", ctx.GetRepo())
		assert.Equal(t, 42, ctx.GetPRNumber())
		assert.Equal(t, "mock-uuid-123", ctx.GetCommentUUID())
		assert.True(t, ctx.IsSupported())

		// Test CreateCommentManager
		cm := integration.CreateCommentManager(ctx, logger)
		assert.NotNil(t, cm)

		// Test GetJobSummaryWriter
		jsw := integration.GetJobSummaryWriter()
		assert.NotNil(t, jsw)
		assert.True(t, jsw.IsJobSummarySupported())

		// Test GetArtifactPublisher (should be nil by default)
		ap := integration.GetArtifactPublisher()
		assert.Nil(t, ap)
	})

	t.Run("custom configuration", func(t *testing.T) {
		config := &MockConfig{
			IsAvailable:         false,
			ContextSupported:    false,
			JobSummarySupported: false,
			ArtifactsSupported:  true,
		}
		integration := NewMockIntegrationWithConfig(logger, config)

		assert.False(t, integration.IsAvailable())

		ctx, err := integration.DetectContext()
		assert.NoError(t, err)
		assert.False(t, ctx.IsSupported())

		jsw := integration.GetJobSummaryWriter()
		assert.Nil(t, jsw)

		ap := integration.GetArtifactPublisher()
		assert.NotNil(t, ap)
	})

	t.Run("detection failure", func(t *testing.T) {
		config := &MockConfig{
			ShouldFailDetection: true,
			DetectionError:      errors.New("custom detection error"),
		}
		integration := NewMockIntegrationWithConfig(logger, config)

		ctx, err := integration.DetectContext()
		assert.Error(t, err)
		assert.Nil(t, ctx)
		assert.Contains(t, err.Error(), "custom detection error")
	})
}

func TestMockCommentManager(t *testing.T) {
	logger := log.New(nil)
	integration := NewMockIntegration(logger).(*MockIntegration)
	ctx, _ := integration.DetectContext()
	cm := integration.CreateCommentManager(ctx, logger)

	t.Run("post new comment", func(t *testing.T) {
		err := cm.PostOrUpdateComment(context.Background(), ctx, "Test comment content")
		assert.NoError(t, err)

		// Verify comment was stored
		comments := integration.GetComments()
		assert.Contains(t, comments, "mock-uuid-123")
		assert.Equal(t, "Test comment content", comments["mock-uuid-123"])
	})

	t.Run("update existing comment", func(t *testing.T) {
		// Post initial comment
		err := cm.PostOrUpdateComment(context.Background(), ctx, "Initial content")
		assert.NoError(t, err)

		// Update comment
		err = cm.PostOrUpdateComment(context.Background(), ctx, "Updated content")
		assert.NoError(t, err)

		// Verify update
		comments := integration.GetComments()
		assert.Equal(t, "Updated content", comments["mock-uuid-123"])
	})

	t.Run("find existing comment", func(t *testing.T) {
		// Post a comment first
		err := cm.PostOrUpdateComment(context.Background(), ctx, "Find me")
		assert.NoError(t, err)

		// Find the comment
		comment, err := cm.FindExistingComment(context.Background(), ctx, "mock-uuid-123")
		assert.NoError(t, err)
		assert.NotNil(t, comment)

		mockComment := comment.(*MockComment)
		assert.Equal(t, "mock-uuid-123", mockComment.UUID)
		assert.Equal(t, "Find me", mockComment.Content)
	})

	t.Run("find non-existent comment", func(t *testing.T) {
		comment, err := cm.FindExistingComment(context.Background(), ctx, "non-existent-uuid")
		assert.NoError(t, err)
		assert.Nil(t, comment)
	})

	t.Run("comment failure", func(t *testing.T) {
		config := &MockConfig{
			IsAvailable:       true,
			ContextSupported:  true,
			ShouldFailComment: true,
			CommentError:      errors.New("API rate limited"),
			Comments:          make(map[string]string),
		}
		integration := NewMockIntegrationWithConfig(logger, config)
		ctx, _ := integration.DetectContext()
		cm := integration.CreateCommentManager(ctx, logger)

		err := cm.PostOrUpdateComment(context.Background(), ctx, "This should fail")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API rate limited")
	})
}

func TestMockJobSummaryWriter(t *testing.T) {
	logger := log.New(nil)
	integration := NewMockIntegration(logger).(*MockIntegration)
	jsw := integration.GetJobSummaryWriter()

	t.Run("write summary", func(t *testing.T) {
		path, err := jsw.WriteJobSummary("# Test Summary\nAll tests passed!")
		assert.NoError(t, err)
		assert.Equal(t, "/tmp/mock-summary.md", path)

		// Verify summary was stored
		summaries := integration.GetWrittenSummaries()
		assert.Len(t, summaries, 1)
		assert.Contains(t, summaries[0], "Test Summary")
	})

	t.Run("multiple summaries", func(t *testing.T) {
		// Create a new integration for this test to avoid state from previous test
		integration := NewMockIntegration(logger).(*MockIntegration)
		jsw := integration.GetJobSummaryWriter()

		jsw.WriteJobSummary("Summary 1")
		jsw.WriteJobSummary("Summary 2")
		jsw.WriteJobSummary("Summary 3")

		summaries := integration.GetWrittenSummaries()
		assert.Len(t, summaries, 3)
		assert.Equal(t, "Summary 1", summaries[0])
		assert.Equal(t, "Summary 2", summaries[1])
		assert.Equal(t, "Summary 3", summaries[2])
	})

	t.Run("summary failure", func(t *testing.T) {
		config := &MockConfig{
			JobSummarySupported: true,
			ShouldFailSummary:   true,
			SummaryError:        errors.New("disk full"),
		}
		integration := NewMockIntegrationWithConfig(logger, config)
		jsw := integration.GetJobSummaryWriter()

		path, err := jsw.WriteJobSummary("This should fail")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "disk full")
		assert.Empty(t, path)
	})
}

func TestMockArtifactPublisher(t *testing.T) {
	logger := log.New(nil)
	config := &MockConfig{
		ArtifactsSupported: true,
		PublishedArtifacts: make(map[string]string),
	}
	integration := NewMockIntegrationWithConfig(logger, config)
	ap := integration.GetArtifactPublisher()

	t.Run("publish artifact", func(t *testing.T) {
		err := ap.PublishArtifact("test-results", "/tmp/results.xml")
		assert.NoError(t, err)

		// Verify artifact was stored
		assert.Equal(t, "/tmp/results.xml", config.PublishedArtifacts["test-results"])
	})

	t.Run("publish multiple artifacts", func(t *testing.T) {
		ap.PublishArtifact("coverage", "/tmp/coverage.html")
		ap.PublishArtifact("logs", "/tmp/test.log")

		assert.Len(t, config.PublishedArtifacts, 3)
		assert.Equal(t, "/tmp/coverage.html", config.PublishedArtifacts["coverage"])
		assert.Equal(t, "/tmp/test.log", config.PublishedArtifacts["logs"])
	})

	t.Run("artifact failure", func(t *testing.T) {
		config.ShouldFailArtifact = true
		config.ArtifactError = errors.New("upload failed")

		err := ap.PublishArtifact("fail", "/tmp/fail.txt")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "upload failed")
	})
}

func TestMockContext(t *testing.T) {
	config := &MockConfig{
		Owner:       "test-owner",
		Repo:        "test-repo",
		PRNumber:    123,
		CommentUUID: "test-uuid",
		Token:       "test-token",
		EventName:   "pull_request",
	}
	ctx := &MockContext{config: config}

	t.Run("context methods", func(t *testing.T) {
		assert.Equal(t, "test-owner", ctx.GetOwner())
		assert.Equal(t, "test-repo", ctx.GetRepo())
		assert.Equal(t, 123, ctx.GetPRNumber())
		assert.Equal(t, "test-uuid", ctx.GetCommentUUID())
		assert.Equal(t, "test-token", ctx.GetToken())
		assert.Equal(t, "pull_request", ctx.GetEventName())
		assert.Equal(t, "mock", ctx.Provider())
		assert.Equal(t, "Mock Context: test-owner/test-repo PR#123", ctx.String())
	})

	t.Run("set comment UUID", func(t *testing.T) {
		ctx.SetCommentUUID("new-uuid-456")
		assert.Equal(t, "new-uuid-456", ctx.GetCommentUUID())
	})
}
