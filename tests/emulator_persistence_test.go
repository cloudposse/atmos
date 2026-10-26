package tests

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
)

// TestEmulatorPersistence proves emulator state persists across `down`/`up` by
// default and that `reset` wipes it. It uses the registry emulator as the subject
// (its data dir, /var/lib/registry, is bind-mounted from the XDG cache): a blob is
// pushed, the container is stopped and removed, brought back up, and the blob is
// still served — then `reset` clears it and the blob is gone.
//
// Opt-in and runtime-gated like the other emulator E2Es: it runs only when
// ATMOS_TEST_FLOCI=true and a container runtime (Docker or Podman) is available.
func TestEmulatorPersistence(t *testing.T) {
	if os.Getenv("ATMOS_TEST_FLOCI") != "true" {
		t.Skip("set ATMOS_TEST_FLOCI=true to run the emulator E2E")
	}

	ctx := context.Background()
	if _, err := container.DetectRuntime(ctx); err != nil {
		t.Skipf("no container runtime available for the emulator E2E: %v", err)
	}

	ensureAtmosRunner(t)

	workdir := copyFlociScenarioFixture(t, filepath.Join("fixtures", "scenarios", "emulator-registry"))

	// Point the XDG cache at a temp dir so persistence is hermetic (and so `reset`
	// targets this test's state, not a developer's real cache). The fixture pins the
	// registry to a fixed host port; this suite runs tests sequentially, so reusing
	// it is safe.
	env := map[string]string{
		"ATMOS_CLI_CONFIG_PATH": workdir,
		"TF_IN_AUTOMATION":      "1",
		"ATMOS_XDG_CACHE_HOME":  t.TempDir(),
	}
	const stack = "local"

	run := func(timeout time.Duration, args ...string) (string, string, error) {
		return runFlociAtmosInDir(t, workdir, env, timeout, args...)
	}
	baseURL := "http://localhost:" + registryHostPort

	up := func() {
		_, stderr, err := run(3*time.Minute, "emulator", "up", "registry", "-s", stack)
		require.NoErrorf(t, err, "emulator up failed: %s", stderr)
		waitRegistryReady(t, baseURL)
	}

	// Remove any container left by a prior crashed run so the first `up` starts from
	// a clean, temp-backed state rather than reusing stale storage.
	_, _, _ = run(2*time.Minute, "emulator", "down", "registry", "-s", stack)
	// Always tear the container down at the end (state lives in the temp XDG dir,
	// which t.TempDir cleans up).
	t.Cleanup(func() {
		_, _, _ = run(2*time.Minute, "emulator", "down", "registry", "-s", stack)
	})

	const repo = "atmos-persist-test"
	blob := []byte("atmos-persistence-proof")
	digest := "sha256:" + hex.EncodeToString(sha256Sum(blob))

	// 1) Bring it up and push a blob.
	up()
	pushRegistryBlob(t, baseURL, repo, digest, blob)
	require.True(t, registryBlobExists(t, baseURL, repo, digest), "blob must exist right after push")

	// 2) down + up: with persistence (the default), the blob must survive.
	_, stderr, err := run(2*time.Minute, "emulator", "down", "registry", "-s", stack)
	require.NoErrorf(t, err, "emulator down failed: %s", stderr)
	up()
	require.True(t, registryBlobExists(t, baseURL, repo, digest),
		"blob must survive down/up because persistence is enabled by default")

	// 3) reset + up: the blob must be gone (the negative/recovery counterpart).
	_, stderr, err = run(2*time.Minute, "emulator", "reset", "registry", "-s", stack, "--force")
	require.NoErrorf(t, err, "emulator reset failed: %s", stderr)
	up()
	require.False(t, registryBlobExists(t, baseURL, repo, digest),
		"blob must be gone after reset wipes the persisted state")
}

func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

// waitRegistryReady blocks until the OCI distribution API answers 200 at /v2/.
func waitRegistryReady(t *testing.T, baseURL string) {
	t.Helper()
	require.Eventually(t, func() bool {
		probeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		req, reqErr := http.NewRequestWithContext(probeCtx, http.MethodGet, baseURL+"/v2/", nil)
		if reqErr != nil {
			return false
		}
		resp, doErr := http.DefaultClient.Do(req)
		if doErr != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 90*time.Second, 2*time.Second, "registry OCI API did not become ready at /v2/")
}

// pushRegistryBlob uploads a blob via the registry's monolithic upload flow
// (POST to start an upload, then PUT the content with its digest).
func pushRegistryBlob(t *testing.T, baseURL, repo, digest string, content []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start an upload session.
	startURL := fmt.Sprintf("%s/v2/%s/blobs/uploads/", baseURL, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, startURL, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, drainClose(resp))
	require.Equalf(t, http.StatusAccepted, resp.StatusCode, "unexpected status starting blob upload")

	// Resolve the (possibly relative) upload location against the base URL and
	// append the digest query parameter for the monolithic PUT.
	loc := resp.Header.Get("Location")
	require.NotEmpty(t, loc, "registry must return an upload Location")
	putURL := resolveUploadURL(t, baseURL, loc, digest)

	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, strings.NewReader(string(content)))
	require.NoError(t, err)
	putReq.Header.Set("Content-Type", "application/octet-stream")
	putResp, err := http.DefaultClient.Do(putReq)
	require.NoError(t, err)
	require.NoError(t, drainClose(putResp))
	require.Equalf(t, http.StatusCreated, putResp.StatusCode, "unexpected status completing blob upload")
}

// resolveUploadURL turns the upload Location into an absolute URL with the digest
// query parameter appended (preserving any state query the registry set).
func resolveUploadURL(t *testing.T, baseURL, location, digest string) string {
	t.Helper()
	base, err := url.Parse(baseURL)
	require.NoError(t, err)
	rel, err := url.Parse(location)
	require.NoError(t, err)
	abs := base.ResolveReference(rel)
	q := abs.Query()
	q.Set("digest", digest)
	abs.RawQuery = q.Encode()
	return abs.String()
}

// registryBlobExists reports whether the registry currently serves the blob
// (HEAD /v2/<repo>/blobs/<digest> -> 200).
func registryBlobExists(t *testing.T, baseURL, repo, digest string) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	headURL := fmt.Sprintf("%s/v2/%s/blobs/%s", baseURL, repo, digest)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, headURL, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, drainClose(resp))
	return resp.StatusCode == http.StatusOK
}

func drainClose(resp *http.Response) error {
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.Body.Close()
}
