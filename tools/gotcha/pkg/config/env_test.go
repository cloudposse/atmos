package config

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestInitEnvironment(t *testing.T) {
	// Reset viper to ensure clean state
	viper.Reset()
	
	// Test that InitEnvironment sets up all bindings and defaults
	InitEnvironment()
	
	// Check that defaults are set
	assert.False(t, viper.GetBool("ci"))
	assert.False(t, viper.GetBool("github.actions"))
	assert.False(t, viper.GetBool("use.mock"))
	assert.False(t, viper.GetBool("no.color"))
	assert.False(t, viper.GetBool("force.color"))
	assert.False(t, viper.GetBool("force.tty"))
	assert.False(t, viper.GetBool("force.no.tty"))
}

func TestIsCI(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected bool
	}{
		{
			name:     "no CI environment",
			envVars:  map[string]string{},
			expected: false,
		},
		{
			name: "CI environment variable set",
			envVars: map[string]string{
				"CI": "true",
			},
			expected: true,
		},
		{
			name: "GitHub Actions environment",
			envVars: map[string]string{
				"GITHUB_ACTIONS": "true",
			},
			expected: true,
		},
		{
			name: "Jenkins environment",
			envVars: map[string]string{
				"JENKINS_URL": "http://jenkins.example.com",
			},
			expected: true,
		},
		{
			name: "Travis CI environment",
			envVars: map[string]string{
				"TRAVIS": "true",
			},
			expected: true,
		},
		{
			name: "CircleCI environment",
			envVars: map[string]string{
				"CIRCLECI": "true",
			},
			expected: true,
		},
		{
			name: "CONTINUOUS_INTEGRATION set",
			envVars: map[string]string{
				"CONTINUOUS_INTEGRATION": "true",
			},
			expected: true,
		},
		{
			name: "BUILD_NUMBER set",
			envVars: map[string]string{
				"BUILD_NUMBER": "123",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper and environment
			viper.Reset()
			clearTestEnv()
			
			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearTestEnv()
			
			// Initialize environment bindings
			InitEnvironment()
			
			// Test IsCI
			result := IsCI()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsCIEnabled(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected bool
	}{
		{
			name:     "CI not enabled",
			envVars:  map[string]string{},
			expected: false,
		},
		{
			name: "CI enabled via GOTCHA_CI",
			envVars: map[string]string{
				"GOTCHA_CI": "true",
			},
			expected: true,
		},
		{
			name: "CI disabled via GOTCHA_CI",
			envVars: map[string]string{
				"GOTCHA_CI": "false",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper and environment
			viper.Reset()
			clearTestEnv()
			
			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearTestEnv()
			
			// Initialize environment bindings
			InitEnvironment()
			
			// Test IsCIEnabled
			result := IsCIEnabled()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsGitHubActions(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected bool
	}{
		{
			name:     "not GitHub Actions",
			envVars:  map[string]string{},
			expected: false,
		},
		{
			name: "GitHub Actions environment",
			envVars: map[string]string{
				"GITHUB_ACTIONS": "true",
			},
			expected: true,
		},
		{
			name: "GitHub Actions false",
			envVars: map[string]string{
				"GITHUB_ACTIONS": "false",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper and environment
			viper.Reset()
			clearTestEnv()
			
			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearTestEnv()
			
			// Initialize environment bindings
			InitEnvironment()
			
			// Test IsGitHubActions
			result := IsGitHubActions()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsGitHubActionsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected bool
	}{
		{
			name:     "GitHub Actions not enabled",
			envVars:  map[string]string{},
			expected: false,
		},
		{
			name: "GitHub Actions enabled via GOTCHA_GITHUB_ACTIONS",
			envVars: map[string]string{
				"GOTCHA_GITHUB_ACTIONS": "true",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper and environment
			viper.Reset()
			clearTestEnv()
			
			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearTestEnv()
			
			// Initialize environment bindings
			InitEnvironment()
			
			// Test IsGitHubActionsEnabled
			result := IsGitHubActionsEnabled()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetGitHubToken(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "no token",
			envVars:  map[string]string{},
			expected: "",
		},
		{
			name: "GITHUB_TOKEN set",
			envVars: map[string]string{
				"GITHUB_TOKEN": "ghp_test123",
			},
			expected: "ghp_test123",
		},
		{
			name: "GOTCHA_GITHUB_TOKEN overrides GITHUB_TOKEN",
			envVars: map[string]string{
				"GITHUB_TOKEN":        "ghp_old",
				"GOTCHA_GITHUB_TOKEN": "ghp_new",
			},
			expected: "ghp_new",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper and environment
			viper.Reset()
			clearTestEnv()
			
			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearTestEnv()
			
			// Initialize environment bindings
			InitEnvironment()
			
			// Test GetGitHubToken
			result := GetGitHubToken()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetCommentUUID(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "no UUID",
			envVars:  map[string]string{},
			expected: "",
		},
		{
			name: "COMMENT_UUID set",
			envVars: map[string]string{
				"COMMENT_UUID": "uuid-123",
			},
			expected: "uuid-123",
		},
		{
			name: "GOTCHA_COMMENT_UUID overrides COMMENT_UUID",
			envVars: map[string]string{
				"COMMENT_UUID":        "uuid-old",
				"GOTCHA_COMMENT_UUID": "uuid-new",
			},
			expected: "uuid-new",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper and environment
			viper.Reset()
			clearTestEnv()
			
			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearTestEnv()
			
			// Initialize environment bindings
			InitEnvironment()
			
			// Test GetCommentUUID
			result := GetCommentUUID()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUseMock(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected bool
	}{
		{
			name:     "mock not enabled",
			envVars:  map[string]string{},
			expected: false,
		},
		{
			name: "mock enabled",
			envVars: map[string]string{
				"GOTCHA_USE_MOCK": "true",
			},
			expected: true,
		},
		{
			name: "mock disabled",
			envVars: map[string]string{
				"GOTCHA_USE_MOCK": "false",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper and environment
			viper.Reset()
			clearTestEnv()
			
			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearTestEnv()
			
			// Initialize environment bindings
			InitEnvironment()
			
			// Test UseMock
			result := UseMock()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestColorSettings(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		noColor     bool
		forceColor  bool
	}{
		{
			name:        "default settings",
			envVars:     map[string]string{},
			noColor:     false,
			forceColor:  false,
		},
		{
			name: "NO_COLOR set",
			envVars: map[string]string{
				"NO_COLOR": "1",
			},
			noColor:    true,
			forceColor: false,
		},
		{
			name: "FORCE_COLOR set",
			envVars: map[string]string{
				"FORCE_COLOR": "1",
			},
			noColor:    false,
			forceColor: true,
		},
		{
			name: "both set (FORCE_COLOR wins)",
			envVars: map[string]string{
				"NO_COLOR":    "1",
				"FORCE_COLOR": "1",
			},
			noColor:    true,
			forceColor: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper and environment
			viper.Reset()
			clearTestEnv()
			
			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearTestEnv()
			
			// Initialize environment bindings
			InitEnvironment()
			
			// Test color settings
			assert.Equal(t, tt.noColor, NoColor())
			assert.Equal(t, tt.forceColor, ForceColor())
		})
	}
}

func TestTTYSettings(t *testing.T) {
	tests := []struct {
		name       string
		envVars    map[string]string
		forceTTY   bool
		forceNoTTY bool
	}{
		{
			name:       "default settings",
			envVars:    map[string]string{},
			forceTTY:   false,
			forceNoTTY: false,
		},
		{
			name: "GOTCHA_FORCE_TTY set",
			envVars: map[string]string{
				"GOTCHA_FORCE_TTY": "true",
			},
			forceTTY:   true,
			forceNoTTY: false,
		},
		{
			name: "GOTCHA_FORCE_NO_TTY set",
			envVars: map[string]string{
				"GOTCHA_FORCE_NO_TTY": "true",
			},
			forceTTY:   false,
			forceNoTTY: true,
		},
		{
			name: "FORCE_TTY set (alternative)",
			envVars: map[string]string{
				"FORCE_TTY": "true",
			},
			forceTTY:   true,
			forceNoTTY: false,
		},
		{
			name: "FORCE_NO_TTY set (alternative)",
			envVars: map[string]string{
				"FORCE_NO_TTY": "true",
			},
			forceTTY:   false,
			forceNoTTY: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper and environment
			viper.Reset()
			clearTestEnv()
			
			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearTestEnv()
			
			// Initialize environment bindings
			InitEnvironment()
			
			// Test TTY settings
			assert.Equal(t, tt.forceTTY, ForceTTY())
			assert.Equal(t, tt.forceNoTTY, ForceNoTTY())
		})
	}
}

func TestGitHubEnvironmentVariables(t *testing.T) {
	tests := []struct {
		name       string
		envVars    map[string]string
		repository string
		eventName  string
		eventPath  string
		summary    string
	}{
		{
			name:       "no GitHub environment",
			envVars:    map[string]string{},
			repository: "",
			eventName:  "",
			eventPath:  "",
			summary:    "",
		},
		{
			name: "GitHub environment set",
			envVars: map[string]string{
				"GITHUB_REPOSITORY":     "owner/repo",
				"GITHUB_EVENT_NAME":     "push",
				"GITHUB_EVENT_PATH":     "/tmp/event.json",
				"GITHUB_STEP_SUMMARY":   "/tmp/summary.md",
			},
			repository: "owner/repo",
			eventName:  "push",
			eventPath:  "/tmp/event.json",
			summary:    "/tmp/summary.md",
		},
		{
			name: "GOTCHA overrides",
			envVars: map[string]string{
				"GITHUB_REPOSITORY":        "owner/repo",
				"GOTCHA_GITHUB_REPOSITORY": "owner/other",
				"GITHUB_EVENT_NAME":        "push",
				"GOTCHA_GITHUB_EVENT_NAME": "pull_request",
			},
			repository: "owner/other",
			eventName:  "pull_request",
			eventPath:  "",
			summary:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper and environment
			viper.Reset()
			clearTestEnv()
			
			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearTestEnv()
			
			// Initialize environment bindings
			InitEnvironment()
			
			// Test GitHub environment variables
			assert.Equal(t, tt.repository, GetGitHubRepository())
			assert.Equal(t, tt.eventName, GetGitHubEventName())
			assert.Equal(t, tt.eventPath, GetGitHubEventPath())
			assert.Equal(t, tt.summary, GetGitHubStepSummary())
		})
	}
}

func TestOutputConfiguration(t *testing.T) {
	tests := []struct {
		name       string
		envVars    map[string]string
		output     string
		showFilter string
		ciProvider string
	}{
		{
			name:       "no configuration",
			envVars:    map[string]string{},
			output:     "",
			showFilter: "",
			ciProvider: "",
		},
		{
			name: "output configuration set",
			envVars: map[string]string{
				"GOTCHA_OUTPUT":      "json",
				"GOTCHA_SHOW":        "failed",
				"GOTCHA_CI_PROVIDER": "github",
			},
			output:     "json",
			showFilter: "failed",
			ciProvider: "github",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper and environment
			viper.Reset()
			clearTestEnv()
			
			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearTestEnv()
			
			// Initialize environment bindings
			InitEnvironment()
			
			// Test output configuration
			assert.Equal(t, tt.output, GetOutput())
			assert.Equal(t, tt.showFilter, GetShowFilter())
			assert.Equal(t, tt.ciProvider, GetCIProvider())
		})
	}
}

func TestDebugConfiguration(t *testing.T) {
	tests := []struct {
		name         string
		envVars      map[string]string
		debugFile    string
		columns      string
		splitStreams bool
	}{
		{
			name:         "no debug configuration",
			envVars:      map[string]string{},
			debugFile:    "",
			columns:      "",
			splitStreams: false,
		},
		{
			name: "debug configuration set",
			envVars: map[string]string{
				"GOTCHA_DEBUG_FILE":   "/tmp/debug.log",
				"COLUMNS":             "120",
				"GOTCHA_SPLIT_STREAMS": "true",
			},
			debugFile:    "/tmp/debug.log",
			columns:      "120",
			splitStreams: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper and environment
			viper.Reset()
			clearTestEnv()
			
			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearTestEnv()
			
			// Initialize environment bindings
			InitEnvironment()
			
			// Test debug configuration
			assert.Equal(t, tt.debugFile, GetDebugFile())
			assert.Equal(t, tt.columns, GetColumns())
			assert.Equal(t, tt.splitStreams, IsSplitStreams())
		})
	}
}

func TestEnvironmentPrecedence(t *testing.T) {
	// Test that GOTCHA_ prefixed variables take precedence
	viper.Reset()
	clearTestEnv()
	
	// Set both versions
	os.Setenv("GITHUB_TOKEN", "token1")
	os.Setenv("GOTCHA_GITHUB_TOKEN", "token2")
	os.Setenv("COMMENT_UUID", "uuid1")
	os.Setenv("GOTCHA_COMMENT_UUID", "uuid2")
	defer clearTestEnv()
	
	InitEnvironment()
	
	// GOTCHA_ should take precedence
	assert.Equal(t, "token2", GetGitHubToken())
	assert.Equal(t, "uuid2", GetCommentUUID())
}

func TestConfigFileBinding(t *testing.T) {
	// Test that configuration can be set via Viper's config file mechanism
	viper.Reset()
	clearTestEnv()
	
	// Simulate setting values as if they came from a config file
	viper.Set("ci", true)
	viper.Set("github.actions", true)
	viper.Set("use.mock", true)
	
	// These should be accessible via the getter functions
	assert.True(t, IsCIEnabled())
	assert.True(t, IsGitHubActionsEnabled())
	assert.True(t, UseMock())
}

func TestRuntimeVsConfiguration(t *testing.T) {
	// Test the distinction between runtime detection and configuration
	viper.Reset()
	clearTestEnv()
	
	// Set runtime environment (actual CI)
	os.Setenv("CI", "true")
	os.Setenv("GITHUB_ACTIONS", "true")
	defer clearTestEnv()
	
	InitEnvironment()
	
	// Runtime detection should be true
	assert.True(t, IsCI())
	assert.True(t, IsGitHubActions())
	
	// But configuration should still be false (defaults)
	assert.False(t, IsCIEnabled())
	assert.False(t, IsGitHubActionsEnabled())
	
	// Now enable via configuration
	os.Setenv("GOTCHA_CI", "true")
	os.Setenv("GOTCHA_GITHUB_ACTIONS", "true")
	
	// Re-initialize to pick up new env vars
	viper.Reset()
	InitEnvironment()
	
	// Both runtime and configuration should be true
	assert.True(t, IsCI())
	assert.True(t, IsGitHubActions())
	assert.True(t, IsCIEnabled())
	assert.True(t, IsGitHubActionsEnabled())
}

// Helper function to clear test environment variables
func clearTestEnv() {
	envVars := []string{
		"CI", "GITHUB_ACTIONS", "GITHUB_RUN_ID", "GOTCHA_CI",
		"GOTCHA_GITHUB_ACTIONS", "GOTCHA_GITHUB_RUN_ID", "GOTCHA_CI_PROVIDER",
		"GITHUB_REPOSITORY", "GOTCHA_GITHUB_REPOSITORY", 
		"GITHUB_EVENT_NAME", "GOTCHA_GITHUB_EVENT_NAME",
		"GITHUB_EVENT_PATH", "GOTCHA_GITHUB_EVENT_PATH",
		"GITHUB_STEP_SUMMARY", "GOTCHA_GITHUB_STEP_SUMMARY",
		"GITHUB_TOKEN", "GOTCHA_GITHUB_TOKEN",
		"COMMENT_UUID", "GOTCHA_COMMENT_UUID",
		"POST_COMMENT", "GOTCHA_POST_COMMENT",
		"GOTCHA_USE_MOCK",
		"FORCE_TTY", "GOTCHA_FORCE_TTY",
		"FORCE_NO_TTY", "GOTCHA_FORCE_NO_TTY",
		"NO_COLOR", "FORCE_COLOR", "TERM", "COLORTERM",
		"GOTCHA_OUTPUT", "GOTCHA_SHOW",
		"CONTINUOUS_INTEGRATION", "BUILD_NUMBER", "JENKINS_URL",
		"TRAVIS", "CIRCLECI",
		"GOTCHA_DEBUG_FILE", "GOTCHA_TEST_MODE", "GOTCHA_FORCE_TUI",
		"GOTCHA_SPLIT_STREAMS", "COLUMNS",
	}
	
	for _, env := range envVars {
		os.Unsetenv(env)
	}
}

func TestMain(m *testing.M) {
	// Ensure clean environment for all tests
	clearTestEnv()
	code := m.Run()
	clearTestEnv()
	os.Exit(code)
}