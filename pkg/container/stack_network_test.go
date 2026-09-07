package container

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasExplicitNetworkOverride(t *testing.T) {
	cases := []struct {
		name    string
		runArgs []string
		want    bool
	}{
		{name: "nil run_args", runArgs: nil, want: false},
		{name: "unrelated run_args", runArgs: []string{"--rm", "-v", "/a:/b"}, want: false},
		{name: "space form", runArgs: []string{"--network", "host"}, want: true},
		{name: "equals form", runArgs: []string{"--network=host"}, want: true},
		{name: "user's own network name", runArgs: []string{"--network=my-net"}, want: true},
		{name: "--net alias, space form", runArgs: []string{"--net", "host"}, want: true},
		{name: "--net alias, equals form", runArgs: []string{"--net=host"}, want: true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasExplicitNetworkOverride(tt.runArgs))
		})
	}
}

func TestStackNetworkName(t *testing.T) {
	// Names built from a stack containing no character that needs substitution keep
	// their exact, readable "atmos-<stack>" form.
	cases := map[string]string{
		"local": "atmos-local",
		"a-b":   "atmos-a-b",
		"":      "atmos-default",
	}
	for in, want := range cases {
		assert.Equal(t, want, StackNetworkName(in), "stack %q", in)
	}
}

func TestSanitizeNetworkToken(t *testing.T) {
	// Allowed characters pass through unchanged -- no hash suffix.
	assert.Equal(t, "ue2-prod_1.2", sanitizeNetworkToken("ue2-prod_1.2"))
	assert.Equal(t, "default", sanitizeNetworkToken(""))
	// Any substitution appends a stable hash suffix of the original input, and
	// sanitizing the same input twice is deterministic.
	got := sanitizeNetworkToken("x/?y")
	assert.True(t, strings.HasPrefix(got, "x--y-"), "got %q", got)
	assert.Equal(t, got, sanitizeNetworkToken("x/?y"))
}

func TestSanitizeNetworkToken_CollisionResistance(t *testing.T) {
	// Two different original values that would sanitize to the identical skeleton
	// ("deploy/prod" and "deploy-prod" both collapse '/' and literal '-' to the same
	// "deploy-prod") must no longer produce the same token, since only the value that
	// required substitution gains a hash suffix.
	assert.NotEqual(t, sanitizeNetworkToken("deploy/prod"), sanitizeNetworkToken("deploy-prod"))
	assert.Equal(t, "deploy-prod", sanitizeNetworkToken("deploy-prod"))
}

func TestStackNetworkName_CollisionResistance(t *testing.T) {
	assert.NotEqual(t, StackNetworkName("deploy/prod"), StackNetworkName("deploy-prod"))
}

func TestStackNetworkAliasScopesByStack(t *testing.T) {
	assert.Equal(t, "dev-aws", StackNetworkAlias("dev", "aws"))
	assert.NotEqual(t, StackNetworkAlias("dev", "aws"), StackNetworkAlias("dev/x", "aws"))
}

func TestStackNetworkAlias_CollisionResistance(t *testing.T) {
	// "deploy/prod"-"aws" and "deploy-prod"-"aws" both concatenate to the same
	// "deploy-prod-aws" skeleton; the substituted one must diverge via its hash suffix.
	assert.NotEqual(t, StackNetworkAlias("deploy/prod", "aws"), StackNetworkAlias("deploy-prod", "aws"))
	assert.Equal(t, "deploy-prod-aws", StackNetworkAlias("deploy-prod", "aws"))
}

// fakeNetworkRuntime is a Runtime that also implements NetworkEnsurer,
// recording EnsureNetwork calls. When inspectInfo is set, it also answers
// Inspect so tests can drive AttachSharedNetwork's "prefer the current
// container network" path.
type fakeNetworkRuntime struct {
	Runtime
	ensured     []string
	err         error
	inspectInfo *Info
}

func (f *fakeNetworkRuntime) EnsureNetwork(_ context.Context, name string) error {
	f.ensured = append(f.ensured, name)
	return f.err
}

func (f *fakeNetworkRuntime) Inspect(context.Context, string) (*Info, error) {
	return f.inspectInfo, nil
}

// plainRuntime is a Runtime without the NetworkEnsurer capability.
type plainRuntime struct {
	Runtime
}

func TestAttachSharedNetwork(t *testing.T) {
	// Force the shared-network path even when the test binary itself runs inside
	// a container (e.g. CI), where AttachSharedNetwork would otherwise join the
	// runner's own network instead of ensuring the per-stack one.
	t.Setenv(envUseCurrentContainerNetwork, "false")

	t.Run("attaches network with stack-scoped alias on success", func(t *testing.T) {
		rt := &fakeNetworkRuntime{}
		var networks []NetworkAttachment
		AttachSharedNetwork(context.Background(), rt, &networks, "dev", "gitserver")
		assert.Equal(t, []string{"atmos-dev"}, rt.ensured)
		assert.Equal(t, []NetworkAttachment{
			{Name: "atmos-dev", Aliases: []string{"dev-gitserver"}},
		}, networks)
	})

	t.Run("network creation failure leaves networks unchanged", func(t *testing.T) {
		rt := &fakeNetworkRuntime{err: errors.New("network create failed")}
		var networks []NetworkAttachment
		AttachSharedNetwork(context.Background(), rt, &networks, "dev", "gitserver")
		assert.Empty(t, networks)
	})

	t.Run("runtime without NetworkEnsurer is a no-op", func(t *testing.T) {
		var networks []NetworkAttachment
		AttachSharedNetwork(context.Background(), &plainRuntime{}, &networks, "dev", "gitserver")
		assert.Empty(t, networks)
	})
}

// TestAttachSharedNetwork_PrefersCurrentContainerNetwork proves that when Atmos's
// own process is itself detected as containerized (CurrentContainerNetwork
// resolves a usable network), AttachSharedNetwork joins that network directly
// instead of creating/ensuring a dedicated per-stack network -- so a job
// container that starts a sibling container can still reach it.
func TestAttachSharedNetwork_PrefersCurrentContainerNetwork(t *testing.T) {
	t.Setenv(envUseCurrentContainerNetwork, "true")

	rt := &fakeNetworkRuntime{inspectInfo: &Info{Networks: []string{"none", "ci-runner-net"}}}
	var networks []NetworkAttachment
	AttachSharedNetwork(context.Background(), rt, &networks, "dev", "gitserver")

	assert.Empty(t, rt.ensured, "EnsureNetwork must not be called when the current container network is usable")
	assert.Equal(t, []NetworkAttachment{
		{Name: "ci-runner-net", Aliases: []string{"dev-gitserver"}},
	}, networks)
}

// TestAttachSharedNetwork_SharesNetworkAcrossKinds proves the actual unification
// goal: a components.container-style call and an emulator-style call for the
// same stack land on the identical network, so both kinds resolve each other.
func TestAttachSharedNetwork_SharesNetworkAcrossKinds(t *testing.T) {
	t.Setenv(envUseCurrentContainerNetwork, "false")

	containerRuntime := &fakeNetworkRuntime{}
	var containerNetworks []NetworkAttachment
	AttachSharedNetwork(context.Background(), containerRuntime, &containerNetworks, "dev", "app")

	emulatorRuntime := &fakeNetworkRuntime{}
	var emulatorNetworks []NetworkAttachment
	AttachSharedNetwork(context.Background(), emulatorRuntime, &emulatorNetworks, "dev", "localstack")

	assert.Equal(t, "atmos-dev", containerNetworks[0].Name)
	assert.Equal(t, containerNetworks[0].Name, emulatorNetworks[0].Name)
	assert.NotEqual(t, containerNetworks[0].Aliases, emulatorNetworks[0].Aliases)
}

// connectCall records one ConnectNetwork invocation on fakeConnectRuntime.
type connectCall struct {
	network     string
	containerID string
	aliases     []string
}

// fakeConnectRuntime extends fakeNetworkRuntime with NetworkConnector, recording
// ConnectNetwork calls so tests can verify AttachSharedNetwork's
// join-current-container-when-reuse-fails behavior (see
// joinCurrentContainerToNetwork) without a real container runtime.
type fakeConnectRuntime struct {
	*fakeNetworkRuntime
	connected  []connectCall
	connectErr error
}

func (f *fakeConnectRuntime) ConnectNetwork(_ context.Context, network, containerID string, aliases []string) error {
	f.connected = append(f.connected, connectCall{network: network, containerID: containerID, aliases: aliases})
	return f.connectErr
}

// stubCurrentHostname overrides currentHostname for the duration of a test,
// restoring the original on cleanup.
func stubCurrentHostname(t *testing.T, hostname string, err error) {
	t.Helper()
	orig := currentHostname
	currentHostname = func() (string, error) { return hostname, err }
	t.Cleanup(func() { currentHostname = orig })
}

// TestAttachSharedNetwork_JoinsCurrentContainerWhenReuseFails proves the fix for
// the job-container-on-default-bridge case: when Atmos is containerized but its
// own current network can't be reused (bridge-only here), AttachSharedNetwork
// must both ensure the dedicated per-stack network for the new container *and*
// connect Atmos's own current container to that same network -- otherwise the
// new container is reachable by alias only from other peers on that network,
// never from Atmos's own (job) container.
func TestAttachSharedNetwork_JoinsCurrentContainerWhenReuseFails(t *testing.T) {
	t.Setenv(envUseCurrentContainerNetwork, "true")
	stubCurrentHostname(t, "job-container-abc", nil)

	base := &fakeNetworkRuntime{inspectInfo: &Info{Networks: []string{"bridge"}}}
	rt := &fakeConnectRuntime{fakeNetworkRuntime: base}

	var networks []NetworkAttachment
	AttachSharedNetwork(context.Background(), rt, &networks, "fixtures", "aws")

	assert.Equal(t, []string{"atmos-fixtures"}, rt.ensured)
	assert.Equal(t, []NetworkAttachment{
		{Name: "atmos-fixtures", Aliases: []string{"fixtures-aws"}},
	}, networks)
	if assert.Len(t, rt.connected, 1) {
		assert.Equal(t, "atmos-fixtures", rt.connected[0].network)
		assert.Equal(t, "job-container-abc", rt.connected[0].containerID)
	}
}

// TestAttachSharedNetwork_NotContainerized_DoesNotJoinCurrentContainer proves
// normal host-based (non-containerized) behavior is unchanged: even with a
// runtime that supports NetworkConnector, the current-container join must not
// be attempted when Atmos isn't (and hasn't opted into acting as if it were)
// containerized.
func TestAttachSharedNetwork_NotContainerized_DoesNotJoinCurrentContainer(t *testing.T) {
	t.Setenv(envUseCurrentContainerNetwork, "")
	restore := stubCurrentNetworkDetection(t, false)
	defer restore()

	rt := &fakeConnectRuntime{fakeNetworkRuntime: &fakeNetworkRuntime{}}
	var networks []NetworkAttachment
	AttachSharedNetwork(context.Background(), rt, &networks, "dev", "gitserver")

	assert.Equal(t, []string{"atmos-dev"}, rt.ensured)
	assert.Empty(t, rt.connected, "current container must not be joined when Atmos isn't containerized")
}

// TestAttachSharedNetwork_JoinIsNoOpWithoutNetworkConnector proves a runtime
// that only implements NetworkEnsurer (not NetworkConnector) behaves exactly
// as before this change: the dedicated network is still created, and no panic
// or error results from the missing capability.
func TestAttachSharedNetwork_JoinIsNoOpWithoutNetworkConnector(t *testing.T) {
	t.Setenv(envUseCurrentContainerNetwork, "true")
	stubCurrentHostname(t, "job-container-abc", nil)

	rt := &fakeNetworkRuntime{inspectInfo: &Info{Networks: []string{"bridge"}}}
	var networks []NetworkAttachment
	assert.NotPanics(t, func() {
		AttachSharedNetwork(context.Background(), rt, &networks, "dev", "gitserver")
	})
	assert.Equal(t, []string{"atmos-dev"}, rt.ensured)
	assert.Equal(t, []NetworkAttachment{
		{Name: "atmos-dev", Aliases: []string{"dev-gitserver"}},
	}, networks)
}

// TestAttachSharedNetwork_JoinFailureIsBestEffort proves a failed
// ConnectNetwork call never propagates as an error or blocks the new
// container's own network attachment -- it only means the current container
// stays unreachable-by-alias, same as if NetworkConnector weren't implemented.
func TestAttachSharedNetwork_JoinFailureIsBestEffort(t *testing.T) {
	t.Setenv(envUseCurrentContainerNetwork, "true")
	stubCurrentHostname(t, "job-container-abc", nil)

	base := &fakeNetworkRuntime{inspectInfo: &Info{Networks: []string{"bridge"}}}
	rt := &fakeConnectRuntime{fakeNetworkRuntime: base, connectErr: errors.New("connect failed")}

	var networks []NetworkAttachment
	assert.NotPanics(t, func() {
		AttachSharedNetwork(context.Background(), rt, &networks, "dev", "gitserver")
	})
	assert.Equal(t, []NetworkAttachment{
		{Name: "atmos-dev", Aliases: []string{"dev-gitserver"}},
	}, networks)
}

// TestAttachSharedNetwork_JoinSkippedWhenHostnameUnknown proves the join is
// skipped (not attempted with an empty container ID) when the current
// container's own hostname can't be determined, mirroring
// CurrentContainerNetwork's own hostname short-circuit.
func TestAttachSharedNetwork_JoinSkippedWhenHostnameUnknown(t *testing.T) {
	t.Setenv(envUseCurrentContainerNetwork, "true")
	stubCurrentHostname(t, "", assert.AnError)

	base := &fakeNetworkRuntime{inspectInfo: &Info{Networks: []string{"bridge"}}}
	rt := &fakeConnectRuntime{fakeNetworkRuntime: base}

	var networks []NetworkAttachment
	AttachSharedNetwork(context.Background(), rt, &networks, "dev", "gitserver")

	assert.Empty(t, rt.connected)
}
