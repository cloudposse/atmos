package docker

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	// Setting this variable (fakeHelperDirEnv) turns the test binary into a fake
	// docker-credential-* helper that records each invocation under the given directory.
	fakeHelperDirEnv = "_ATMOS_TEST_FAKE_DOCKER_CRED_HELPER_DIR"
	// The mode variable (fakeHelperModeEnv) selects the fake helper's behavior: "ok" (default), "notfound", "fail",
	// "notimplemented" (reply on stdout), "notimplemented-stderr" or "hang".
	fakeHelperModeEnv = "_ATMOS_TEST_FAKE_DOCKER_CRED_HELPER_MODE"
	// The credsStore/credHelpers value (fakeHelperName) that routes to the fake helper.
	fakeHelperName = "atmostest"
	// The file (fakeHelperCallsFile) that records one JSON object per helper invocation.
	fakeHelperCallsFile = "calls.jsonl"
	// The credential the fake helper returns for a get action.
	fakeHelperPrevUser   = "prev-user"
	fakeHelperPrevSecret = "prev-secret"
)

// fakeHelperCall is one recorded invocation of the fake credential helper.
type fakeHelperCall struct {
	Action string `json:"action"`
	Input  string `json:"input"`
}

func TestMain(m *testing.M) {
	// When re-executed as a fake docker-credential-* helper, speak the helper
	// protocol instead of running the tests. This keeps the tests from ever
	// executing a real credential helper and works on every platform.
	if dir, ok := os.LookupEnv(fakeHelperDirEnv); ok && dir != "" {
		mode, _ := os.LookupEnv(fakeHelperModeEnv)
		os.Exit(runFakeCredentialHelper(dir, mode, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// runFakeCredentialHelper implements the subset of the docker-credential-helpers
// protocol (store/erase) that ConfigManager uses.
func runFakeCredentialHelper(dir, mode string, args []string) int {
	if len(args) != 1 {
		return 2
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return 3
	}
	switch mode {
	case "notfound":
		_, _ = io.WriteString(os.Stdout, helperNotFoundMessage+"\n")
		return 1
	case "fail":
		_, _ = io.WriteString(os.Stdout, "fake helper failure\n")
		_, _ = io.WriteString(os.Stderr, "fake helper diagnostics\n")
		return 1
	case "notimplemented":
		// What docker-credential-ecr-login replies to store and erase.
		_, _ = io.WriteString(os.Stdout, helperNotImplementedMessage+"\n")
		return 1
	case "notimplemented-stderr":
		_, _ = io.WriteString(os.Stderr, helperNotImplementedMessage+"\n")
		return 1
	case "hang":
		time.Sleep(30 * time.Second)
		return 0
	}
	f, err := os.OpenFile(filepath.Join(dir, fakeHelperCallsFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return 4
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(fakeHelperCall{Action: args[0], Input: string(input)}); err != nil {
		return 5
	}
	if args[0] == helperActionGet {
		// A stored credential, so a restore after a failed save can be verified.
		_, _ = io.WriteString(os.Stdout, `{"ServerURL":"`+string(input)+`","Username":"`+fakeHelperPrevUser+`","Secret":"`+fakeHelperPrevSecret+`"}`)
	}
	return 0
}

// installFakeCredentialHelper places a copy of the test binary on PATH as
// docker-credential-atmostest and returns the directory where its calls are recorded.
// PATH is replaced (not prepended) so no real docker-credential-* helper can run.
func installFakeCredentialHelper(t *testing.T, mode string) string {
	t.Helper()

	exe, err := os.Executable()
	require.NoError(t, err)

	binDir := t.TempDir()
	name := "docker-credential-" + fakeHelperName
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dst := filepath.Join(binDir, name)
	if err := os.Link(exe, dst); err != nil {
		copyFile(t, exe, dst)
	}

	callsDir := t.TempDir()
	t.Setenv("PATH", binDir)
	t.Setenv(fakeHelperDirEnv, callsDir)
	t.Setenv(fakeHelperModeEnv, mode)
	return callsDir
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	require.NoError(t, err)
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	require.NoError(t, err)
	_, err = io.Copy(out, in)
	require.NoError(t, err)
	require.NoError(t, out.Close())
}

// readFakeHelperCalls returns the recorded fake helper invocations in order.
func readFakeHelperCalls(t *testing.T, callsDir string) []fakeHelperCall {
	t.Helper()
	f, err := os.Open(filepath.Join(callsDir, fakeHelperCallsFile))
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	defer f.Close()

	var calls []fakeHelperCall
	dec := json.NewDecoder(f)
	for dec.More() {
		var c fakeHelperCall
		require.NoError(t, dec.Decode(&c))
		calls = append(calls, c)
	}
	return calls
}
