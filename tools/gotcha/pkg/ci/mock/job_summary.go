package mock

import (
	"fmt"
	"os"
	"sync"
)

// MockJobSummaryWriter implements ci.JobSummaryWriter for testing.
type MockJobSummaryWriter struct {
	config *MockConfig
	mu     sync.Mutex
}

// WriteJobSummary simulates writing a job summary.
func (w *MockJobSummaryWriter) WriteJobSummary(content string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.config.ShouldFailSummary {
		if w.config.SummaryError != nil {
			return "", w.config.SummaryError
		}
		return "", fmt.Errorf("mock summary write failed")
	}

	// Store the summary content
	w.config.WrittenSummaries = append(w.config.WrittenSummaries, content)

	// Optionally write to actual file for testing
	if w.config.JobSummaryPath != "" && w.config.JobSummaryPath != "/tmp/mock-summary.md" {
		err := os.WriteFile(w.config.JobSummaryPath, []byte(content), 0o644)
		if err != nil {
			return "", err
		}
	}

	return w.config.JobSummaryPath, nil
}

// IsJobSummarySupported checks if job summaries are supported.
func (w *MockJobSummaryWriter) IsJobSummarySupported() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.config.JobSummarySupported
}

// GetJobSummaryPath returns the mock job summary path.
func (w *MockJobSummaryWriter) GetJobSummaryPath() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.config.JobSummaryPath
}

// MockArtifactPublisher implements ci.ArtifactPublisher for testing.
type MockArtifactPublisher struct {
	config *MockConfig
	mu     sync.Mutex
}

// PublishArtifact simulates publishing an artifact.
func (p *MockArtifactPublisher) PublishArtifact(name string, path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.config.ShouldFailArtifact {
		if p.config.ArtifactError != nil {
			return p.config.ArtifactError
		}
		return fmt.Errorf("mock artifact publish failed")
	}

	// Store the artifact information
	if p.config.PublishedArtifacts == nil {
		p.config.PublishedArtifacts = make(map[string]string)
	}
	p.config.PublishedArtifacts[name] = path

	return nil
}

// IsArtifactSupported checks if artifacts are supported.
func (p *MockArtifactPublisher) IsArtifactSupported() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.config.ArtifactsSupported
}
