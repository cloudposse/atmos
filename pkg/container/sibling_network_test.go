package container

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSiblingContainerNetworking_Real is the regression test for the
// job-container-on-default-bridge bug (an emulator container started through
// a mounted Docker socket reported an endpoint -- a guessed bridge-gateway IP
// -- that the job container that started it couldn't actually reach).
//
// Unlike every other test in this package touching AttachSharedNetwork /
// CurrentContainerNetwork, this one does NOT stub ProcessRunsInContainer,
// currentHostname, or Inspect: it exercises the real self-detection and
// network-join mechanism against whatever container this test process is
// actually running in, talking to the daemon however it actually reaches it
// (typically a mounted socket). On a normal, non-containerized dev/CI
// machine, ProcessRunsInContainer() is false and this is a no-op skip -- it
// only exercises the mechanism it's meant to test when actually run nested
// inside a container. See sibling_network_docker_test.go, which drives
// exactly that by running `go test -run TestSiblingContainerNetworking_Real`
// inside a container started with no `--network` (Docker's default bridge)
// and the host socket mounted in -- the precise shape of the field
// reproduction.
func TestSiblingContainerNetworking_Real(t *testing.T) {
	if !ProcessRunsInContainer() {
		t.Skip("only meaningful when this test process is itself running inside a container -- see sibling_network_docker_test.go, which drives that")
	}

	ctx := context.Background()
	runtime, err := DetectRuntimeWithPreference(ctx, "docker")
	if err != nil {
		t.Skipf("no Docker CLI reachable through a mounted socket, skipping: %v", err)
		return
	}
	if err := runtime.Pull(ctx, "alpine:latest"); err != nil {
		t.Skipf("Docker not available, skipping: %v", err)
		return
	}

	stack, name := "siblingtest", "probe"
	var networks []NetworkAttachment
	AttachSharedNetwork(ctx, runtime, &networks, stack, name)
	require.NotEmpty(t, networks, "AttachSharedNetwork must produce at least one network attachment for the sibling container")

	containerID, err := runtime.Create(ctx, &CreateConfig{
		Name:     "atmos-sibling-probe",
		Image:    "alpine:latest",
		Command:  []string{"nc", "-l", "-p", "8080"},
		Networks: networks,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = runtime.Remove(ctx, containerID, true) })
	require.NoError(t, runtime.Start(ctx, containerID))

	alias := StackNetworkAlias(stack, name)

	// This is the exact operation the AWS provider performs against the
	// emulator's reported endpoint -- a plain TCP dial by hostname, from the
	// current process, no docker exec involved. Retried briefly: the
	// sibling's nc listener may not be bound the instant its container
	// reports "started".
	var dialErr error
	for range 20 {
		var conn net.Conn
		conn, dialErr = net.DialTimeout("tcp", net.JoinHostPort(alias, "8080"), 2*time.Second)
		if dialErr == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.NoError(t, dialErr, "current (self) container must reach the sibling container by its network alias -- "+
		"this is exactly what fails when Atmos's own container can't be joined to the sibling's network")
}
