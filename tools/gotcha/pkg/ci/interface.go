package ci

import (
	"context"

	log "github.com/charmbracelet/log"
)

// Provider name constants for CI providers.
// Only include providers that have actual implementations.
const (
	GitHub = "github"
	// Future providers can be added here as they are implemented:
	// GitLab      = "gitlab"
	// Bitbucket   = "bitbucket"
	// AzureDevOps = "azuredevops"
	// CircleCI    = "circleci".
	Unknown = "unknown"
)

// Integration is the main CI integration interface.
type Integration interface {
	// Core functionality.
	DetectContext() (Context, error)
	CreateCommentManager(ctx Context, logger *log.Logger) CommentManager

	// Optional capabilities - return nil if not supported.
	GetJobSummaryWriter() JobSummaryWriter
	GetArtifactPublisher() ArtifactPublisher

	// Metadata.
	Provider() string
	IsAvailable() bool
}

// Context provides CI-specific context information.
type Context interface {
	GetOwner() string
	GetRepo() string
	GetPRNumber() int
	GetCommentUUID() string
	GetToken() string
	GetEventName() string
	IsSupported() bool
	Provider() string
	String() string
}

// CommentManager handles CI comment operations.
type CommentManager interface {
	PostOrUpdateComment(ctx context.Context, ciCtx Context, content string) error
	FindExistingComment(ctx context.Context, ciCtx Context, uuid string) (interface{}, error)
}

// JobSummaryWriter handles CI/CD job summary generation (optional capability).
type JobSummaryWriter interface {
	WriteJobSummary(content string) (string, error)
	IsJobSummarySupported() bool
	GetJobSummaryPath() string
}

// ArtifactPublisher handles CI/CD artifact publishing (optional capability).
type ArtifactPublisher interface {
	PublishArtifact(name string, path string) error
	IsArtifactSupported() bool
}
