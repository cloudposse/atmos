package tests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
)

// localGitOpsExampleDir is the example project the GitOps E2E tests drive, so the
// human-runnable demo and the automated test share one source of truth.
const localGitOpsExampleDir = "../examples/local-gitops"

// localGitOpsStack is the single stack the example defines.
const localGitOpsStack = "local"

// gitServerEmulator / k8sEmulator are the emulator component names in the example.
const (
	gitServerEmulator = "gitserver"
	k8sEmulator       = "kubernetes"
)

// gitOpsE2EEnv builds the command environment: the copied example as the config
// root, with any inherited git identity/credentials cleared so the only auth in
// play is the throwaway atmos:atmos embedded in the emulator remote URL.
func gitOpsE2EEnv(workdir string) map[string]string {
	return map[string]string{
		"ATMOS_CLI_CONFIG_PATH": workdir,
		"GIT_TERMINAL_PROMPT":   "0",
		// Reproduce a CI runner: no global/system git identity. The provisioner's
		// commit must succeed via the repo's configured commit.author, not an ambient
		// developer identity. os.DevNull is cross-platform ("/dev/null" or "NUL").
		"GIT_CONFIG_GLOBAL": os.DevNull,
		"GIT_CONFIG_SYSTEM": os.DevNull,
	}
}

// requireGitOpsRuntime gates the GitOps E2E tests on the opt-in flag plus an
// available container runtime, mirroring the native AWS emulator E2E.
func requireGitOpsRuntime(t *testing.T) {
	t.Helper()
	if os.Getenv("ATMOS_TEST_FLOCI") != "true" {
		t.Skip("set ATMOS_TEST_FLOCI=true to run the local GitOps E2E")
	}
	if _, err := container.DetectRuntime(context.Background()); err != nil {
		t.Skipf("no container runtime available for the local GitOps E2E: %v", err)
	}
}

// TestLocalGitOpsPushE2E proves the PUSH half of the GitOps loop end to end: Atmos
// renders a Kubernetes component and pushes it to the local Gitea Git server
// emulator, with no real Git host and no cluster involved. The assertion clones
// the repository back from the emulator and confirms the rendered manifest — and
// the provenance commit trailer — actually landed.
//
// Opt-in and runtime-gated (ATMOS_TEST_FLOCI=true + a container runtime).
func TestLocalGitOpsPushE2E(t *testing.T) {
	requireGitOpsRuntime(t)
	ensureAtmosRunner(t)
	requireGitBinary(t)

	workdir := copyFlociScenarioFixture(t, localGitOpsExampleDir)
	env := gitOpsE2EEnv(workdir)

	run := func(timeout time.Duration, args ...string) (string, string, error) {
		return runFlociAtmosInDir(t, workdir, env, timeout, args...)
	}

	// Start the Git server emulator; it auto-bootstraps the admin user + repo.
	_, stderr, err := run(3*time.Minute, "emulator", "up", gitServerEmulator, "-s", localGitOpsStack)
	require.NoErrorf(t, err, "emulator up gitserver failed: %s", stderr)
	t.Cleanup(func() {
		_, _, _ = run(2*time.Minute, "emulator", "down", gitServerEmulator, "-s", localGitOpsStack)
	})

	// Render the demo app and push it to the Gitea `deployments` repository. The git
	// provision target needs no cluster/identity — it only writes, commits, and pushes.
	_, stderr, err = run(3*time.Minute, "kubernetes", "apply", "demo-app", "-s", localGitOpsStack, "--target", "deployments")
	require.NoErrorf(t, err, "kubernetes apply --target deployments failed: %s", stderr)

	// Clone the repository back from the emulator and assert the manifests landed.
	clone := filepath.Join(t.TempDir(), "deployments")
	requireGitClone(t, "http://atmos:atmos@localhost:3000/atmos/deployments.git", clone)

	rendered := readTreeUnder(t, filepath.Join(clone, "clusters", "local", "demo-app"))
	require.Contains(t, rendered, "kind: ConfigMap", "pushed tree should contain the rendered ConfigMap")
	require.Contains(t, rendered, "gitops-demo", "pushed manifests should carry the demo namespace")
	require.Contains(t, rendered, "Delivered by Atmos", "pushed ConfigMap should carry the demo message")

	// The commit must carry the provenance trailer the git provisioner adds.
	message := gitLastCommitMessage(t, clone)
	require.Contains(t, message, "Atmos-Component: demo-app", "commit should carry the component provenance trailer")
}

// TestLocalGitOpsRoundTripE2E proves the FULL loop: Atmos pushes the demo app to
// the Gitea emulator, Flux (running in the k3s emulator) watches the repository and
// reconciles the manifests back into the cluster. The assertion polls the cluster
// until the pushed resource appears — the round trip closed without anyone running
// `kubectl apply`.
//
// Opt-in and runtime-gated; it also needs a privileged-capable runtime for k3s and
// network egress to provision the Flux install manifest (pulled JIT by the flux
// component's `source:` field) and pull controller images.
func TestLocalGitOpsRoundTripE2E(t *testing.T) {
	requireGitOpsRuntime(t)
	ensureAtmosRunner(t)

	workdir := copyFlociScenarioFixture(t, localGitOpsExampleDir)
	env := gitOpsE2EEnv(workdir)

	run := func(timeout time.Duration, args ...string) (string, string, error) {
		return runFlociAtmosInDir(t, workdir, env, timeout, args...)
	}
	const id = "--identity"
	const k3sIdentity = "local-k3s"

	// No vendor step: the flux component provisions its install manifest just-in-time
	// from the pinned release asset (its `source:` field) on the apply below.

	// Bring up both emulators (Git server + k3s). Always tear them down.
	for _, emulatorName := range []string{gitServerEmulator, k8sEmulator} {
		_, stderr, err := run(5*time.Minute, "emulator", "up", emulatorName, "-s", localGitOpsStack)
		require.NoErrorf(t, err, "emulator up %s failed: %s", emulatorName, stderr)
		name := emulatorName
		t.Cleanup(func() {
			_, _, _ = run(2*time.Minute, "emulator", "down", name, "-s", localGitOpsStack)
		})
	}

	// Install Flux (controllers + CRDs), then the Git source/sync custom resources.
	_, stderr, err := run(5*time.Minute, "kubernetes", "apply", "flux", "-s", localGitOpsStack, id, k3sIdentity)
	require.NoErrorf(t, err, "apply flux failed: %s", stderr)

	// The CRDs Flux just installed must be established before its custom resources
	// apply; retry the sync apply briefly to absorb that propagation window.
	require.Eventuallyf(t, func() bool {
		_, _, err := run(2*time.Minute, "kubernetes", "apply", "flux-sync", "-s", localGitOpsStack, id, k3sIdentity)
		return err == nil
	}, 3*time.Minute, 10*time.Second, "flux-sync (GitRepository/Kustomization) never applied cleanly")

	// Flux's source-controller does the actual clone from Gitea; wait until it is
	// Ready so the subsequent reconcile is bounded by Flux's interval, not by cold
	// controller startup (image pulls are slow on a memory-constrained runner).
	require.Eventuallyf(t, func() bool {
		stdout := kubectlInCluster(t, run, "-n", "flux-system", "get", "deploy", "source-controller",
			"-o", "jsonpath={.status.readyReplicas}")
		return strings.Contains(stdout, "1")
	}, 6*time.Minute, 15*time.Second, "Flux source-controller never became Ready")

	// Push the demo app to the Gitea `deployments` repository.
	_, stderr, err = run(3*time.Minute, "kubernetes", "apply", "demo-app", "-s", localGitOpsStack, "--target", "deployments")
	require.NoErrorf(t, err, "apply demo-app --target deployments failed: %s", stderr)

	// Close the loop: poll the cluster until Flux has reconciled the pushed ConfigMap
	// into the demo namespace. The k3s image bundles `kubectl`, reached via
	// `emulator exec` (use `kubectl` directly — `k3s kubectl` misparses under exec).
	require.Eventuallyf(t, func() bool {
		stdout := kubectlInCluster(t, run, "-n", "gitops-demo", "get", "configmap", "demo-app", "-o", "name")
		return strings.Contains(stdout, "configmap/demo-app")
	}, 5*time.Minute, 15*time.Second, "Flux never reconciled the pushed demo-app ConfigMap into the cluster")
}

// kubectlInCluster runs `kubectl <args>` inside the k3s emulator container via
// `atmos emulator exec` and returns its stdout (empty on error, so callers can
// poll with require.Eventually).
func kubectlInCluster(t *testing.T, run func(time.Duration, ...string) (string, string, error), args ...string) string {
	t.Helper()
	execArgs := append([]string{"emulator", "exec", k8sEmulator, "-s", localGitOpsStack, "--", "kubectl"}, args...)
	stdout, _, err := run(time.Minute, execArgs...)
	if err != nil {
		return ""
	}
	return stdout
}

// requireGitBinary skips the test when the git CLI is unavailable (the verification
// clone shells out to it).
func requireGitBinary(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git CLI not available for clone-back verification: %v", err)
	}
}

// requireGitClone clones url into dir, failing the test on error.
func requireGitClone(t *testing.T, url, dir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", url, dir)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git clone of the emulator repo failed: %s", string(out))
}

// gitLastCommitMessage returns the full message (subject + body + trailers) of the
// repository's most recent commit.
func gitLastCommitMessage(t *testing.T, dir string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "log", "-1", "--format=%B%n%(trailers)")
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git log failed: %s", string(out))
	return string(out)
}

// readTreeUnder concatenates the contents of every regular file under root, so a
// test can assert on rendered manifests without depending on exact filenames.
func readTreeUnder(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
		return nil
	})
	require.NoErrorf(t, err, "reading pushed tree under %s", root)
	return b.String()
}
