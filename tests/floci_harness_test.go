//nolint:depguard // The Floci harness creates AWS clients for opt-in AWS-compatible integration tests.
package tests

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/stretchr/testify/require"
)

const (
	flociDefaultEndpoint = "http://localhost:4566"
	flociAWSRegion       = "us-east-1"
)

type flociHarnessOptions struct {
	FixtureDir      string
	EndpointEnvVar  string
	DefaultEndpoint string
	ClearAWSEnv     bool
	ExtraEnv        map[string]string
}

type flociHarness struct {
	Endpoint string
	TestID   string
	Workdir  string
	Env      map[string]string
}

func newFlociHarness(t *testing.T, opts flociHarnessOptions) *flociHarness {
	t.Helper()

	endpoint := requireFlociEndpoint(t, opts.EndpointEnvVar, opts.DefaultEndpoint)
	ensureFlociAtmosRunner(t)

	testID := uniqueFlociTestID(t)
	workdir := copyFlociScenarioFixture(t, opts.FixtureDir)
	env := flociHarnessCommandEnv(t, endpoint, workdir, testID, opts.ClearAWSEnv)
	if opts.EndpointEnvVar != "" {
		env[opts.EndpointEnvVar] = endpoint
	}
	for key, value := range opts.ExtraEnv {
		env[key] = strings.ReplaceAll(value, "{test_id}", testID)
	}

	return &flociHarness{
		Endpoint: endpoint,
		TestID:   testID,
		Workdir:  workdir,
		Env:      env,
	}
}

func ensureFlociAtmosRunner(t *testing.T) {
	t.Helper()

	ensureAtmosRunner(t)
}

func (h *flociHarness) Run(t *testing.T, timeout time.Duration, args ...string) (string, string, error) {
	t.Helper()
	return runFlociAtmosInDir(t, h.Workdir, h.Env, timeout, args...)
}

func (h *flociHarness) SSMClient(t *testing.T) *ssm.Client {
	t.Helper()
	return newFlociSSMClient(t, h.Endpoint)
}

func (h *flociHarness) SecretsManagerClient(t *testing.T) *secretsmanager.Client {
	t.Helper()
	return newFlociSecretsManagerClient(t, h.Endpoint)
}

func copyFlociScenarioFixture(t *testing.T, sourceRelPath string) string {
	t.Helper()

	sourceAbs, err := filepath.Abs(sourceRelPath)
	require.NoError(t, err)

	workdir := filepath.Join(t.TempDir(), filepath.Base(sourceRelPath))
	require.NoError(t, copyFlociFixture(sourceAbs, workdir))
	return workdir
}

func flociHarnessCommandEnv(t *testing.T, endpoint, absWorkdir, testID string, clearAWSEnv bool) map[string]string {
	t.Helper()

	env := map[string]string{
		"ATMOS_CLI_CONFIG_PATH":      absWorkdir,
		"ATMOS_FLOCI_TEST_ID":        testID,
		"FLOCI_ENDPOINT_URL":         endpoint,
		"ATMOS_FLOCI_ALIAS_LOCK_DIR": t.TempDir(),
		"TF_IN_AUTOMATION":           "1",
	}
	if clearAWSEnv {
		env["AWS_ACCESS_KEY_ID"] = ""
		env["AWS_SECRET_ACCESS_KEY"] = ""
		env["AWS_SESSION_TOKEN"] = ""
		env["AWS_PROFILE"] = ""
		env["AWS_DEFAULT_REGION"] = ""
		env["AWS_REGION"] = ""
		env["AWS_ENDPOINT_URL"] = ""
		env["AWS_EC2_METADATA_DISABLED"] = "true"
		return env
	}

	env["AWS_ACCESS_KEY_ID"] = "test"
	env["AWS_SECRET_ACCESS_KEY"] = "test"
	env["AWS_DEFAULT_REGION"] = flociAWSRegion
	env["AWS_REGION"] = flociAWSRegion
	env["AWS_ENDPOINT_URL"] = endpoint
	return env
}

func copyFlociFixture(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return os.MkdirAll(dstDir, 0o755)
		}
		if entry.Name() == ".git" && entry.IsDir() {
			return filepath.SkipDir
		}

		targetPath := filepath.Join(dstDir, relPath)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(targetPath, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, info.Mode().Perm())
	})
}

func requireFlociEndpoint(t *testing.T, endpointEnvVar, defaultEndpoint string) string {
	t.Helper()

	if os.Getenv("ATMOS_TEST_FLOCI") != "true" {
		t.Skip("set ATMOS_TEST_FLOCI=true and start Floci before running this integration test")
	}

	endpoint, fromEnv := resolveFlociEndpoint(endpointEnvVar, defaultEndpoint)

	// When this test's endpoint was not explicitly provided and auto-start (TestMain)
	// could not bring Floci up (e.g. no container runtime is available), skip with
	// that reason rather than failing the health check. An explicitly provided
	// endpoint is always health-checked so a misconfiguration fails loudly.
	if !fromEnv && flociAutostartSkip != "" {
		t.Skip(flociAutostartSkip)
	}

	endpoint = normalizeFlociEndpoint(endpoint)

	parsed, err := url.Parse(endpoint)
	require.NoError(t, err)
	require.NotEmpty(t, parsed.Host)

	address := parsed.Host
	if _, _, splitErr := net.SplitHostPort(address); splitErr != nil {
		port := parsed.Port()
		if port == "" {
			port = "4566"
		}
		address = net.JoinHostPort(parsed.Hostname(), port)
	}

	// Poll for up to flociStartupTimeout instead of checking once: CI pre-supplies
	// these endpoints via GitHub Actions service containers, which are only
	// guaranteed to have started the container process, not to have the app inside
	// actually serving HTTP yet -- the same cold-start window the local
	// testcontainers auto-start path already tolerates via
	// wait.ForHTTP(...).WithStartupTimeout(flociStartupTimeout) in
	// startFlociContainer. A single 2s check here previously failed
	// TestAzureSecretsFlociE2E with the TCP port already accepting connections but
	// the HTTP handler not yet ready to respond.
	err = pollUntil(flociStartupTimeout, func() error {
		conn, dialErr := net.DialTimeout("tcp", address, 2*time.Second)
		if dialErr != nil {
			return dialErr
		}
		defer func() { _ = conn.Close() }()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if reqErr != nil {
			return reqErr
		}
		// #nosec G107 -- endpoint is an opt-in local Floci test target.
		resp, respErr := http.DefaultClient.Do(req)
		if respErr != nil {
			return respErr
		}
		return resp.Body.Close()
	})
	require.NoErrorf(t, err, "Floci HTTP endpoint is not reachable at %s within %s", endpoint, flociStartupTimeout)

	return endpoint
}

// pollUntil retries fn every 500ms until it succeeds or timeout elapses, returning
// fn's last error on timeout. The deadline is checked before each call to fn so a
// slow fn can't push the total run time past timeout by an extra call, and the
// retry sleep is capped to the time remaining so it never oversleeps the deadline.
func pollUntil(timeout time.Duration, fn func() error) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		if time.Now().After(deadline) {
			return lastErr
		}
		if lastErr = fn(); lastErr == nil {
			return nil
		}
		if remaining := time.Until(deadline); remaining > 0 {
			sleep := 500 * time.Millisecond
			if remaining < sleep {
				sleep = remaining
			}
			time.Sleep(sleep)
		}
	}
}

// resolveFlociEndpoint determines the Floci endpoint to use and reports whether it
// came from an explicitly set environment variable (as opposed to a built-in
// default). The resolution order mirrors the long-standing harness behavior:
// the named endpoint var, then AWS_ENDPOINT_URL (only when no named var is given),
// then FLOCI_ENDPOINT_URL, then the provided default, then the package default.
func resolveFlociEndpoint(endpointEnvVar, defaultEndpoint string) (endpoint string, fromEnv bool) {
	if endpointEnvVar != "" {
		if v := os.Getenv(endpointEnvVar); v != "" {
			return v, true
		}
	} else if v := os.Getenv("AWS_ENDPOINT_URL"); v != "" {
		return v, true
	}
	if v := os.Getenv("FLOCI_ENDPOINT_URL"); v != "" {
		return v, true
	}
	if defaultEndpoint != "" {
		return defaultEndpoint, false
	}
	return flociDefaultEndpoint, false
}

func normalizeFlociEndpoint(endpoint string) string {
	if strings.Contains(endpoint, "://") {
		return endpoint
	}
	return "http://" + endpoint
}

func runFlociAtmos(t *testing.T, env map[string]string, timeout time.Duration, args ...string) (string, string, error) {
	t.Helper()

	return runFlociAtmosInDir(t, "", env, timeout, args...)
}

func runFlociAtmosInDir(t *testing.T, dir string, env map[string]string, timeout time.Duration, args ...string) (string, string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := atmosRunner.CommandContext(ctx, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = mergeCommandEnv(cmd.Env, env)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return stdout.String(), stderr.String(), ctx.Err()
	}
	return stdout.String(), stderr.String(), err
}

func mergeCommandEnv(base []string, overrides map[string]string) []string {
	if len(base) == 0 {
		base = os.Environ()
	}
	env := append([]string{}, base...)
	for key, value := range overrides {
		prefix := key + "="
		for i := 0; i < len(env); i++ {
			if strings.HasPrefix(env[i], prefix) {
				env = append(env[:i], env[i+1:]...)
				i--
			}
		}
		env = append(env, prefix+value)
	}
	return env
}

func newFlociSSMClient(t *testing.T, endpoint string) *ssm.Client {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(flociAWSRegion),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	require.NoError(t, err)

	return ssm.NewFromConfig(cfg, func(options *ssm.Options) {
		options.BaseEndpoint = aws.String(endpoint)
	})
}

func newFlociSecretsManagerClient(t *testing.T, endpoint string) *secretsmanager.Client {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(flociAWSRegion),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	require.NoError(t, err)

	return secretsmanager.NewFromConfig(cfg, func(options *secretsmanager.Options) {
		options.BaseEndpoint = aws.String(endpoint)
	})
}

func uniqueFlociTestID(t *testing.T) string {
	t.Helper()

	name := strings.ToLower(t.Name())
	name = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), name)
}

func TestPollUntilSucceedsImmediately(t *testing.T) {
	t.Parallel()

	calls := 0
	err := pollUntil(time.Second, func() error {
		calls++
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls, "fn should not be retried once it succeeds")
}

func TestPollUntilRetriesUntilSuccessWithinBudget(t *testing.T) {
	t.Parallel()

	calls := 0
	err := pollUntil(5*time.Second, func() error {
		calls++
		if calls < 3 {
			return errors.New("not ready yet")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, calls, "should stop retrying as soon as fn succeeds")
}

func TestPollUntilReturnsLastErrorOnTimeout(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("still not ready")
	calls := 0
	err := pollUntil(600*time.Millisecond, func() error {
		calls++
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)
	require.Greater(t, calls, 1, "should have retried at least once before the budget elapsed")
}
