package github

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
	"github.com/spf13/viper"
)

// Context represents GitHub Actions environment information.
type Context struct {
	Owner       string
	Repo        string
	PRNumber    int
	CommentUUID string
	Token       string
	EventName   string
	IsActions   bool
}

// DetectContext checks if running in GitHub Actions and extracts context information.
func DetectContext() (*Context, error) {
	// Check if running in GitHub Actions
	isActions := config.IsGitHubActions()

	if !isActions {
		return nil, ErrNotGitHubActions
	}

	// Parse repository (format: owner/repo)
	repository := config.GetGitHubRepository()
	if repository == "" {
		return nil, ErrRepositoryNotSet
	}

	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("%w: %s (expected owner/repo)", ErrInvalidRepositoryFormat, repository)
	}

	owner := parts[0]
	repo := parts[1]

	// Get event information
	eventName := config.GetGitHubEventName()
	if eventName == "" {
		return nil, ErrEventNameNotSet
	}

	// Get PR number from event payload
	prNumber, err := extractPRNumber(eventName)
	if err != nil {
		return nil, fmt.Errorf("failed to extract PR number: %w", err)
	}

	// Get comment UUID
	commentUUID := config.GetCommentUUID()
	if commentUUID == "" {
		return nil, ErrCommentUUIDNotSet
	}

	// Get GitHub token
	token := config.GetGitHubToken()
	if token == "" {
		return nil, ErrGitHubTokenNotAvailable
	}

	return &Context{
		Owner:       owner,
		Repo:        repo,
		PRNumber:    prNumber,
		CommentUUID: commentUUID,
		Token:       token,
		EventName:   eventName,
		IsActions:   true,
	}, nil
}

// extractPRNumber extracts the PR number from GitHub event payload.
func extractPRNumber(eventName string) (int, error) {
	// For pull_request events, we can get the number from the event payload
	if eventName == "pull_request" || eventName == "pull_request_target" {
		return getPRNumberFromEventPayload()
	}

	// For push events, we need to check if there's a PR associated
	// This is more complex and might require additional API calls
	// For now, return an error for non-PR events
	return 0, fmt.Errorf("%w: '%s'", ErrUnsupportedEventType, eventName)
}

// getPRNumberFromEventPayload reads the GitHub event payload and extracts PR number.
func getPRNumberFromEventPayload() (int, error) {
	// Get event path from configuration
	eventPath := config.GetGitHubEventPath()
	if eventPath == "" {
		return 0, ErrEventPathNotSet
	}

	file, err := os.Open(eventPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open event payload file: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return 0, fmt.Errorf("failed to read event payload: %w", err)
	}

	var payload struct {
		PullRequest struct {
			Number int `json:"number"`
		} `json:"pull_request"`
		Number int `json:"number"` // Direct field for some events
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return 0, fmt.Errorf("failed to parse event payload: %w", err)
	}

	// Try pull_request.number first
	if payload.PullRequest.Number > 0 {
		return payload.PullRequest.Number, nil
	}

	// Try direct number field
	if payload.Number > 0 {
		return payload.Number, nil
	}

	return 0, ErrNoPRNumberInEvent
}

// IsSupported checks if the current GitHub context supports PR comments.
func (c *Context) IsSupported() bool {
	if !c.IsActions {
		return false
	}

	// Support pull_request and pull_request_target events
	supportedEvents := []string{"pull_request", "pull_request_target"}
	for _, event := range supportedEvents {
		if c.EventName == event {
			return true
		}
	}

	return false
}

// String returns a string representation of the context.
func (c *Context) String() string {
	return fmt.Sprintf("GitHub Actions: %s/%s PR#%d (event: %s)",
		c.Owner, c.Repo, c.PRNumber, c.EventName)
}

// GetPRNumberFromEnv attempts to get PR number from environment variables as fallback.
func GetPRNumberFromEnv() (int, error) {
	// Try various environment variables that might contain PR number
	envVars := []string{
		"GITHUB_EVENT_NUMBER",
		"PR_NUMBER",
		"PULL_REQUEST_NUMBER",
	}

	for _, envVar := range envVars {
		_ = viper.BindEnv(envVar)
		if value := viper.GetString(envVar); value != "" {
			if num, err := strconv.Atoi(value); err == nil && num > 0 {
				return num, nil
			}
		}
	}

	return 0, ErrNoPRNumberInEnv
}
