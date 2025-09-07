package mock

import (
	"fmt"
	"os"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/ci"
)

func init() {
	// Register mock integration with the CI factory
	ci.RegisterIntegration("mock", NewMockIntegration)
}

// MockIntegration implements the CI Integration interface for testing.
type MockIntegration struct {
	logger *log.Logger
	config *MockConfig
	mu     sync.RWMutex
}

// MockConfig allows configuring the mock integration's behavior.
type MockConfig struct {
	// Integration behavior
	IsAvailable         bool
	ShouldFailDetection bool
	DetectionError      error

	// Context configuration
	ContextSupported bool
	Owner            string
	Repo             string
	PRNumber         int
	CommentUUID      string
	Token            string
	EventName        string

	// Comment manager behavior
	ShouldFailComment bool
	CommentError      error
	Comments          map[string]string // UUID -> content

	// Job summary behavior
	JobSummarySupported bool
	JobSummaryPath      string
	ShouldFailSummary   bool
	SummaryError        error
	WrittenSummaries    []string

	// Artifact behavior
	ArtifactsSupported bool
	ShouldFailArtifact bool
	ArtifactError      error
	PublishedArtifacts map[string]string // name -> path
}

// NewMockIntegration creates a new mock CI integration.
func NewMockIntegration(logger *log.Logger) ci.Integration {
	return &MockIntegration{
		logger: logger,
		config: DefaultMockConfig(),
	}
}

// NewMockIntegrationWithConfig creates a mock integration with custom configuration.
func NewMockIntegrationWithConfig(logger *log.Logger, config *MockConfig) *MockIntegration {
	return &MockIntegration{
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
func (m *MockIntegration) DetectContext() (ci.Context, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.config.ShouldFailDetection {
		if m.config.DetectionError != nil {
			return nil, m.config.DetectionError
		}
		return nil, ci.ErrContextNotDetected
	}

	return &MockContext{
		config: m.config,
	}, nil
}

// CreateCommentManager creates a mock comment manager.
func (m *MockIntegration) CreateCommentManager(ctx ci.Context, logger *log.Logger) ci.CommentManager {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return &MockCommentManager{
		config: m.config,
		logger: logger,
	}
}

// GetJobSummaryWriter returns a mock job summary writer if supported.
func (m *MockIntegration) GetJobSummaryWriter() ci.JobSummaryWriter {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.config.JobSummarySupported {
		return nil
	}

	return &MockJobSummaryWriter{
		config: m.config,
	}
}

// GetArtifactPublisher returns a mock artifact publisher if supported.
func (m *MockIntegration) GetArtifactPublisher() ci.ArtifactPublisher {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.config.ArtifactsSupported {
		return nil
	}

	return &MockArtifactPublisher{
		config: m.config,
	}
}

// Provider returns the mock provider identifier.
func (m *MockIntegration) Provider() string {
	return "mock"
}

// IsAvailable checks if the mock integration is available.
func (m *MockIntegration) IsAvailable() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check for GOTCHA_USE_MOCK environment variable
	if os.Getenv("GOTCHA_USE_MOCK") == "true" {
		return true
	}

	return m.config.IsAvailable
}

// SetConfig updates the mock configuration (useful for testing).
func (m *MockIntegration) SetConfig(config *MockConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
}

// GetConfig returns the current mock configuration.
func (m *MockIntegration) GetConfig() *MockConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// GetComments returns all stored comments (for testing).
func (m *MockIntegration) GetComments() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid race conditions
	comments := make(map[string]string)
	for k, v := range m.config.Comments {
		comments[k] = v
	}
	return comments
}

// GetWrittenSummaries returns all written summaries (for testing).
func (m *MockIntegration) GetWrittenSummaries() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy
	summaries := make([]string, len(m.config.WrittenSummaries))
	copy(summaries, m.config.WrittenSummaries)
	return summaries
}

// MockContext implements the ci.Context interface for testing.
type MockContext struct {
	config *MockConfig
}

func (c *MockContext) GetOwner() string          { return c.config.Owner }
func (c *MockContext) GetRepo() string           { return c.config.Repo }
func (c *MockContext) GetPRNumber() int          { return c.config.PRNumber }
func (c *MockContext) GetCommentUUID() string    { return c.config.CommentUUID }
func (c *MockContext) GetToken() string          { return c.config.Token }
func (c *MockContext) GetEventName() string      { return c.config.EventName }
func (c *MockContext) IsSupported() bool         { return c.config.ContextSupported }
func (c *MockContext) Provider() string { return "mock" }
func (c *MockContext) String() string {
	return fmt.Sprintf("Mock Context: %s/%s PR#%d", c.config.Owner, c.config.Repo, c.config.PRNumber)
}

// SetCommentUUID allows updating the UUID (for testing discriminator support).
func (c *MockContext) SetCommentUUID(uuid string) {
	c.config.CommentUUID = uuid
}
