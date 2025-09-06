package mock

import (
	"fmt"
	"os"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/vcs"
)

func init() {
	// Register mock provider with the VCS factory
	vcs.RegisterProvider(vcs.Platform("mock"), NewMockProvider)
}

// MockProvider implements the VCS Provider interface for testing.
type MockProvider struct {
	logger *log.Logger
	config *MockConfig
	mu     sync.RWMutex
}

// MockConfig allows configuring the mock provider's behavior.
type MockConfig struct {
	// Provider behavior
	IsAvailable          bool
	ShouldFailDetection  bool
	DetectionError       error
	
	// Context configuration
	ContextSupported     bool
	Owner                string
	Repo                 string
	PRNumber             int
	CommentUUID          string
	Token                string
	EventName            string
	
	// Comment manager behavior
	ShouldFailComment    bool
	CommentError         error
	Comments             map[string]string // UUID -> content
	
	// Job summary behavior
	JobSummarySupported  bool
	JobSummaryPath       string
	ShouldFailSummary    bool
	SummaryError         error
	WrittenSummaries     []string
	
	// Artifact behavior
	ArtifactsSupported   bool
	ShouldFailArtifact   bool
	ArtifactError        error
	PublishedArtifacts   map[string]string // name -> path
}

// NewMockProvider creates a new mock VCS provider.
func NewMockProvider(logger *log.Logger) vcs.Provider {
	return &MockProvider{
		logger: logger,
		config: DefaultMockConfig(),
	}
}

// NewMockProviderWithConfig creates a mock provider with custom configuration.
func NewMockProviderWithConfig(logger *log.Logger, config *MockConfig) *MockProvider {
	return &MockProvider{
		logger: logger,
		config: config,
	}
}

// DefaultMockConfig returns a default mock configuration.
func DefaultMockConfig() *MockConfig {
	return &MockConfig{
		IsAvailable:         true,
		ContextSupported:    true,
		Owner:               "mock-owner",
		Repo:                "mock-repo",
		PRNumber:            42,
		CommentUUID:         "mock-uuid-123",
		Token:               "mock-token",
		EventName:           "pull_request",
		Comments:            make(map[string]string),
		JobSummarySupported: true,
		JobSummaryPath:      "/tmp/mock-summary.md",
		WrittenSummaries:    []string{},
		ArtifactsSupported:  false,
		PublishedArtifacts:  make(map[string]string),
	}
}

// DetectContext returns a mock context.
func (p *MockProvider) DetectContext() (vcs.Context, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if p.config.ShouldFailDetection {
		if p.config.DetectionError != nil {
			return nil, p.config.DetectionError
		}
		return nil, vcs.ErrContextNotDetected
	}
	
	return &MockContext{
		config: p.config,
	}, nil
}

// CreateCommentManager creates a mock comment manager.
func (p *MockProvider) CreateCommentManager(ctx vcs.Context, logger *log.Logger) vcs.CommentManager {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	return &MockCommentManager{
		config: p.config,
		logger: logger,
	}
}

// GetJobSummaryWriter returns a mock job summary writer if supported.
func (p *MockProvider) GetJobSummaryWriter() vcs.JobSummaryWriter {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if !p.config.JobSummarySupported {
		return nil
	}
	
	return &MockJobSummaryWriter{
		config: p.config,
	}
}

// GetArtifactPublisher returns a mock artifact publisher if supported.
func (p *MockProvider) GetArtifactPublisher() vcs.ArtifactPublisher {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if !p.config.ArtifactsSupported {
		return nil
	}
	
	return &MockArtifactPublisher{
		config: p.config,
	}
}

// GetPlatform returns the mock platform identifier.
func (p *MockProvider) GetPlatform() vcs.Platform {
	return vcs.Platform("mock")
}

// IsAvailable checks if the mock provider is available.
func (p *MockProvider) IsAvailable() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	// Check for GOTCHA_USE_MOCK environment variable
	if os.Getenv("GOTCHA_USE_MOCK") == "true" {
		return true
	}
	
	return p.config.IsAvailable
}

// SetConfig updates the mock configuration (useful for testing).
func (p *MockProvider) SetConfig(config *MockConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config = config
}

// GetConfig returns the current mock configuration.
func (p *MockProvider) GetConfig() *MockConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

// GetComments returns all stored comments (for testing).
func (p *MockProvider) GetComments() map[string]string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	// Return a copy to avoid race conditions
	comments := make(map[string]string)
	for k, v := range p.config.Comments {
		comments[k] = v
	}
	return comments
}

// GetWrittenSummaries returns all written summaries (for testing).
func (p *MockProvider) GetWrittenSummaries() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	// Return a copy
	summaries := make([]string, len(p.config.WrittenSummaries))
	copy(summaries, p.config.WrittenSummaries)
	return summaries
}

// MockContext implements the vcs.Context interface for testing.
type MockContext struct {
	config *MockConfig
}

func (c *MockContext) GetOwner() string       { return c.config.Owner }
func (c *MockContext) GetRepo() string        { return c.config.Repo }
func (c *MockContext) GetPRNumber() int       { return c.config.PRNumber }
func (c *MockContext) GetCommentUUID() string { return c.config.CommentUUID }
func (c *MockContext) GetToken() string       { return c.config.Token }
func (c *MockContext) GetEventName() string   { return c.config.EventName }
func (c *MockContext) IsSupported() bool      { return c.config.ContextSupported }
func (c *MockContext) GetPlatform() vcs.Platform { return vcs.Platform("mock") }
func (c *MockContext) String() string {
	return fmt.Sprintf("Mock Context: %s/%s PR#%d", c.config.Owner, c.config.Repo, c.config.PRNumber)
}

// SetCommentUUID allows updating the UUID (for testing discriminator support).
func (c *MockContext) SetCommentUUID(uuid string) {
	c.config.CommentUUID = uuid
}