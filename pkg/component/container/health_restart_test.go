package container

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	ctr "github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestContainerSpec_RestartPolicy(t *testing.T) {
	tests := []struct {
		name string
		run  *schema.ContainerRunStep
		want *ctr.RestartPolicy
	}{
		{name: "nil run", run: nil, want: nil},
		{name: "no restart", run: &schema.ContainerRunStep{}, want: nil},
		{name: "empty policy ignored", run: &schema.ContainerRunStep{Restart: &schema.ContainerRestart{}}, want: nil},
		{
			name: "policy with retries",
			run:  &schema.ContainerRunStep{Restart: &schema.ContainerRestart{Policy: "on-failure", MaxRetries: 3}},
			want: &ctr.RestartPolicy{Policy: "on-failure", MaxRetries: 3},
		},
		{
			name: "policy without retries",
			run:  &schema.ContainerRunStep{Restart: &schema.ContainerRestart{Policy: "unless-stopped"}},
			want: &ctr.RestartPolicy{Policy: "unless-stopped"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := ContainerSpec{Run: tt.run}
			assert.Equal(t, tt.want, spec.RestartPolicy())
		})
	}
}

func TestContainerSpec_HealthCheck(t *testing.T) {
	tests := []struct {
		name string
		hc   *schema.ContainerHealthCheck
		want *ctr.HealthCheck
	}{
		{name: "nil", hc: nil, want: nil},
		{
			name: "CMD-SHELL strips prefix",
			hc:   &schema.ContainerHealthCheck{Test: []string{"CMD-SHELL", "curl -f http://localhost || exit 1"}, Interval: "30s", Retries: 3},
			want: &ctr.HealthCheck{Cmd: "curl -f http://localhost || exit 1", Interval: "30s", Retries: 3},
		},
		{
			name: "CMD joins exec args into shell string",
			hc:   &schema.ContainerHealthCheck{Test: []string{"CMD", "curl", "-f", "http://localhost"}},
			want: &ctr.HealthCheck{Cmd: "curl -f http://localhost"},
		},
		{
			name: "bare string list treated as CMD-SHELL",
			hc:   &schema.ContainerHealthCheck{Test: []string{"curl -f http://localhost"}},
			want: &ctr.HealthCheck{Cmd: "curl -f http://localhost"},
		},
		{
			name: "NONE disables",
			hc:   &schema.ContainerHealthCheck{Test: []string{"NONE"}},
			want: &ctr.HealthCheck{Disable: true},
		},
		{
			name: "disable flag wins over a test",
			hc:   &schema.ContainerHealthCheck{Test: []string{"CMD-SHELL", "true"}, Disable: true},
			want: &ctr.HealthCheck{Disable: true},
		},
		{
			name: "all duration fields pass through",
			hc: &schema.ContainerHealthCheck{
				Test:          []string{"CMD-SHELL", "true"},
				Interval:      "30s",
				Timeout:       "5s",
				Retries:       2,
				StartPeriod:   "10s",
				StartInterval: "2s",
			},
			want: &ctr.HealthCheck{Cmd: "true", Interval: "30s", Timeout: "5s", Retries: 2, StartPeriod: "10s", StartInterval: "2s"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := ContainerSpec{Run: &schema.ContainerRunStep{HealthCheck: tt.hc}}
			if tt.hc == nil {
				spec.Run.HealthCheck = nil
			}
			assert.Equal(t, tt.want, spec.HealthCheck())
		})
	}
}

func TestContainerSpec_ValidateRun(t *testing.T) {
	tests := []struct {
		name    string
		run     *schema.ContainerRunStep
		wantErr error
	}{
		{name: "nil run ok", run: nil},
		{name: "valid restart", run: &schema.ContainerRunStep{Restart: &schema.ContainerRestart{Policy: "on-failure", MaxRetries: 3}}},
		{
			name:    "unknown policy",
			run:     &schema.ContainerRunStep{Restart: &schema.ContainerRestart{Policy: "sometimes"}},
			wantErr: errUtils.ErrInvalidContainerRestartPolicy,
		},
		{
			name:    "negative max_retries",
			run:     &schema.ContainerRunStep{Restart: &schema.ContainerRestart{Policy: "on-failure", MaxRetries: -1}},
			wantErr: errUtils.ErrInvalidContainerRestartPolicy,
		},
		{
			name: "valid healthcheck durations",
			run:  &schema.ContainerRunStep{HealthCheck: &schema.ContainerHealthCheck{Interval: "30s", Timeout: "5s", StartPeriod: "1m30s", StartInterval: "2s"}},
		},
		{
			name:    "bad duration",
			run:     &schema.ContainerRunStep{HealthCheck: &schema.ContainerHealthCheck{Interval: "30sec"}},
			wantErr: errUtils.ErrInvalidContainerHealthCheck,
		},
		{
			name:    "negative retries",
			run:     &schema.ContainerRunStep{HealthCheck: &schema.ContainerHealthCheck{Retries: -1}},
			wantErr: errUtils.ErrInvalidContainerHealthCheck,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := ContainerSpec{Run: tt.run}
			err := spec.ValidateRun()
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestFromComponentSection_RestartAndHealthCheck(t *testing.T) {
	section := map[string]any{
		"image": "nginx:alpine",
		"run": map[string]any{
			"restart": map[string]any{
				"policy":      "unless-stopped",
				"max_retries": 5,
			},
			"healthcheck": map[string]any{
				"test":         []any{"CMD-SHELL", "wget -q -O /dev/null http://localhost/ || exit 1"},
				"interval":     "30s",
				"retries":      3,
				"start_period": "10s",
			},
		},
	}
	spec, err := FromComponentSection(section)
	require.NoError(t, err)
	require.NotNil(t, spec.Run.Restart)
	assert.Equal(t, "unless-stopped", spec.Run.Restart.Policy)
	assert.Equal(t, 5, spec.Run.Restart.MaxRetries)
	require.NotNil(t, spec.Run.HealthCheck)
	assert.Equal(t, []string{"CMD-SHELL", "wget -q -O /dev/null http://localhost/ || exit 1"}, spec.Run.HealthCheck.Test)
	assert.Equal(t, "30s", spec.Run.HealthCheck.Interval)
	assert.Equal(t, 3, spec.Run.HealthCheck.Retries)

	// Mappers resolve the decoded section end to end.
	assert.Equal(t, &ctr.RestartPolicy{Policy: "unless-stopped", MaxRetries: 5}, spec.RestartPolicy())
	assert.Equal(t, &ctr.HealthCheck{Cmd: "wget -q -O /dev/null http://localhost/ || exit 1", Interval: "30s", Retries: 3, StartPeriod: "10s"}, spec.HealthCheck())
	require.NoError(t, spec.ValidateRun())
}

func TestHealthCell(t *testing.T) {
	assert.Equal(t, "-", healthCell(""))
	assert.Equal(t, "healthy", healthCell("healthy"))
	assert.Equal(t, "unhealthy", healthCell("unhealthy"))
	assert.Equal(t, "starting", healthCell("starting"))
}

func TestFromComponentSection_HealthCheckScalarTest(t *testing.T) {
	// A bare string `test` decodes into a one-element slice (WeaklyTypedInput) and
	// is treated as CMD-SHELL.
	section := map[string]any{
		"image": "nginx:alpine",
		"run": map[string]any{
			"healthcheck": map[string]any{
				"test": "curl -f http://localhost || exit 1",
			},
		},
	}
	spec, err := FromComponentSection(section)
	require.NoError(t, err)
	require.NotNil(t, spec.HealthCheck())
	assert.Equal(t, "curl -f http://localhost || exit 1", spec.HealthCheck().Cmd)
}
