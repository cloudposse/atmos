//nolint:forbidigo // This opt-in integration test is configured through environment variables.
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	flociGCPDefaultEndpoint = "localhost:4588"
	flociGCPProjectID       = "test-project"
)

func TestSecretFlociGCPSecretManager(t *testing.T) {
	endpoint := requireFlociGCPEndpoint(t)

	ensureAtmosRunner(t)

	workdir := filepath.Join("fixtures", "scenarios", "floci-gcp-secrets")
	absWorkdir, err := filepath.Abs(workdir)
	require.NoError(t, err)

	testID := uniqueFlociTestID(t)
	value := "floci-gcp-secret-value-" + testID
	env := flociGCPSecretCommandEnv(endpoint, absWorkdir)
	client := newFlociGCPSecretManagerClient(t, endpoint)
	secretName := flociGCPSecretResourceName("local", "API_TOKEN")
	defer bestEffortFlociGCPSecretDelete(t, client, secretName)

	stdout, stderr, err := runFlociAtmosInDir(
		t, absWorkdir, env, 2*time.Minute,
		"secret", "set",
		"-s", "local",
		"-c", "app",
		"--type", "terraform",
		"API_TOKEN="+value,
	)
	require.NoError(t, err, "secret set failed:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	requireFlociGCPSecretExists(t, client, secretName)

	stdout, stderr, err = runFlociAtmosInDir(
		t, absWorkdir, env, 2*time.Minute,
		"secret", "get",
		"-s", "local",
		"-c", "app",
		"--type", "terraform",
		"--raw",
		"API_TOKEN",
	)
	require.NoError(t, err, "secret get failed:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	require.Equal(t, value, stdout)

	stdout, stderr, err = runFlociAtmosInDir(
		t, absWorkdir, env, 2*time.Minute,
		"secret", "list",
		"-s", "local",
		"-c", "app",
		"--type", "terraform",
		"--format", "json",
	)
	require.NoError(t, err, "secret list failed:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	requireFlociGCPSecretListStatus(t, stdout, "API_TOKEN", "initialized")

	stdout, stderr, err = runFlociAtmosInDir(
		t, absWorkdir, env, 2*time.Minute,
		"describe", "component", "app",
		"-s", "local",
		"--format", "json",
	)
	require.NoError(t, err, "describe component failed:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	require.Contains(t, stdout, value, "describe component should resolve !secret when masking is disabled")

	stdout, stderr, err = runFlociAtmosInDir(
		t, absWorkdir, env, 2*time.Minute,
		"secret", "delete",
		"-s", "local",
		"-c", "app",
		"--type", "terraform",
		"--force",
		"API_TOKEN",
	)
	require.NoError(t, err, "secret delete failed:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	requireFlociGCPSecretGone(t, client, secretName)

	stdout, stderr, err = runFlociAtmosInDir(
		t, absWorkdir, env, 2*time.Minute,
		"secret", "list",
		"-s", "local",
		"-c", "app",
		"--type", "terraform",
		"--format", "json",
	)
	require.NoError(t, err, "secret list after delete failed:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	requireFlociGCPSecretListStatus(t, stdout, "API_TOKEN", "missing")
}

func requireFlociGCPSecretListStatus(t *testing.T, output, secret, status string) {
	t.Helper()

	var entries []struct {
		Secret string
		Status string
	}
	require.NoError(t, json.Unmarshal([]byte(output), &entries), "secret list output should be JSON")

	for _, entry := range entries {
		if entry.Secret == secret {
			require.Equal(t, status, entry.Status)
			return
		}
	}
	require.Failf(t, "secret not listed", "expected secret %q in output:\n%s", secret, output)
}

func requireFlociGCPEndpoint(t *testing.T) string {
	t.Helper()

	if os.Getenv("ATMOS_TEST_FLOCI_GCP") != "true" {
		t.Skip("set ATMOS_TEST_FLOCI_GCP=true and start floci-gcp before running this integration test")
	}

	endpoint := os.Getenv("SECRET_MANAGER_EMULATOR_HOST")
	if endpoint == "" {
		endpoint = os.Getenv("FLOCI_GCP_ENDPOINT")
	}
	if endpoint == "" {
		endpoint = flociGCPDefaultEndpoint
	}
	endpoint = normalizeFlociGCPGRPCEndpoint(endpoint)

	conn, err := net.DialTimeout("tcp", endpoint, 2*time.Second)
	if err != nil {
		t.Skipf("floci-gcp is not reachable at %s: %v", endpoint, err)
	}
	require.NoError(t, conn.Close())

	return endpoint
}

func flociGCPSecretCommandEnv(endpoint, absWorkdir string) map[string]string {
	return map[string]string{
		"ATMOS_CLI_CONFIG_PATH":        absWorkdir,
		"ATMOS_MASK":                   "false",
		"FLOCI_GCP_ENDPOINT":           "http://" + endpoint,
		"FLOCI_GCP_PROJECT":            flociGCPProjectID,
		"GOOGLE_CLOUD_PROJECT":         flociGCPProjectID,
		"SECRET_MANAGER_EMULATOR_HOST": endpoint,
	}
}

func normalizeFlociGCPGRPCEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if strings.Contains(endpoint, "://") {
		if parsed, err := url.Parse(endpoint); err == nil && parsed.Host != "" {
			return parsed.Host
		}
	}
	return endpoint
}

func newFlociGCPSecretManagerClient(t *testing.T, endpoint string) *secretmanager.Client {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	client, err := secretmanager.NewClient(ctx, option.WithGRPCConn(conn))
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Logf("closing floci-gcp secret manager client: %v", err)
		}
		if err := conn.Close(); err != nil && status.Code(err) != codes.Canceled {
			t.Logf("closing floci-gcp grpc connection: %v", err)
		}
	})

	return client
}

func flociGCPSecretResourceName(testID, key string) string {
	return fmt.Sprintf("projects/%s/secrets/%s", flociGCPProjectID, flociGCPSecretID(testID, key))
}

func flociGCPSecretID(testID, key string) string {
	parts := append(strings.Split(testID, "-"), "app", key)
	secretID := strings.Join(parts, "_")
	secretID = strings.ReplaceAll(secretID, "__", "_")
	return strings.Trim(secretID, "_")
}

func requireFlociGCPSecretExists(t *testing.T, client *secretmanager.Client, name string) {
	t.Helper()

	require.Eventually(t, func() bool {
		exists, err := flociGCPSecretExists(client, name)
		if err != nil {
			t.Logf("checking floci-gcp secret %s: %v", name, err)
			return false
		}
		return exists
	}, 10*time.Second, 200*time.Millisecond, "expected floci-gcp secret %s to exist", name)
}

func requireFlociGCPSecretGone(t *testing.T, client *secretmanager.Client, name string) {
	t.Helper()

	require.Eventually(t, func() bool {
		exists, err := flociGCPSecretExists(client, name)
		if err != nil {
			t.Logf("checking floci-gcp secret %s: %v", name, err)
			return false
		}
		return !exists
	}, 10*time.Second, 200*time.Millisecond, "expected floci-gcp secret %s to be deleted", name)
}

func flociGCPSecretExists(client *secretmanager.Client, name string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.GetSecret(ctx, &secretmanagerpb.GetSecretRequest{Name: name})
	if err == nil {
		return true, nil
	}
	if status.Code(err) == codes.NotFound {
		return false, nil
	}
	return false, err
}

func bestEffortFlociGCPSecretDelete(t *testing.T, client *secretmanager.Client, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.DeleteSecret(ctx, &secretmanagerpb.DeleteSecretRequest{Name: name})
	if err == nil || status.Code(err) == codes.NotFound {
		return
	}
	t.Logf("best-effort floci-gcp secret cleanup failed for %s: %v", name, err)
}
