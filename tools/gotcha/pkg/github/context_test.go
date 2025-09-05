package github

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectContext(t *testing.T) {
	// Save original environment
	originalEnv := map[string]string{
		"GITHUB_ACTIONS":      os.Getenv("GITHUB_ACTIONS"),
		"GITHUB_REPOSITORY":   os.Getenv("GITHUB_REPOSITORY"),
		"GITHUB_EVENT_NAME":   os.Getenv("GITHUB_EVENT_NAME"),
		"GITHUB_EVENT_PATH":   os.Getenv("GITHUB_EVENT_PATH"),
		"GOTCHA_COMMENT_UUID": os.Getenv("GOTCHA_COMMENT_UUID"),
		"GOTCHA_GITHUB_TOKEN": os.Getenv("GOTCHA_GITHUB_TOKEN"),
		"GITHUB_TOKEN":        os.Getenv("GITHUB_TOKEN"),
	}

	// Clean up after test
	defer func() {
		for key, value := range originalEnv {
			if value != "" {
				os.Setenv(key, value)
			} else {
				os.Unsetenv(key)
			}
		}
		viper.Reset()
	}()

	tests := []struct {
		name        string
		env         map[string]string
		eventData   string
		expectError bool
		expected    *Context
	}{
		{
			name: "valid pull_request context",
			env: map[string]string{
				"GITHUB_ACTIONS":      "true",
				"GITHUB_REPOSITORY":   "owner/repo",
				"GITHUB_EVENT_NAME":   "pull_request",
				"GOTCHA_COMMENT_UUID": "test-uuid-123",
				"GITHUB_TOKEN":        "test-token",
			},
			eventData: `{"pull_request": {"number": 123}}`,
			expected: &Context{
				Owner:       "owner",
				Repo:        "repo",
				PRNumber:    123,
				CommentUUID: "test-uuid-123",
				Token:       "test-token",
				EventName:   "pull_request",
				IsActions:   true,
			},
		},
		{
			name: "valid pull_request_target context",
			env: map[string]string{
				"GITHUB_ACTIONS":      "true",
				"GITHUB_REPOSITORY":   "cloudposse/atmos",
				"GITHUB_EVENT_NAME":   "pull_request_target",
				"GOTCHA_COMMENT_UUID": "uuid-456",
				"GOTCHA_GITHUB_TOKEN": "gotcha-token",
			},
			eventData: `{"number": 456}`,
			expected: &Context{
				Owner:       "cloudposse",
				Repo:        "atmos",
				PRNumber:    456,
				CommentUUID: "uuid-456",
				Token:       "gotcha-token",
				EventName:   "pull_request_target",
				IsActions:   true,
			},
		},
		{
			name:        "not in GitHub Actions",
			env:         map[string]string{},
			expectError: true,
		},
		{
			name: "missing repository",
			env: map[string]string{
				"GITHUB_ACTIONS": "true",
			},
			expectError: true,
		},
		{
			name: "invalid repository format",
			env: map[string]string{
				"GITHUB_ACTIONS":    "true",
				"GITHUB_REPOSITORY": "invalid-format",
			},
			expectError: true,
		},
		{
			name: "missing event name",
			env: map[string]string{
				"GITHUB_ACTIONS":    "true",
				"GITHUB_REPOSITORY": "owner/repo",
			},
			expectError: true,
		},
		{
			name: "unsupported event type",
			env: map[string]string{
				"GITHUB_ACTIONS":    "true",
				"GITHUB_REPOSITORY": "owner/repo",
				"GITHUB_EVENT_NAME": "push",
			},
			expectError: true,
		},
		{
			name: "missing UUID",
			env: map[string]string{
				"GITHUB_ACTIONS":    "true",
				"GITHUB_REPOSITORY": "owner/repo",
				"GITHUB_EVENT_NAME": "pull_request",
			},
			eventData:   `{"pull_request": {"number": 123}}`,
			expectError: true,
		},
		{
			name: "missing token",
			env: map[string]string{
				"GITHUB_ACTIONS":      "true",
				"GITHUB_REPOSITORY":   "owner/repo",
				"GITHUB_EVENT_NAME":   "pull_request",
				"GOTCHA_COMMENT_UUID": "test-uuid",
			},
			eventData:   `{"pull_request": {"number": 123}}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			for key := range originalEnv {
				os.Unsetenv(key)
			}
			viper.Reset()

			// Set test environment
			for key, value := range tt.env {
				os.Setenv(key, value)
			}

			// Create event file if needed
			var eventFile *os.File
			if tt.eventData != "" {
				var err error
				eventFile, err = os.CreateTemp("", "github_event_*.json")
				require.NoError(t, err)
				defer os.Remove(eventFile.Name())

				_, err = eventFile.WriteString(tt.eventData)
				require.NoError(t, err)
				eventFile.Close()

				os.Setenv("GITHUB_EVENT_PATH", eventFile.Name())
			}

			ctx, err := DetectContext()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, ctx)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, ctx)
				assert.Equal(t, tt.expected.Owner, ctx.Owner)
				assert.Equal(t, tt.expected.Repo, ctx.Repo)
				assert.Equal(t, tt.expected.PRNumber, ctx.PRNumber)
				assert.Equal(t, tt.expected.CommentUUID, ctx.CommentUUID)
				assert.Equal(t, tt.expected.Token, ctx.Token)
				assert.Equal(t, tt.expected.EventName, ctx.EventName)
				assert.Equal(t, tt.expected.IsActions, ctx.IsActions)
			}
		})
	}
}

func TestContextIsSupported(t *testing.T) {
	tests := []struct {
		name      string
		context   *Context
		supported bool
	}{
		{
			name: "pull_request event",
			context: &Context{
				EventName: "pull_request",
				IsActions: true,
			},
			supported: true,
		},
		{
			name: "pull_request_target event",
			context: &Context{
				EventName: "pull_request_target",
				IsActions: true,
			},
			supported: true,
		},
		{
			name: "push event",
			context: &Context{
				EventName: "push",
				IsActions: true,
			},
			supported: false,
		},
		{
			name: "not in actions",
			context: &Context{
				EventName: "pull_request",
				IsActions: false,
			},
			supported: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.context.IsSupported()
			assert.Equal(t, tt.supported, result)
		})
	}
}

func TestContextString(t *testing.T) {
	ctx := &Context{
		Owner:     "cloudposse",
		Repo:      "atmos",
		PRNumber:  123,
		EventName: "pull_request",
	}

	expected := "GitHub Actions: cloudposse/atmos PR#123 (event: pull_request)"
	assert.Equal(t, expected, ctx.String())
}

func TestGetPRNumberFromEnv(t *testing.T) {
	// Save original environment
	originalEnv := map[string]string{
		"GITHUB_EVENT_NUMBER": os.Getenv("GITHUB_EVENT_NUMBER"),
		"PR_NUMBER":           os.Getenv("PR_NUMBER"),
		"PULL_REQUEST_NUMBER": os.Getenv("PULL_REQUEST_NUMBER"),
	}

	// Clean up after test
	defer func() {
		for key, value := range originalEnv {
			if value != "" {
				os.Setenv(key, value)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	tests := []struct {
		name        string
		env         map[string]string
		expected    int
		expectError bool
	}{
		{
			name: "GITHUB_EVENT_NUMBER",
			env: map[string]string{
				"GITHUB_EVENT_NUMBER": "123",
			},
			expected: 123,
		},
		{
			name: "PR_NUMBER",
			env: map[string]string{
				"PR_NUMBER": "456",
			},
			expected: 456,
		},
		{
			name: "PULL_REQUEST_NUMBER",
			env: map[string]string{
				"PULL_REQUEST_NUMBER": "789",
			},
			expected: 789,
		},
		{
			name:        "no env vars set",
			env:         map[string]string{},
			expectError: true,
		},
		{
			name: "invalid number",
			env: map[string]string{
				"PR_NUMBER": "not-a-number",
			},
			expectError: true,
		},
		{
			name: "zero number",
			env: map[string]string{
				"PR_NUMBER": "0",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			for key := range originalEnv {
				os.Unsetenv(key)
			}

			// Set test environment
			for key, value := range tt.env {
				os.Setenv(key, value)
			}

			result, err := GetPRNumberFromEnv()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
