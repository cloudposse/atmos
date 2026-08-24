package container

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddHealthAndRestart(t *testing.T) {
	tests := []struct {
		name     string
		config   *CreateConfig
		expected []string
	}{
		{
			name:     "neither set appends nothing",
			config:   &CreateConfig{},
			expected: nil,
		},
		{
			name:     "restart without max_retries",
			config:   &CreateConfig{Restart: &RestartPolicy{Policy: "unless-stopped"}},
			expected: []string{"--restart", "unless-stopped"},
		},
		{
			name:     "restart on-failure with max_retries",
			config:   &CreateConfig{Restart: &RestartPolicy{Policy: "on-failure", MaxRetries: 5}},
			expected: []string{"--restart", "on-failure:5"},
		},
		{
			name:     "max_retries ignored without on-failure",
			config:   &CreateConfig{Restart: &RestartPolicy{Policy: "always", MaxRetries: 5}},
			expected: []string{"--restart", "always"},
		},
		{
			name: "full health check",
			config: &CreateConfig{HealthCheck: &HealthCheck{
				Cmd:           "curl -f http://localhost || exit 1",
				Interval:      "30s",
				Timeout:       "5s",
				Retries:       3,
				StartPeriod:   "10s",
				StartInterval: "2s",
			}},
			expected: []string{
				"--health-cmd", "curl -f http://localhost || exit 1",
				"--health-interval", "30s",
				"--health-timeout", "5s",
				"--health-start-period", "10s",
				"--health-start-interval", "2s",
				"--health-retries", "3",
			},
		},
		{
			name:     "disabled health check",
			config:   &CreateConfig{HealthCheck: &HealthCheck{Disable: true, Cmd: "ignored"}},
			expected: []string{"--no-healthcheck"},
		},
		{
			name: "restart and health together",
			config: &CreateConfig{
				Restart:     &RestartPolicy{Policy: "on-failure", MaxRetries: 2},
				HealthCheck: &HealthCheck{Cmd: "true", Retries: 1},
			},
			expected: []string{"--restart", "on-failure:2", "--health-cmd", "true", "--health-retries", "1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addHealthAndRestart(nil, tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildCreateArgs_HealthAndRestart(t *testing.T) {
	// The flags are emitted between the runtime flags and the image, and an empty
	// config leaves the previous layout untouched.
	config := &CreateConfig{
		Name:        "svc",
		Image:       "nginx:alpine",
		Restart:     &RestartPolicy{Policy: "unless-stopped"},
		HealthCheck: &HealthCheck{Cmd: "wget -q -O - http://localhost", Interval: "30s"},
	}
	result := buildCreateArgs(config)
	assert.Equal(t, []string{
		"create", "--name", "svc", "-it",
		"--restart", "unless-stopped",
		"--health-cmd", "wget -q -O - http://localhost",
		"--health-interval", "30s",
		"nginx:alpine",
	}, result)
}

func TestParseHealth(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"Up 2 hours (healthy)", "healthy"},
		{"Up 5 minutes (unhealthy)", "unhealthy"},
		{"Up 3 seconds (health: starting)", "starting"},
		{"Up Less than a second (starting)", "starting"},
		{"Up 2 hours", ""},
		{"Exited (0) 3 minutes ago", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.want, parseHealth(tt.status))
		})
	}
}

func TestNormalizeHealth(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"healthy", "healthy"},
		{"Healthy", "healthy"},
		{"unhealthy", "unhealthy"},
		{"starting", "starting"},
		{"none", ""},
		{"", ""},
		{"bogus", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeHealth(tt.in))
		})
	}
}
