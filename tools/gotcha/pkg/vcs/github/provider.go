package github

import (
	"context"
	"os"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/vcs"
)

func init() {
	// Register GitHub provider with the VCS factory
	vcs.RegisterProvider(vcs.PlatformGitHub, NewGitHubProvider)
}

// GitHubProvider implements the VCS Provider interface for GitHub.
type GitHubProvider struct {
	logger *log.Logger
}

// NewGitHubProvider creates a new GitHub VCS provider.
func NewGitHubProvider(logger *log.Logger) vcs.Provider {
	return &GitHubProvider{
		logger: logger,
	}
}

// DetectContext detects GitHub Actions context from environment.
func (p *GitHubProvider) DetectContext() (vcs.Context, error) {
	ctx, err := DetectContext()
	if err != nil {
		return nil, err
	}
	return &gitHubContext{ctx}, nil
}

// CreateCommentManager creates a GitHub comment manager.
func (p *GitHubProvider) CreateCommentManager(ctx vcs.Context, logger *log.Logger) vcs.CommentManager {
	// Extract the underlying GitHub context
	ghCtx, ok := ctx.(*gitHubContext)
	if !ok {
		p.logger.Error("Invalid context type for GitHub provider")
		return nil
	}
	
	client := NewClient(ghCtx.underlying.Token)
	return &gitHubCommentManager{
		manager: NewCommentManager(client, logger),
		logger:  logger,
	}
}

// GetJobSummaryWriter returns a GitHub job summary writer if available.
func (p *GitHubProvider) GetJobSummaryWriter() vcs.JobSummaryWriter {
	if os.Getenv("GITHUB_STEP_SUMMARY") != "" {
		return &GitHubJobSummaryWriter{}
	}
	return nil
}

// GetArtifactPublisher returns nil as GitHub Actions handles artifacts differently.
func (p *GitHubProvider) GetArtifactPublisher() vcs.ArtifactPublisher {
	// GitHub Actions handles artifacts through workflow commands, not API
	return nil
}

// GetPlatform returns the GitHub platform identifier.
func (p *GitHubProvider) GetPlatform() vcs.Platform {
	return vcs.PlatformGitHub
}

// IsAvailable checks if GitHub Actions environment is available.
func (p *GitHubProvider) IsAvailable() bool {
	return os.Getenv("GITHUB_ACTIONS") != ""
}

// gitHubContext wraps the GitHub-specific Context to implement vcs.Context.
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
func (c *gitHubContext) GetPlatform() vcs.Platform { return vcs.PlatformGitHub }
func (c *gitHubContext) String() string         { return c.underlying.String() }

// SetCommentUUID allows updating the UUID (needed for job discriminator support).
func (c *gitHubContext) SetCommentUUID(uuid string) {
	c.underlying.CommentUUID = uuid
}

// gitHubCommentManager wraps the GitHub CommentManager to implement vcs.CommentManager.
type gitHubCommentManager struct {
	manager *CommentManager
	logger  *log.Logger
}

func (m *gitHubCommentManager) PostOrUpdateComment(ctx context.Context, vcsCtx vcs.Context, content string) error {
	ghCtx, ok := vcsCtx.(*gitHubContext)
	if !ok {
		return vcs.ErrContextNotSupported
	}
	return m.manager.PostOrUpdateComment(ctx, ghCtx.underlying, content)
}

func (m *gitHubCommentManager) FindExistingComment(ctx context.Context, vcsCtx vcs.Context, uuid string) (interface{}, error) {
	ghCtx, ok := vcsCtx.(*gitHubContext)
	if !ok {
		return nil, vcs.ErrContextNotSupported
	}
	return m.manager.FindExistingComment(ctx, ghCtx.underlying.Owner, ghCtx.underlying.Repo, ghCtx.underlying.PRNumber, uuid)
}