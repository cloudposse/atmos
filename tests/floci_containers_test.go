package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/cloudposse/atmos/pkg/container"
)

// Floci emulator images, pinned by immutable digest. Keep these in sync with the
// service containers in .github/workflows/test.yml (job "[floci] go e2e"); the
// trailing comment tracks the human-readable tag.
const (
	flociAWSImage   = "floci/floci@sha256:c88ec20bf221630dd195d38a14eeb0ac52ddfa72c37ebb3c8aa17f63ae27c5f2"     // 1.5.23.
	flociGCPImage   = "floci/floci-gcp@sha256:a6420f308ad721fa4a203b70658563eab9c8fbc8d091feca2d95016239f5854a" // latest.
	flociAzureImage = "floci/floci-az@sha256:1e514c57db14dc41938f7925bbc1aca0293aa4da272c7014d98f1fba378cedb2"  // latest.

	// Startup timeout bounding how long we wait for an emulator to answer HTTP.
	flociStartupTimeout = 90 * time.Second
)

// flociAutostartSkip, when non-empty, explains why Floci auto-start could not run
// (for example, no container runtime is available). The harness consults it so the
// opt-in Floci tests skip with an actionable message instead of failing on a
// connection error.
var flociAutostartSkip string

// flociServiceSpec describes one Floci emulator container that auto-start can bring up.
type flociServiceSpec struct {
	name     string // Human-readable label used in log/error messages.
	image    string // Pinned image reference.
	port     string // Internal container port (for example, "4566").
	endpoint string // Env var the harness reads (for example, "FLOCI_ENDPOINT_URL").
}

// flociServiceSpecs returns the emulator containers backing the AWS, GCP, and
// Azure Floci test suites, mapping each to the endpoint env var its harness reads.
func flociServiceSpecs() []flociServiceSpec {
	return []flociServiceSpec{
		{name: "aws", image: flociAWSImage, port: "4566", endpoint: "FLOCI_ENDPOINT_URL"},
		{name: "gcp", image: flociGCPImage, port: "4588", endpoint: "FLOCI_GCP_ENDPOINT"},
		{name: "azure", image: flociAzureImage, port: "4577", endpoint: "FLOCI_AZURE_ENDPOINT"},
	}
}

// maybeStartFloci auto-starts the Floci emulator containers needed by the opt-in
// Floci E2E tests, but only when ATMOS_TEST_FLOCI=true and the relevant endpoint
// env vars are not already set. CI pre-sets the endpoints (pointing at GitHub
// Actions service containers), so auto-start no-ops there. Locally, where the
// endpoints are unset, it brings each emulator up on a dynamic host port and
// populates the matching env var so the harness can discover it.
//
// It works with either Docker or Podman: the container runtime is auto-detected
// via pkg/container, and for Podman the API socket is wired into testcontainers.
//
// It returns a cleanup function (safe to call even when nil) that terminates any
// containers it started. When no container runtime is available it records the
// reason in flociAutostartSkip and returns (nil, nil) so the suite continues and
// the Floci tests skip cleanly.
func maybeStartFloci(ctx context.Context) (func(), error) {
	if os.Getenv("ATMOS_TEST_FLOCI") != "true" {
		return nil, nil
	}

	pending := pendingFlociServices()
	if len(pending) == 0 {
		// Every endpoint is already provided (for example, CI service containers).
		return nil, nil
	}

	if reason := configureFlociContainerRuntime(ctx); reason != "" {
		flociAutostartSkip = reason
		return nil, nil
	}

	var started []testcontainers.Container
	cleanup := func() {
		// Terminate in reverse order; the reaper is the backstop for anything missed.
		for i := len(started) - 1; i >= 0; i-- {
			_ = started[i].Terminate(context.Background())
		}
	}

	for _, spec := range pending {
		ctr, endpoint, err := startFlociContainer(ctx, spec)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("auto-starting Floci %s emulator: %w", spec.name, err)
		}
		started = append(started, ctr)
		flociSetenv(spec.endpoint, endpoint)
	}

	return cleanup, nil
}

// pendingFlociServices returns the emulator specs whose endpoint env var is unset,
// i.e. the ones auto-start must launch.
func pendingFlociServices() []flociServiceSpec {
	var pending []flociServiceSpec
	for _, spec := range flociServiceSpecs() {
		if os.Getenv(spec.endpoint) == "" {
			pending = append(pending, spec)
		}
	}
	return pending
}

// configureFlociContainerRuntime auto-detects an available container runtime and,
// for Podman, points testcontainers at its API socket. It returns "" on success or
// a human-readable reason when no runtime is available.
func configureFlociContainerRuntime(ctx context.Context) string {
	runtime, err := container.DetectRuntime(ctx)
	if err != nil {
		return fmt.Sprintf("no container runtime (Docker or Podman) is available to auto-start Floci: %v", err)
	}

	// Docker exposes the conventional socket that testcontainers finds on its own.
	if container.GetRuntimeType(runtime) != container.TypePodman {
		return ""
	}

	// Podman: testcontainers looks for a Docker socket by default, so point it at the
	// Podman API socket unless the caller already configured DOCKER_HOST.
	if os.Getenv("DOCKER_HOST") == "" {
		socket, sockErr := podmanSocketPath(ctx)
		if sockErr != nil {
			return fmt.Sprintf("Podman is running but its API socket could not be determined for Floci auto-start: %v", sockErr)
		}
		flociSetenv("DOCKER_HOST", "unix://"+socket)
	}

	// Rootless Podman cannot run the privileged Ryuk reaper; cleanup() terminates the
	// containers we start explicitly, so disable Ryuk to avoid a startup failure.
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		flociSetenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
	return ""
}

// podmanSocketPath returns the host path of the Podman API socket. On macOS and
// Windows this comes from the active Podman machine; on native Linux it is the
// socket Podman itself reports.
func podmanSocketPath(ctx context.Context) (string, error) {
	// macOS/Windows: the host-side socket is owned by the running Podman machine.
	if out, err := exec.CommandContext(ctx, "podman", "machine", "inspect",
		"--format", "{{.ConnectionInfo.PodmanSocket.Path}}").Output(); err == nil {
		if path := firstNonEmptyLine(string(out)); path != "" {
			return path, nil
		}
	}

	// Native Linux: ask Podman for its (rootless or system) API socket directly.
	out, err := exec.CommandContext(ctx, "podman", "info",
		"--format", "{{.Host.RemoteSocket.Path}}").Output()
	if err != nil {
		return "", err
	}
	if path := firstNonEmptyLine(string(out)); path != "" {
		return path, nil
	}
	return "", fmt.Errorf("podman did not report an API socket path")
}

// firstNonEmptyLine returns the first non-blank, trimmed line of s, or "".
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// startFlociContainer launches a single Floci emulator container and returns it
// along with the http://host:port endpoint mapped to its exposed port.
func startFlociContainer(ctx context.Context, spec flociServiceSpec) (testcontainers.Container, string, error) {
	portProto := spec.port + "/tcp"
	req := testcontainers.ContainerRequest{
		Image:        spec.image,
		ExposedPorts: []string{portProto},
		WaitingFor: wait.ForHTTP("/").
			WithPort(portProto).
			WithStartupTimeout(flociStartupTimeout).
			// Floci answers on "/" with a non-2xx status; any HTTP response means ready.
			WithStatusCodeMatcher(func(int) bool { return true }),
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", err
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		_ = ctr.Terminate(context.Background())
		return nil, "", err
	}

	mapped, err := ctr.MappedPort(ctx, portProto)
	if err != nil {
		_ = ctr.Terminate(context.Background())
		return nil, "", err
	}

	return ctr, fmt.Sprintf("http://%s:%s", host, mapped.Port()), nil
}

// flociSetenv sets a process-level env var from TestMain (which has no *testing.T,
// and where the value must persist for every test in the package).
func flociSetenv(key, value string) {
	os.Setenv(key, value) //nolint:lintroller // Set before m.Run(); no *testing.T available in TestMain.
}
