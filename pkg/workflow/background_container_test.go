package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFirstNonEmpty(t *testing.T) {
	assert.Equal(t, "x", firstNonEmpty("", "  ", "x", "y"))
	assert.Equal(t, "podman", firstNonEmpty("podman", "docker"))
	assert.Empty(t, firstNonEmpty("", "   ", ""))
	assert.Empty(t, firstNonEmpty())
}

func TestToPortBindings(t *testing.T) {
	assert.Nil(t, toPortBindings(nil))

	out := toPortBindings([]schema.ContainerPort{
		{Host: 8080, Container: 80}, // default protocol -> tcp.
		{Host: 53, Container: 53, Protocol: "udp"},
	})
	require.Len(t, out, 2)
	assert.Equal(t, container.PortBinding{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"}, out[0])
	assert.Equal(t, container.PortBinding{HostPort: 53, ContainerPort: 53, Protocol: "udp"}, out[1])
}

func TestToMounts(t *testing.T) {
	assert.Nil(t, toMounts(nil))

	out := toMounts([]schema.ContainerMount{
		{Source: "/h", Target: "/c"},
		{Type: "bind", Source: "/a", Target: "/b", ReadOnly: true},
	})
	require.Len(t, out, 2)
	assert.Equal(t, container.Mount{Source: "/h", Target: "/c"}, out[0])
	assert.Equal(t, container.Mount{Type: "bind", Source: "/a", Target: "/b", ReadOnly: true}, out[1])
}

func TestToRestartPolicy(t *testing.T) {
	assert.Nil(t, toRestartPolicy(nil))
	assert.Nil(t, toRestartPolicy(&schema.ContainerRestart{}), "empty policy yields nil (runtime default)")

	got := toRestartPolicy(&schema.ContainerRestart{Policy: "on-failure", MaxRetries: 3})
	require.NotNil(t, got)
	assert.Equal(t, container.RestartPolicy{Policy: "on-failure", MaxRetries: 3}, *got)
}

func TestResolveHealthTest(t *testing.T) {
	cases := []struct {
		name        string
		test        []string
		wantCmd     string
		wantDisable bool
	}{
		{"empty", nil, "", false},
		{"none disables", []string{"NONE"}, "", true},
		{"cmd prefix stripped", []string{"CMD", "redis-cli", "ping"}, "redis-cli ping", false},
		{"cmd-shell prefix stripped", []string{"CMD-SHELL", "wget", "-q", "url"}, "wget -q url", false},
		{"unprefixed joined", []string{"curl", "-f", "http://x"}, "curl -f http://x", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, disable := resolveHealthTest(tc.test)
			assert.Equal(t, tc.wantCmd, cmd)
			assert.Equal(t, tc.wantDisable, disable)
		})
	}
}

func TestToHealthCheck(t *testing.T) {
	assert.Nil(t, toHealthCheck(nil))

	// Explicit disable wins.
	assert.Equal(t, &container.HealthCheck{Disable: true}, toHealthCheck(&schema.ContainerHealthCheck{Disable: true}))

	// A NONE test also disables.
	assert.Equal(t, &container.HealthCheck{Disable: true}, toHealthCheck(&schema.ContainerHealthCheck{Test: []string{"NONE"}}))

	// A normal healthcheck carries the resolved command plus timing fields.
	got := toHealthCheck(&schema.ContainerHealthCheck{
		Test:        []string{"CMD", "true"},
		Interval:    "2s",
		Timeout:     "1s",
		Retries:     5,
		StartPeriod: "3s",
	})
	assert.Equal(t, &container.HealthCheck{Cmd: "true", Interval: "2s", Timeout: "1s", Retries: 5, StartPeriod: "3s"}, got)
}

// TestContainerRunner_Start_DryRun exercises the detached-start path without a
// container runtime: DryRun short-circuits before the runtime is touched, so the
// config build (which calls every to* mapping helper) and handle creation run.
func TestContainerRunner_Start_DryRun(t *testing.T) {
	cr := &ContainerRunner{Stack: "test-stack", DryRun: true}
	step := &schema.WorkflowStep{
		Name: "svc",
		Run: &schema.ContainerRunStep{
			Image:       "nginx:alpine",
			Command:     "nginx -g 'daemon off;'",
			Ports:       []schema.ContainerPort{{Host: 8080, Container: 80}},
			Mounts:      []schema.ContainerMount{{Source: "/h", Target: "/c"}},
			Restart:     &schema.ContainerRestart{Policy: "always"},
			HealthCheck: &schema.ContainerHealthCheck{Test: []string{"CMD", "true"}, Interval: "2s"},
			Provider:    "docker",
		},
	}

	h, err := cr.Start(context.Background(), step, []string{"K=V"})
	require.NoError(t, err)
	require.NotNil(t, h)
	assert.Equal(t, "svc", h.Name())

	// Dry-run handle methods are no-ops (no runtime attached).
	require.NoError(t, h.WaitReady(context.Background()))
	require.NoError(t, h.Stop(context.Background()))
}

func TestContainerRunner_Start_Errors(t *testing.T) {
	cr := &ContainerRunner{Stack: "test-stack", DryRun: true}

	t.Run("missing run block", func(t *testing.T) {
		_, err := cr.Start(context.Background(), &schema.WorkflowStep{Name: "svc"}, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, schema.ErrWorkflowControlStepInvalid)
		assert.Contains(t, err.Error(), "with.image")
	})

	t.Run("empty image", func(t *testing.T) {
		_, err := cr.Start(context.Background(), &schema.WorkflowStep{Name: "svc", Run: &schema.ContainerRunStep{Image: "  "}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "with.image")
	})

	t.Run("unparseable command", func(t *testing.T) {
		_, err := cr.Start(context.Background(), &schema.WorkflowStep{
			Name: "svc",
			Run:  &schema.ContainerRunStep{Image: "alpine", Command: `echo "unterminated`},
		}, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, schema.ErrWorkflowControlStepInvalid)
		assert.Contains(t, err.Error(), "command")
	})
}
