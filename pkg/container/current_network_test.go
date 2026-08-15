package container

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFirstReachableNetwork(t *testing.T) {
	assert.Equal(t, "github_network_123", firstReachableNetwork([]string{"", "none", "host", "github_network_123"}))
	assert.Empty(t, firstReachableNetwork([]string{"", "none", "host"}))
}

func TestPreferCurrentContainerNetwork(t *testing.T) {
	tests := []struct {
		name            string
		runsInContainer bool
		githubActions   string
		override        string
		want            bool
	}{
		{
			name:            "containerized with no opt-in prefers the network alias",
			runsInContainer: true,
			want:            true,
		},
		{
			name:            "non-containerized stays false",
			runsInContainer: false,
			want:            false,
		},
		{
			name:            "GITHUB_ACTIONS unrelated -- containerized already wins on its own",
			runsInContainer: true,
			githubActions:   "true",
			want:            true,
		},
		{
			name:            "explicit override false wins over containerized detection",
			runsInContainer: true,
			override:        "false",
			want:            false,
		},
		{
			name:            "explicit override true is still gated by containerized detection",
			runsInContainer: false,
			override:        "true",
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := stubCurrentNetworkDetection(t, tt.runsInContainer)
			defer restore()

			t.Setenv("GITHUB_ACTIONS", tt.githubActions)
			t.Setenv(envUseCurrentContainerNetwork, tt.override)

			assert.Equal(t, tt.want, PreferCurrentContainerNetwork())
		})
	}
}

type staticInspectRuntime struct {
	Runtime
	info *Info
}

func (r staticInspectRuntime) Inspect(context.Context, string) (*Info, error) {
	return r.info, nil
}

func TestCurrentContainerNetwork(t *testing.T) {
	t.Setenv(envUseCurrentContainerNetwork, "true")
	restore := stubCurrentNetworkDetection(t, true)
	defer restore()

	got := CurrentContainerNetwork(context.Background(), staticInspectRuntime{
		info: &Info{Networks: []string{"none", "github_network_123"}},
	})

	assert.Equal(t, "github_network_123", got)
}

// TestCurrentContainerNetwork_UndeterminableFallsBackToEmpty covers a
// containerized run where the network still can't be determined -- e.g.
// --network host, or a runtime whose Inspect reports no usable network --
// confirming callers still get "" and therefore fall back to a dedicated
// per-stack network instead of erroring.
func TestCurrentContainerNetwork_UndeterminableFallsBackToEmpty(t *testing.T) {
	t.Setenv(envUseCurrentContainerNetwork, "")
	restore := stubCurrentNetworkDetection(t, true)
	defer restore()

	got := CurrentContainerNetwork(context.Background(), staticInspectRuntime{
		info: &Info{Networks: []string{"host"}},
	})

	assert.Empty(t, got)
}

func stubCurrentNetworkDetection(t *testing.T, inContainer bool) func() {
	t.Helper()

	origProcessRunsInContainer := ProcessRunsInContainer
	origReadProcFile := readProcFile

	ProcessRunsInContainer = func() bool { return inContainer }
	readProcFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }

	return func() {
		ProcessRunsInContainer = origProcessRunsInContainer
		readProcFile = origReadProcFile
	}
}
