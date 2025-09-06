package vcs

import (
	"context"

	"github.com/charmbracelet/log"
)

// Platform represents a VCS platform type.
type Platform string

const (
	PlatformGitHub      Platform = "github"
	PlatformGitLab      Platform = "gitlab"
	PlatformBitbucket   Platform = "bitbucket"
	PlatformAzureDevOps Platform = "azuredevops"
	PlatformUnknown     Platform = "unknown"
)

// Provider is the main VCS provider interface.
type Provider interface {
	// Core functionality.
	DetectContext() (Context, error)
	CreateCommentManager(ctx Context, logger *log.Logger) CommentManager

	// Optional capabilities - return nil if not supported.
	GetJobSummaryWriter() JobSummaryWriter
	GetArtifactPublisher() ArtifactPublisher

	// Metadata.
	GetPlatform() Platform
	IsAvailable() bool
}

// Context provides VCS-specific context information.
type Context interface {
	GetOwner() string
	GetRepo() string
	GetPRNumber() int
	GetCommentUUID() string
	GetToken() string
	GetEventName() string
	IsSupported() bool
	GetPlatform() Platform
	String() string
}

// CommentManager handles VCS comment operations.
type CommentManager interface {
	PostOrUpdateComment(ctx context.Context, vcsCtx Context, content string) error
	FindExistingComment(ctx context.Context, vcsCtx Context, uuid string) (interface{}, error)
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
