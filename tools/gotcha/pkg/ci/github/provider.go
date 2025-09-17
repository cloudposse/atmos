package github

import (
	"context"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/ci"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
)

func init() {
	// Register GitHub integration with the CI factory
	ci.RegisterIntegration(ci.GitHub, NewGitHubIntegration)
}

// GitHubIntegration implements the CI Integration interface for GitHub Actions.
type GitHubIntegration struct {
	logger *log.Logger
}

// NewGitHubIntegration creates a new GitHub CI integration.
func NewGitHubIntegration(logger *log.Logger) ci.Integration {
	return &GitHubIntegration{
		logger: logger,
	}
}

// DetectContext detects GitHub Actions context from environment.
func (g *GitHubIntegration) DetectContext() (ci.Context, error) {
	ctx, err := DetectContext()
	if err != nil {
		return nil, err
	}
	return &gitHubContext{ctx}, nil
}

// CreateCommentManager creates a GitHub comment manager.
func (g *GitHubIntegration) CreateCommentManager(ctx ci.Context, logger *log.Logger) ci.CommentManager {
	// Extract the underlying GitHub context
	ghCtx, ok := ctx.(*gitHubContext)
	if !ok {
		g.logger.Error("Invalid context type for GitHub integration")
		return nil
	}

	client := NewClient(ghCtx.underlying.Token)
	return &gitHubCommentManager{
		manager: NewCommentManager(client, logger),
		logger:  logger,
	}
}

// GetJobSummaryWriter returns a GitHub job summary writer if available.
func (g *GitHubIntegration) GetJobSummaryWriter() ci.JobSummaryWriter {
	if config.GetGitHubStepSummary() != "" {
		return &GitHubJobSummaryWriter{}
	}
	return nil
}

// GetArtifactPublisher returns nil as GitHub Actions handles artifacts differently.
func (g *GitHubIntegration) GetArtifactPublisher() ci.ArtifactPublisher {
	// GitHub Actions handles artifacts through workflow commands, not API
	return nil
}

// Provider returns the GitHub provider identifier.
func (g *GitHubIntegration) Provider() string {
	return ci.GitHub
}

// IsAvailable checks if GitHub Actions environment is available.
func (g *GitHubIntegration) IsAvailable() bool {
	return config.IsGitHubActions()
}

// gitHubContext wraps the GitHub-specific Context to implement ci.Context.
type gitHubContext struct {
	underlying *Context
}

func (c *gitHubContext) GetOwner() string       { return c.underlying.Owner }
func (c *gitHubContext) GetRepo() string        { return c.underlying.Repo }
func (c *gitHubContext) GetPRNumber() int       { return c.underlying.PRNumber }
func (c *gitHubContext) GetCommentUUID() string { return c.underlying.CommentUUID }
func (c *gitHubContext) GetToken() string       { return c.underlying.Token }
func (c *gitHubContext) GetEventName() string   { return c.underlying.EventName }
func (c *gitHubContext) IsSupported() bool      { return c.underlying.IsSupported() }
func (c *gitHubContext) Provider() string       { return ci.GitHub }
func (c *gitHubContext) String() string         { return c.underlying.String() }

// SetCommentUUID allows updating the UUID (needed for job discriminator support).
func (c *gitHubContext) SetCommentUUID(uuid string) {
	c.underlying.CommentUUID = uuid
}

// gitHubCommentManager wraps the GitHub CommentManager to implement ci.CommentManager.
type gitHubCommentManager struct {
	manager *CommentManager
	logger  *log.Logger
}

func (m *gitHubCommentManager) PostOrUpdateComment(ctx context.Context, ciCtx ci.Context, content string) error {
	ghCtx, ok := ciCtx.(*gitHubContext)
	if !ok {
		return ci.ErrContextNotSupported
	}
	return m.manager.PostOrUpdateComment(ctx, ghCtx.underlying, content)
}

func (m *gitHubCommentManager) FindExistingComment(ctx context.Context, ciCtx ci.Context, uuid string) (interface{}, error) {
	ghCtx, ok := ciCtx.(*gitHubContext)
	if !ok {
		return nil, ci.ErrContextNotSupported
	}
	return m.manager.FindExistingComment(ctx, ghCtx.underlying.Owner, ghCtx.underlying.Repo, ghCtx.underlying.PRNumber, uuid)
}
