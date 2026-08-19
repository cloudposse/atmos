package container

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestSiblingContainerNetworking_Docker drives
// TestSiblingContainerNetworking_Real (sibling_network_test.go) from outside:
// it runs the entire `go test` invocation itself inside a container started
// with no --network (Docker's default bridge) and the host Docker socket
// mounted in -- the precise shape of the field reproduction this feature
// fixes (a CI job container that talks to Docker only through a mounted
// socket, sitting on the default bridge because nothing told it to join a
// custom network). Because the nested test process is genuinely
// containerized, its ProcessRunsInContainer()/currentHostname()/Inspect calls
// are all real, not stubbed -- this is what actually proves the fix works,
// rather than a mocked runtime asserting the code path taken.
//
// Deliberately opt-in via ATMOS_TEST_SIBLING_CONTAINER=1 (mirroring the
// existing ATMOS_TEST_REGISTRY_CACHE gate in docker_test.go): it downloads
// and builds inside a nested container and is comparatively slow. Run it at
// minimum before merging any change to pkg/container's networking machinery,
// and ideally in a dedicated CI job.
func TestSiblingContainerNetworking_Docker(t *testing.T) {
	if os.Getenv("ATMOS_TEST_SIBLING_CONTAINER") != "1" {
		t.Skip("set ATMOS_TEST_SIBLING_CONTAINER=1 to run the nested job-container networking regression test")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker CLI not available: %v", err)
	}

	repoRoot, err := testhelpers.FindRepoRoot()
	require.NoError(t, err)

	goModCache, err := goEnv(t, "GOMODCACHE")
	require.NoError(t, err)
	goBuildCache, err := goEnv(t, "GOCACHE")
	require.NoError(t, err)

	// No --network flag: this must land on Docker's default bridge, exactly
	// like a plain `docker run` job container with no custom network -- the
	// scenario AttachSharedNetwork's current-container join exists to fix.
	args := []string{
		"run", "--rm",
		"-v", repoRoot + ":/workspace",
		"-v", goModCache + ":/go/pkg/mod",
		"-v", goBuildCache + ":/root/.cache/go-build",
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"-w", "/workspace",
		"-e", "GOMODCACHE=/go/pkg/mod",
		"-e", "GOCACHE=/root/.cache/go-build",
		"golang:1.26-alpine",
		"sh", "-c",
		"apk add --no-cache docker-cli >/dev/null && " +
			"go test ./pkg/container/... -run TestSiblingContainerNetworking_Real -v -timeout 180s",
	}

	cmd := exec.Command("docker", args...)
	output, runErr := cmd.CombinedOutput()
	t.Logf("nested job-container test output:\n%s", output)
	require.NoError(t, runErr, "nested job-container run of TestSiblingContainerNetworking_Real must succeed")
	require.Contains(t, string(output), "PASS", "nested test run must report PASS, not just exit 0")
}

// goEnv returns the trimmed output of `go env <key>` on the host, used to
// mount the host's module/build caches into the nested container so it
// doesn't re-download the entire module graph on every run.
func goEnv(t *testing.T, key string) (string, error) {
	t.Helper()

	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
