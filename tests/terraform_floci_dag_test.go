//nolint:forbidigo // This opt-in integration test is configured through environment variables.
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

const (
	flociDefaultEndpoint = "http://localhost:4566"
	flociAWSRegion       = "us-east-1"
)

var flociMarkerKeys = []string{
	"seed",
	"bucket-marker",
	"queue-marker",
	"topic-marker",
	"final-marker",
}

func TestTerraformFlociApplyDestroyDAG(t *testing.T) {
	endpoint := requireFlociEndpoint(t)
	RequireTerraform(t)

	ensureFlociAtmosRunner(t)

	workdir := filepath.Join("fixtures", "scenarios", "terraform-floci-dag")
	absWorkdir, err := filepath.Abs(workdir)
	require.NoError(t, err)

	t.Run("rejects concurrent apply without auto approve", func(t *testing.T) {
		sandbox := setupFlociSandbox(t, absWorkdir)
		env := flociCommandEnv(t, endpoint, absWorkdir, sandbox, uniqueFlociTestID(t))

		stdout, stderr, err := runFlociAtmos(
			t, env, 2*time.Minute,
			"terraform", "apply", "--all", "-s", "local", "--max-concurrency", "2", "-i", "false",
		)

		require.Error(t, err)
		require.Contains(t, stdout+stderr, "requires -auto-approve")
	})

	t.Run("sequential apply destroy lifecycle", func(t *testing.T) {
		runFlociLifecycle(t, endpoint, absWorkdir, 1)
	})

	t.Run("parallel apply destroy lifecycle", func(t *testing.T) {
		runFlociLifecycle(t, endpoint, absWorkdir, 4)
	})

	t.Run("aliases sharing source path are sequential without workdir", func(t *testing.T) {
		runFlociAliasSequentialProbe(t, endpoint, absWorkdir)
	})
}

func TestTerraformFlociAffectedApplyDestroyDAG(t *testing.T) {
	endpoint := requireFlociEndpoint(t)
	RequireTerraform(t)
	RequireExecutable(t, "git", "terraform affected Floci tests")

	ensureFlociAtmosRunner(t)

	workdir := filepath.Join("fixtures", "scenarios", "terraform-floci-dag")
	absWorkdir, err := filepath.Abs(workdir)
	require.NoError(t, err)

	t.Run("direct affected apply destroy", func(t *testing.T) {
		repos := setupFlociAffectedRepos(t, absWorkdir)
		testID := uniqueFlociTestID(t)
		env := flociCommandEnv(t, endpoint, repos.HeadDir, nil, testID)
		client := newFlociSSMClient(t, endpoint)
		summaryFile := filepath.Join(t.TempDir(), "affected-direct-summary.json")

		defer bestEffortFlociDestroyInDir(t, repos.HeadDir, env)

		_, stderr, err := runFlociAtmosInDir(t, repos.HeadDir, env, 5*time.Minute,
			"terraform", "plan", "--affected",
			"--repo-path", repos.BaseDir,
			"-s", "local",
			"--max-concurrency", "4",
			"--execution-summary-file", summaryFile,
			"-i", "false",
		)
		requireTerraformPlanExit(t, err, stderr)
		requireTerraformSummaryNodes(t, summaryFile, []string{"seed-local"})

		_, stderr, err = runFlociAtmosInDir(t, repos.HeadDir, env, 5*time.Minute,
			"terraform", "apply", "--affected",
			"--repo-path", repos.BaseDir,
			"-s", "local",
			"--max-concurrency", "4",
			"-i", "false",
			"-auto-approve",
		)
		require.NoError(t, err, "affected apply failed:\n%s", stderr)
		requireFlociParametersExist(t, client, flociParameterNamesForKeys(testID, []string{"seed"}))
		requireFlociParametersGone(t, client, flociParameterNamesForKeys(testID, []string{"bucket-marker", "queue-marker", "topic-marker", "final-marker"}))

		_, stderr, err = runFlociAtmosInDir(t, repos.HeadDir, env, 5*time.Minute,
			"terraform", "destroy", "--affected",
			"--repo-path", repos.BaseDir,
			"-s", "local",
			"--max-concurrency", "4",
			"-i", "false",
			"-auto-approve",
		)
		require.NoError(t, err, "affected destroy failed:\n%s", stderr)
		requireFlociParametersGone(t, client, flociParameterNames(testID))
	})

	t.Run("include dependents expands affected graph", func(t *testing.T) {
		repos := setupFlociAffectedRepos(t, absWorkdir)
		testID := uniqueFlociTestID(t)
		env := flociCommandEnv(t, endpoint, repos.HeadDir, nil, testID)
		client := newFlociSSMClient(t, endpoint)
		summaryFile := filepath.Join(t.TempDir(), "affected-dependents-summary.json")

		defer bestEffortFlociDestroyInDir(t, repos.HeadDir, env)

		_, stderr, err := runFlociAtmosInDir(t, repos.HeadDir, env, 5*time.Minute,
			"terraform", "plan", "--affected", "--include-dependents",
			"--repo-path", repos.BaseDir,
			"-s", "local",
			"--max-concurrency", "4",
			"--execution-summary-file", summaryFile,
			"-i", "false",
		)
		requireTerraformPlanExit(t, err, stderr)
		requireTerraformSummaryNodes(t, summaryFile, []string{
			"seed-local",
			"bucket-marker-local",
			"queue-marker-local",
			"topic-marker-local",
			"final-marker-local",
		})

		_, stderr, err = runFlociAtmosInDir(t, repos.HeadDir, env, 5*time.Minute,
			"terraform", "apply", "--affected", "--include-dependents",
			"--repo-path", repos.BaseDir,
			"-s", "local",
			"--max-concurrency", "4",
			"-i", "false",
			"-auto-approve",
		)
		require.NoError(t, err, "affected apply with dependents failed:\n%s", stderr)
		requireFlociParametersExist(t, client, flociParameterNames(testID))

		_, stderr, err = runFlociAtmosInDir(t, repos.HeadDir, env, 5*time.Minute,
			"terraform", "destroy", "--affected", "--include-dependents",
			"--repo-path", repos.BaseDir,
			"-s", "local",
			"--max-concurrency", "4",
			"-i", "false",
			"-auto-approve",
		)
		require.NoError(t, err, "affected destroy with dependents failed:\n%s", stderr)
		requireFlociParametersGone(t, client, flociParameterNames(testID))
	})
}

func ensureFlociAtmosRunner(t *testing.T) {
	t.Helper()

	ensureAtmosRunner(t)
}

func runFlociLifecycle(t *testing.T, endpoint, absWorkdir string, maxConcurrency int) {
	t.Helper()

	sandbox := setupFlociSandbox(t, absWorkdir)
	testID := uniqueFlociTestID(t)
	env := flociCommandEnv(t, endpoint, absWorkdir, sandbox, testID)
	client := newFlociSSMClient(t, endpoint)
	parameterNames := flociParameterNames(testID)

	defer bestEffortFlociDestroy(t, env)

	_, stderr, err := runFlociAtmos(
		t, env, 5*time.Minute,
		"terraform", "apply", "--all", "-s", "local",
		"--max-concurrency", fmt.Sprintf("%d", maxConcurrency),
		"-i", "false",
		"-auto-approve",
	)
	require.NoError(t, err, "apply failed:\n%s", stderr)
	requireFlociParametersExist(t, client, parameterNames)

	_, stderr, err = runFlociAtmos(
		t, env, 5*time.Minute,
		"terraform", "destroy", "--all", "-s", "local",
		"--max-concurrency", fmt.Sprintf("%d", maxConcurrency),
		"-i", "false",
		"-auto-approve",
	)
	require.NoError(t, err, "destroy failed:\n%s", stderr)
	requireFlociParametersGone(t, client, parameterNames)
}

func runFlociAliasSequentialProbe(t *testing.T, endpoint, absWorkdir string) {
	t.Helper()

	sandbox := setupFlociSandbox(t, absWorkdir)
	testID := uniqueFlociTestID(t)
	aliasLockDir := t.TempDir()
	env := flociCommandEnv(t, endpoint, absWorkdir, sandbox, testID)
	env["ATMOS_FLOCI_ALIAS_LOCK_DIR"] = aliasLockDir

	defer bestEffortFlociAliasDestroy(t, env)

	_, stderr, err := runFlociAtmos(
		t, env, 5*time.Minute,
		"terraform", "apply",
		"--components=alias-one,alias-two",
		"-s", "local",
		"--max-concurrency", "2",
		"-i", "false",
		"-auto-approve",
	)
	require.NoError(t, err, "alias apply failed:\n%s", stderr)

	eventsPath := filepath.Join(aliasLockDir, "events.log")
	events, err := os.ReadFile(eventsPath)
	require.NoError(t, err, "expected alias probe event log")
	log := string(events)
	oneStart := strings.Index(log, "start alias-one")
	oneEnd := strings.Index(log, "end alias-one")
	twoStart := strings.Index(log, "start alias-two")
	twoEnd := strings.Index(log, "end alias-two")
	require.GreaterOrEqual(t, oneStart, 0, "missing start alias-one")
	require.GreaterOrEqual(t, oneEnd, 0, "missing end alias-one")
	require.GreaterOrEqual(t, twoStart, 0, "missing start alias-two")
	require.GreaterOrEqual(t, twoEnd, 0, "missing end alias-two")
	require.True(t, oneEnd < twoStart || twoEnd < oneStart, "alias execution intervals overlapped")
	require.NoFileExists(t, filepath.Join(aliasLockDir, "overlap"), "aliases sharing one Terraform source path overlapped")
}

func setupFlociSandbox(t *testing.T, absWorkdir string) *testhelpers.SandboxEnvironment {
	t.Helper()

	sandbox, err := testhelpers.SetupSandbox(t, absWorkdir)
	require.NoError(t, err)
	t.Cleanup(sandbox.Cleanup)
	t.Chdir(absWorkdir)
	return sandbox
}

type flociAffectedRepos struct {
	BaseDir string
	HeadDir string
}

func setupFlociAffectedRepos(t *testing.T, absWorkdir string) flociAffectedRepos {
	t.Helper()

	rootDir := t.TempDir()
	baseDir := filepath.Join(rootDir, "base")
	headDir := filepath.Join(rootDir, "head")

	require.NoError(t, copyFlociFixture(absWorkdir, baseDir))
	require.NoError(t, copyFlociFixture(absWorkdir, headDir))
	initFlociGitRepo(t, baseDir, "base")

	seedPath := filepath.Join(headDir, "components", "terraform", "seed", "main.tf")
	seedData, err := os.ReadFile(seedPath)
	require.NoError(t, err)
	updatedSeed := string(seedData) + "\n# affected test change\n"
	require.NoError(t, os.WriteFile(seedPath, []byte(updatedSeed), 0o644))
	initFlociGitRepo(t, headDir, "head")

	return flociAffectedRepos{
		BaseDir: baseDir,
		HeadDir: headDir,
	}
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

func initFlociGitRepo(t *testing.T, dir, message string) {
	t.Helper()

	runFlociGit(t, dir, "init")
	runFlociGit(t, dir, "config", "user.email", "atmos-floci@example.com")
	runFlociGit(t, dir, "config", "user.name", "Atmos Floci")
	runFlociGit(t, dir, "remote", "add", "origin", "https://github.com/example/atmos-floci-fixture.git")
	runFlociGit(t, dir, "add", ".")
	runFlociGit(t, dir, "commit", "-m", message)
}

func runFlociGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", cmdArgs...) //nolint:gosec // Test helper runs fixed git commands against temp fixtures.
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s failed:\n%s", strings.Join(args, " "), string(output))
}

func requireFlociEndpoint(t *testing.T) string {
	t.Helper()

	if os.Getenv("ATMOS_TEST_FLOCI") != "true" {
		t.Skip("set ATMOS_TEST_FLOCI=true and start Floci before running this integration test")
	}

	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	if endpoint == "" {
		endpoint = os.Getenv("FLOCI_ENDPOINT_URL")
	}
	if endpoint == "" {
		endpoint = flociDefaultEndpoint
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

	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		t.Skipf("Floci is not reachable at %s: %v", endpoint, err)
	}
	require.NoError(t, conn.Close())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	require.NoError(t, err)
	// #nosec G107 -- endpoint is an opt-in local Floci test target.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("Floci HTTP endpoint is not reachable at %s: %v", endpoint, err)
	}
	require.NoError(t, resp.Body.Close())

	return endpoint
}

func normalizeFlociEndpoint(endpoint string) string {
	if strings.Contains(endpoint, "://") {
		return endpoint
	}
	return "http://" + endpoint
}

func flociCommandEnv(t *testing.T, endpoint, absWorkdir string, sandbox *testhelpers.SandboxEnvironment, testID string) map[string]string {
	t.Helper()

	env := map[string]string{
		"ATMOS_CLI_CONFIG_PATH":      absWorkdir,
		"ATMOS_FLOCI_TEST_ID":        testID,
		"AWS_ACCESS_KEY_ID":          "test",
		"AWS_SECRET_ACCESS_KEY":      "test",
		"AWS_DEFAULT_REGION":         flociAWSRegion,
		"AWS_REGION":                 flociAWSRegion,
		"AWS_ENDPOINT_URL":           endpoint,
		"FLOCI_ENDPOINT_URL":         endpoint,
		"ATMOS_FLOCI_ALIAS_LOCK_DIR": t.TempDir(),
		"TF_IN_AUTOMATION":           "1",
	}
	if sandbox != nil {
		for key, value := range sandbox.GetEnvironmentVariables() {
			env[key] = value
		}
	}
	return env
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

func flociParameterNames(testID string) []string {
	return flociParameterNamesForKeys(testID, flociMarkerKeys)
}

func flociParameterNamesForKeys(testID string, keys []string) []string {
	names := make([]string, 0, len(keys))
	for _, key := range keys {
		names = append(names, "/atmos/pr5/floci/"+testID+"/"+key)
	}
	return names
}

func requireFlociParametersExist(t *testing.T, client *ssm.Client, names []string) {
	t.Helper()

	for _, name := range names {
		require.Eventually(t, func() bool {
			exists, err := flociParameterExists(client, name)
			if err != nil {
				t.Logf("checking Floci parameter %s: %v", name, err)
				return false
			}
			return exists
		}, 10*time.Second, 200*time.Millisecond, "expected Floci parameter %s to exist", name)
	}
}

func requireFlociParametersGone(t *testing.T, client *ssm.Client, names []string) {
	t.Helper()

	for _, name := range names {
		require.Eventually(t, func() bool {
			exists, err := flociParameterExists(client, name)
			if err != nil {
				t.Logf("checking Floci parameter %s: %v", name, err)
				return false
			}
			return !exists
		}, 10*time.Second, 200*time.Millisecond, "expected Floci parameter %s to be deleted", name)
	}
}

func flociParameterExists(client *ssm.Client, name string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(name)})
	if err == nil {
		return true, nil
	}
	var notFound *ssmtypes.ParameterNotFound
	if errors.As(err, &notFound) {
		return false, nil
	}
	return false, err
}

func bestEffortFlociDestroy(t *testing.T, env map[string]string) {
	t.Helper()

	_, stderr, err := runFlociAtmos(
		t, env, 3*time.Minute,
		"terraform", "destroy", "--all", "-s", "local",
		"--max-concurrency", "4",
		"-i", "false",
		"-auto-approve",
	)
	if err != nil {
		t.Logf("best-effort Floci cleanup failed: %v\n%s", err, stderr)
	}
}

func bestEffortFlociDestroyInDir(t *testing.T, dir string, env map[string]string) {
	t.Helper()

	_, stderr, err := runFlociAtmosInDir(t, dir, env, 3*time.Minute,
		"terraform", "destroy", "--all", "-s", "local",
		"--max-concurrency", "4",
		"-i", "false",
		"-auto-approve",
	)
	if err != nil {
		t.Logf("best-effort Floci cleanup failed: %v\n%s", err, stderr)
	}
}

func bestEffortFlociAliasDestroy(t *testing.T, env map[string]string) {
	t.Helper()

	_, stderr, err := runFlociAtmos(
		t, env, 3*time.Minute,
		"terraform", "destroy",
		"--components=alias-one,alias-two",
		"-s", "local",
		"--max-concurrency", "2",
		"-i", "false",
		"-auto-approve",
	)
	if err != nil {
		t.Logf("best-effort Floci alias cleanup failed: %v\n%s", err, stderr)
	}
}

func requireTerraformPlanExit(t *testing.T, err error, stderr string) {
	t.Helper()

	if err == nil {
		return
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
		return
	}
	require.NoError(t, err, "terraform plan failed:\n%s", stderr)
}

func requireTerraformSummaryNodes(t *testing.T, summaryFile string, expected []string) {
	t.Helper()

	data, err := os.ReadFile(summaryFile)
	require.NoError(t, err)

	var summary struct {
		Results []struct {
			NodeID    string `json:"node_id"`
			Processed bool   `json:"processed"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal(data, &summary))

	actual := make([]string, 0, len(summary.Results))
	for _, result := range summary.Results {
		if result.Processed {
			actual = append(actual, result.NodeID)
		}
	}
	require.ElementsMatch(t, expected, actual)
}

func uniqueFlociTestID(t *testing.T) string {
	t.Helper()

	name := strings.ToLower(t.Name())
	name = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), name)
}
