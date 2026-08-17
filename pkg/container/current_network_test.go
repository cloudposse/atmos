package container

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirstReachableNetwork(t *testing.T) {
	assert.Equal(t, "github_network_123", firstReachableNetwork([]string{"", "none", "host", "github_network_123"}))
	assert.Empty(t, firstReachableNetwork([]string{"", "none", "host"}))
	// Docker's default "bridge" network doesn't support embedded DNS/aliases,
	// so it must be skipped in favor of a user-defined network even when it
	// sorts first in the inspection results.
	assert.Equal(t, "my-user-defined-net", firstReachableNetwork([]string{"bridge", "my-user-defined-net"}))
	assert.Empty(t, firstReachableNetwork([]string{"bridge"}))
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
			name:            "explicit override true wins even when containerized detection disagrees",
			runsInContainer: false,
			override:        "true",
			want:            true,
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

// TestCurrentContainerNetwork_OverrideTrueBypassesDetectionButStillFailsSoft
// covers the "explicit override true" branch of PreferCurrentContainerNetwork
// directly through CurrentContainerNetwork: even though ProcessRunsInContainer
// would say false, the override forces the runtime.Inspect path -- and, on a
// non-containerized host where Inspect naturally fails or reports no usable
// network, CurrentContainerNetwork still returns "" rather than panicking or
// erroring, so callers fall back to a dedicated network exactly as before.
func TestCurrentContainerNetwork_OverrideTrueBypassesDetectionButStillFailsSoft(t *testing.T) {
	t.Setenv(envUseCurrentContainerNetwork, "true")
	restore := stubCurrentNetworkDetection(t, false)
	defer restore()

	got := CurrentContainerNetwork(context.Background(), staticInspectRuntime{
		info: &Info{Networks: []string{"host", "none"}},
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

// erroringInspectRuntime is a Runtime whose Inspect always fails, used to cover
// CurrentContainerNetwork's "inspect failure" fallback.
type erroringInspectRuntime struct {
	Runtime
}

func (erroringInspectRuntime) Inspect(context.Context, string) (*Info, error) {
	return nil, errors.New("inspect failed")
}

// TestCurrentContainerNetwork_InspectFailureOrNilInfoFallsBackToEmpty covers the
// two ways runtime.Inspect can fail to yield usable data -- an error, or a nil
// Info with no error (e.g. an unknown container ID) -- confirming both make
// CurrentContainerNetwork fail soft to "" instead of panicking on a nil
// dereference or propagating the error.
func TestCurrentContainerNetwork_InspectFailureOrNilInfoFallsBackToEmpty(t *testing.T) {
	t.Setenv(envUseCurrentContainerNetwork, "true")
	restore := stubCurrentNetworkDetection(t, true)
	defer restore()

	t.Run("inspect returns an error", func(t *testing.T) {
		got := CurrentContainerNetwork(context.Background(), erroringInspectRuntime{})
		assert.Empty(t, got)
	})

	t.Run("inspect returns nil info without an error", func(t *testing.T) {
		got := CurrentContainerNetwork(context.Background(), staticInspectRuntime{info: nil})
		assert.Empty(t, got)
	})
}

// TestCurrentContainerNetwork_UndeterminableHostnameFallsBackToEmpty covers the
// "hostname can't be determined" short-circuit: even when the process is
// (or claims to be) containerized and the runtime would otherwise resolve a
// network, an unresolvable hostname must still make CurrentContainerNetwork
// fail soft to "" rather than propagate the error or call Inspect at all.
func TestCurrentContainerNetwork_UndeterminableHostnameFallsBackToEmpty(t *testing.T) {
	t.Setenv(envUseCurrentContainerNetwork, "true")
	restore := stubCurrentNetworkDetection(t, true)
	defer restore()

	origHostname := currentHostname
	currentHostname = func() (string, error) { return "", assert.AnError }
	defer func() { currentHostname = origHostname }()

	inspected := false
	got := CurrentContainerNetwork(context.Background(), inspectSpyRuntime{
		staticInspectRuntime: staticInspectRuntime{info: &Info{Networks: []string{"ci-runner-net"}}},
		onInspect:            func() { inspected = true },
	})

	assert.Empty(t, got)
	assert.False(t, inspected, "Inspect must not be called when the hostname can't be determined")
}

type inspectSpyRuntime struct {
	staticInspectRuntime
	onInspect func()
}

func (r inspectSpyRuntime) Inspect(ctx context.Context, containerID string) (*Info, error) {
	r.onInspect()
	return r.staticInspectRuntime.Inspect(ctx, containerID)
}

// TestDefaultProcessRunsInContainer exercises the real (non-stubbed) detection
// heuristic directly. The dockerenv/containerenv marker-file checks go through
// the markerFileExists seam, so this suite runs deterministically regardless of
// whether the test host itself is containerized -- it stubs the marker files as
// absent to assert the heuristic falls through to the /proc/1/cgroup marker
// scan, which readProcFile makes fully controllable.
func TestDefaultProcessRunsInContainer(t *testing.T) {
	origMarkerFileExists := markerFileExists
	origReadProcFile := readProcFile
	defer func() {
		markerFileExists = origMarkerFileExists
		readProcFile = origReadProcFile
	}()

	markerFileExists = func(string) bool { return false }

	tests := []struct {
		name    string
		content []byte
		err     error
		want    bool
	}{
		{name: "cgroup file unreadable", err: os.ErrNotExist, want: false},
		{name: "cgroup with no container markers", content: []byte("0::/\n"), want: false},
		{name: "cgroup with docker marker", content: []byte("0::/docker/abc123\n"), want: true},
		{name: "cgroup with kubepods marker", content: []byte("0::/kubepods/besteffort/pod1\n"), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readProcFile = func(string) ([]byte, error) { return tt.content, tt.err }
			assert.Equal(t, tt.want, defaultProcessRunsInContainer())
		})
	}
}

// TestDefaultProcessRunsInContainer_MarkerFilePresentShortCircuits covers the
// marker-file-present path directly: when either /.dockerenv or
// /run/.containerenv is reported present, defaultProcessRunsInContainer must
// return true without ever consulting /proc/1/cgroup.
func TestDefaultProcessRunsInContainer_MarkerFilePresentShortCircuits(t *testing.T) {
	origMarkerFileExists := markerFileExists
	origReadProcFile := readProcFile
	defer func() {
		markerFileExists = origMarkerFileExists
		readProcFile = origReadProcFile
	}()

	tests := []struct {
		name       string
		presentFor string
	}{
		{name: "dockerenv present", presentFor: "/.dockerenv"},
		{name: "containerenv present", presentFor: "/run/.containerenv"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			markerFileExists = func(path string) bool { return path == tt.presentFor }
			readProcFile = func(string) ([]byte, error) {
				t.Fatal("readProcFile must not be called when a marker file short-circuits detection")
				return nil, nil
			}

			assert.True(t, defaultProcessRunsInContainer())
		})
	}
}

// TestDefaultMarkerFileExists exercises the real os.Stat-backed implementation
// behind the markerFileExists seam directly (every other test in this file
// stubs the seam instead) -- confirming it reports true for a path that
// exists on disk and false for one that doesn't, the two states
// defaultProcessRunsInContainer's marker-file short-circuit depends on.
func TestDefaultMarkerFileExists(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "marker")
	require.NoError(t, os.WriteFile(existing, nil, 0o600))

	assert.True(t, defaultMarkerFileExists(existing))
	assert.False(t, defaultMarkerFileExists(filepath.Join(dir, "does-not-exist")))
}
