package tests

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
)

func TestScaffoldAWSLandingZoneFlociE2E(t *testing.T) {
	runScaffoldTerraformE2E(t, &scaffoldTerraformE2E{
		Template:   "aws/landing-zone",
		Project:    "e2e-aws-lz",
		Emulator:   "aws",
		Components: []string{"kms", "audit-trail", "baseline", "monitoring", "iam-baseline"},
	})
}

func TestScaffoldAWSAppFlociE2E(t *testing.T) {
	runScaffoldTerraformE2E(t, &scaffoldTerraformE2E{
		Template:   "aws/app",
		Project:    "e2e-aws-app",
		Emulator:   "aws",
		Components: []string{"app"},
	})
}

func TestScaffoldGCPLandingZoneFlociE2E(t *testing.T) {
	runScaffoldTerraformE2E(t, &scaffoldTerraformE2E{
		Template:   "gcp/landing-zone",
		Project:    "e2e-gcp-lz",
		Emulator:   "gcp",
		Components: []string{"foundation"},
	})
}

func TestScaffoldAzureLandingZoneFlociE2E(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Floci Azure TLS certificates are currently reliable in Linux CI, not local macOS")
	}
	runScaffoldTerraformE2E(t, &scaffoldTerraformE2E{
		Template:   "azure/landing-zone",
		Project:    "e2e-az-lz",
		Emulator:   "azure",
		Components: []string{"network"},
		ExtraEnv: map[string]string{
			"FLOCI_AZ_TLS_ENABLED": "true",
		},
	})
}

func TestInitFromTemplateRepoGiteaE2E(t *testing.T) {
	requireGitOpsRuntime(t)
	requireGitBinary(t)
	ensureAtmosRunner(t)

	workdir := copyFlociScenarioFixture(t, localGitOpsExampleDir)
	env := gitOpsE2EEnv(workdir)
	run := func(timeout time.Duration, args ...string) (string, string, error) {
		return runFlociAtmosInDir(t, workdir, env, timeout, args...)
	}

	_, stderr, err := run(3*time.Minute, "emulator", "up", gitServerEmulator, "-s", localGitOpsStack)
	require.NoErrorf(t, err, "emulator up gitserver failed: %s", stderr)
	t.Cleanup(func() {
		_, _, _ = run(2*time.Minute, "emulator", "down", gitServerEmulator, "-s", localGitOpsStack)
	})
	waitForTCP(t, "localhost:3000", 90*time.Second)

	clone := filepath.Join(t.TempDir(), "deployments")
	requireGitClone(t, "http://atmos:atmos@localhost:3000/atmos/deployments.git", clone)
	branch := "template-ref"
	runGit(t, clone, "checkout", "-B", branch)
	templateSubdir := filepath.Join(clone, "templates", "aws-app")
	require.NoError(t, copyFlociFixture(filepath.Join(repoRoot, "examples", "scaffolds", "aws", "app"), templateSubdir))
	runGit(t, clone, "add", ".")
	runGit(
		t, clone,
		"-c", "user.name=Atmos Test",
		"-c", "user.email=atmos-test@localhost",
		"-c", "commit.gpgsign=false",
		"commit", "-m", "Add scaffold template",
	)
	runGit(t, clone, "push", "origin", branch)

	target := filepath.Join(t.TempDir(), "from-template-repo")
	templateURL := "git::http://atmos:atmos@localhost:3000/atmos/deployments.git//templates/aws-app"
	runAtmosForScaffoldTest(t, workdir, map[string]string{
		"ATMOS_INIT_INTERACTIVE": "false",
		"GIT_TERMINAL_PROMPT":    "0",
		"XDG_CACHE_HOME":         filepath.Join(t.TempDir(), ".cache"),
	}, 3*time.Minute, "init", templateURL, target, "--ref", branch, "--no-git", "--set", "project_name=e2e-template")
	runAtmosForScaffoldTest(t, target, map[string]string{
		"ATMOS_CLI_CONFIG_PATH": target,
	}, 2*time.Minute, "validate", "stacks")
}

type scaffoldTerraformE2E struct {
	Template   string
	Project    string
	Emulator   string
	Components []string
	ExtraEnv   map[string]string
}

func runScaffoldTerraformE2E(t *testing.T, tc *scaffoldTerraformE2E) {
	t.Helper()

	requireScaffoldE2ERuntime(t)
	RequireTerraformOrTofu(t)
	ensureAtmosRunner(t)

	target := filepath.Join(t.TempDir(), "project")
	env := scaffoldE2EEnv(t, target, tc.ExtraEnv)
	runAtmosForScaffoldTest(
		t, "", map[string]string{
			"ATMOS_INIT_INTERACTIVE":         "false",
			"ATMOS_SCAFFOLD_SOURCE_OVERRIDE": filepath.Join(repoRoot, "examples", "scaffolds"),
			"XDG_CACHE_HOME":                 filepath.Join(t.TempDir(), ".cache"),
		}, 2*time.Minute,
		"init", tc.Template, target,
		"--no-git",
		"--set", "project_name="+tc.Project,
		"--set", "terraform_command="+terraformCommandForScaffoldE2E(t),
	)
	runAtmosForScaffoldTest(t, target, env, 2*time.Minute, "validate", "stacks")

	runAtmosForScaffoldTest(t, target, env, 3*time.Minute, "emulator", "up", tc.Emulator, "-s", "dev")
	t.Cleanup(func() {
		_, _, _ = runFlociAtmosInDir(t, target, env, 2*time.Minute, "emulator", "down", tc.Emulator, "-s", "dev")
	})

	for _, component := range tc.Components {
		runAtmosForScaffoldTest(t, target, env, 5*time.Minute, "terraform", "apply", component, "-s", "dev", "-auto-approve")
	}
	for i := len(tc.Components) - 1; i >= 0; i-- {
		component := tc.Components[i]
		t.Cleanup(func() {
			_, _, _ = runFlociAtmosInDir(t, target, env, 5*time.Minute, "terraform", "destroy", component, "-s", "dev", "-auto-approve")
		})
	}
}

func requireScaffoldE2ERuntime(t *testing.T) {
	t.Helper()

	if os.Getenv("ATMOS_TEST_FLOCI") != "true" {
		t.Skip("set ATMOS_TEST_FLOCI=true to run scaffold Floci E2E tests")
	}
	if _, err := container.DetectRuntime(context.Background()); err != nil {
		t.Skipf("no container runtime available for scaffold Floci E2E tests: %v", err)
	}
}

func scaffoldE2EEnv(t *testing.T, target string, extra map[string]string) map[string]string {
	t.Helper()

	env := map[string]string{
		"ATMOS_CLI_CONFIG_PATH":          target,
		"ATMOS_FLOCI_ALIAS_LOCK_DIR":     t.TempDir(),
		"TF_IN_AUTOMATION":               "1",
		"AWS_ACCESS_KEY_ID":              "",
		"AWS_SECRET_ACCESS_KEY":          "",
		"AWS_SESSION_TOKEN":              "",
		"AWS_PROFILE":                    "",
		"AWS_DEFAULT_REGION":             "",
		"AWS_REGION":                     "",
		"AWS_ENDPOINT_URL":               "",
		"AWS_EC2_METADATA_DISABLED":      "true",
		"ARM_SKIP_PROVIDER_REGISTER":     "true",
		"ARM_SKIP_PROVIDER_REGISTRATION": "true",
	}
	for key, value := range extra {
		env[key] = value
	}
	return env
}

func terraformCommandForScaffoldE2E(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("terraform"); err == nil {
		return "terraform"
	}
	if _, err := exec.LookPath("tofu"); err == nil {
		return "tofu"
	}
	t.Skip("terraform or tofu is required for scaffold Floci E2E tests")
	return ""
}

func waitForTCP(t *testing.T, address string, timeout time.Duration) {
	t.Helper()

	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, timeout, 2*time.Second, "service did not start listening on %s", address)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = mergeCommandEnv(os.Environ(), map[string]string{
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_CONFIG_GLOBAL":   os.DevNull,
		"GIT_CONFIG_SYSTEM":   os.DevNull,
	})
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
}
