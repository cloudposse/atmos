package github

import (
	"fmt"
	"os"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
)

// GitHubJobSummaryWriter implements ci.JobSummaryWriter for GitHub Actions.
type GitHubJobSummaryWriter struct{}

// WriteJobSummary writes content to the GitHub Actions step summary.
func (w *GitHubJobSummaryWriter) WriteJobSummary(content string) (string, error) {
	summaryPath := config.GetGitHubStepSummary()
	if summaryPath == "" {
		return "", nil
	}

	file, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, constants.DefaultFilePerms)
	if err != nil {
		return "", fmt.Errorf("failed to open GitHub step summary file: %w", err)
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		return "", fmt.Errorf("failed to write to GitHub step summary: %w", err)
	}

	return summaryPath, nil
}

// IsJobSummarySupported checks if GitHub step summaries are supported.
func (w *GitHubJobSummaryWriter) IsJobSummarySupported() bool {
	return config.GetGitHubStepSummary() != ""
}

// GetJobSummaryPath returns the path to the GitHub step summary file.
func (w *GitHubJobSummaryWriter) GetJobSummaryPath() string {
	return config.GetGitHubStepSummary()
}
