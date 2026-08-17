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
// recording EnsureNetwork calls.
type fakeNetworkRuntime struct {
	Runtime
	ensured []string
	err     error
}

func (f *fakeNetworkRuntime) EnsureNetwork(_ context.Context, name string) error {
	f.ensured = append(f.ensured, name)
	return f.err
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
