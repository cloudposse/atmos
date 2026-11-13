//nolint:forbidigo // Test tool discovery inspects and updates the process environment.
package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var testToolPathLocks sync.Map

type cachedTestTool struct {
	Repo    string
	Version string
	Binary  string
}

var cachedTestTools = []cachedTestTool{
	{Repo: "opentofu/opentofu", Version: "1.12.2", Binary: "tofu"},
	{Repo: "hashicorp/terraform", Version: "1.15.6", Binary: "terraform"},
	{Repo: "hashicorp/packer", Version: "1.14.2", Binary: "packer"},
	{Repo: "helmfile/helmfile", Version: "v1.1.0", Binary: "helmfile"},
	{Repo: "helm/helm", Version: "v3.19.2", Binary: "helm"},
}

// RequireExecutable checks if an executable is available in PATH.
// Unlike requireExecutablePath, a bypass (ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true)
// is a full no-op here: callers that only need a boolean gate, not the resolved
// path, don't need the tool to actually be resolvable when checks are disabled.
func RequireExecutable(t *testing.T, name string, purpose string) {
	t.Helper()

	prependCachedTestTool(name)

	if !ShouldCheckPreconditions() {
		return
	}

	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("'%s' not found in PATH: required for %s. Install the tool or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true",
			name, purpose)
	}
}

// requireExecutablePath resolves and returns the executable's path, skipping
// the test if it is not found. Callers that need the resolved path (to, e.g.,
// copy the binary into an isolated per-test directory) should use this instead
// of a separate, independent exec.LookPath call afterward. Two independent
// LookPath calls create a TOCTOU race: the Windows acceptance job runs
// packages concurrently, and another package's test can mutate the shared
// Atmos toolchain cache directory (see prependCachedTestTool) between the two
// calls, turning a binary that existed a moment ago into a spurious
// "not found" error. Resolving the path exactly once here closes that window.
// These constants bound how long requireExecutablePath polls exec.LookPath
// before giving up. CI installs each toolchain binary and updates PATH in an
// earlier job step; on some runners (observed on Windows) that update has
// occasionally not been visible yet to the very first lookup here, so a poll
// avoids failing on a one-off resolution lag rather than the tool actually
// being absent. 15s (rather than a token couple of seconds) is deliberate:
// the Windows acceptance job runs many Go packages' test binaries
// concurrently on constrained runners, and this exact race recurred in CI
// even after closing one confirmed root cause (a test writing real installs
// into the shared toolchain cache dir, see the pkg/toolchain fix referenced
// in this repo's git history) -- so a short window risks masking further,
// not-yet-identified contention rather than tolerating genuine scheduling
// delay under load.
// These are package-level vars, not consts, solely so tests can shrink them
// temporarily (see withShortExecutablePathRetry in
// precondition_require_executable_path_test.go) to exercise the retry loop's
// sleep/exhaustion branches without waiting out the real 15s production
// window. Production code paths never mutate them.
var (
	executablePathRetryTimeout  = 15 * time.Second
	executablePathRetryInterval = 100 * time.Millisecond
)

func requireExecutablePath(t *testing.T, name string, purpose string) string {
	t.Helper()

	prependCachedTestTool(name)

	deadline := time.Now().Add(executablePathRetryTimeout)
	var path string
	var err error
	for {
		path, err = exec.LookPath(name)
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(executablePathRetryInterval)
	}
	if err != nil {
		forensics := executableLookupForensics(name)
		if !ShouldCheckPreconditions() {
			// ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true (set for every CI
			// acceptance job) means the caller wants a hard failure instead of
			// a skip when a tool that CI is expected to provision is missing --
			// never silently hand back an empty path, which turns into a far
			// more confusing "open : file not found" error downstream.
			t.Fatalf("'%s' not found in PATH: required for %s. Precondition checks are disabled (ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true), so this is a hard failure rather than a skip: %v\n%s",
				name, purpose, err, forensics)
		}
		t.Skipf("'%s' not found in PATH: required for %s. Install the tool or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true\n%s",
			name, purpose, forensics)
	}
	return path
}

// executableLookupForensics reports the on-disk state of every toolchain-like
// PATH entry at the moment a binary lookup failed. The Windows acceptance job
// has a recurring "installed tool vanished mid-suite" failure whose root cause
// is still being pinned down (writer-side install/uninstall windows in the
// shared toolchain cache are the leading theory); this dump turns the next CI
// failure into direct evidence of WHICH mechanism fired: directory deleted
// (uninstall/reinstall), tree renamed aside (onedir .bak present), writer
// mid-flight (.lock sibling present), or PATH itself corrupted (no toolchain
// entries at all).
func executableLookupForensics(name string) string {
	var b strings.Builder
	entries := filepath.SplitList(os.Getenv("PATH"))
	fmt.Fprintf(&b, "-- lookup forensics for %q (%d PATH entries) --\n", name, len(entries))

	candidates := []string{name}
	if withExt := cachedTestToolBinaryNameForOS(name, runtime.GOOS); withExt != name {
		candidates = append(candidates, withExt)
	}

	toolchainEntries := 0
	for _, dir := range entries {
		if !strings.Contains(strings.ToLower(dir), "toolchain") {
			continue
		}
		toolchainEntries++
		describeToolchainPathEntry(&b, dir, candidates)
	}
	if toolchainEntries == 0 {
		b.WriteString("NO toolchain-like PATH entries found -- PATH itself lost the toolchain dirs\n")
	}
	return b.String()
}

// describeToolchainPathEntry appends one PATH entry's on-disk state (target
// presence, directory contents, writer-lock sibling) to the forensics report.
func describeToolchainPathEntry(b *strings.Builder, dir string, candidates []string) {
	present := false
	for _, cand := range candidates {
		// #nosec G703 -- dir comes from this process's own PATH and cand from a fixed test-tool list; read-only stat for diagnostics.
		if st, statErr := os.Stat(filepath.Join(dir, cand)); statErr == nil && !st.IsDir() {
			present = true
			break
		}
	}
	fmt.Fprintf(b, "%s -> target present=%v", dir, present)
	if dirEntries, readErr := os.ReadDir(dir); readErr == nil {
		names := make([]string, 0, len(dirEntries))
		for _, e := range dirEntries {
			names = append(names, e.Name())
		}
		fmt.Fprintf(b, "; contents(%d): %s", len(names), strings.Join(names, ", "))
	} else {
		fmt.Fprintf(b, "; dir unreadable: %v", readErr)
	}
	// The installer's writer lock is a ".lock" sibling of the version dir
	// (see pkg/toolchain/installer); its presence means a writer was active.
	// #nosec G703 -- dir comes from this process's own PATH; read-only stat for diagnostics.
	if _, lockErr := os.Stat(dir + ".lock"); lockErr == nil {
		b.WriteString("; WRITER LOCK PRESENT")
	}
	b.WriteString("\n")
}

func prependCachedTestTool(binary string) {
	tool, ok := cachedTestToolForBinary(binary)
	if !ok {
		return
	}

	lockValue, _ := testToolPathLocks.LoadOrStore(binary, &sync.Once{})
	lockValue.(*sync.Once).Do(func() {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return
		}

		// Unit tests provision tools into test-toolchain, while CI uses the normal
		// Atmos toolchain cache. Check both so package-local integration tests can
		// use the tools installed by the acceptance workflow on every platform.
		for _, binDir := range cachedTestToolBinDirs(cacheDir, tool) {
			if _, ok := cachedTestToolBinaryPath(binDir, tool.Binary); !ok {
				continue
			}

			path := os.Getenv("PATH")
			if path == "" {
				os.Setenv("PATH", binDir)
				return
			}
			os.Setenv("PATH", binDir+string(os.PathListSeparator)+path)
			return
		}
	})
}

func cachedTestToolBinDirs(cacheDir string, tool cachedTestTool) []string {
	toolPath := filepath.Join(filepath.FromSlash(tool.Repo), tool.Version)
	return []string{
		filepath.Join(cacheDir, "atmos", "test-toolchain", "bin", toolPath),
		filepath.Join(cacheDir, "atmos", "toolchain", "bin", toolPath),
	}
}

// cachedTestToolBinaryPath returns the installed test-tool binary path, including
// the .exe artifact generated by the Windows toolchain.
func cachedTestToolBinaryPath(binDir string, binary string) (string, bool) {
	candidates := []string{filepath.Join(binDir, binary)}
	if filepath.Ext(binary) == "" {
		candidates = append(candidates, filepath.Join(binDir, binary+".exe"))
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}

	return "", false
}

// cachedTestToolBinaryName returns the expected on-disk binary name for the
// current OS. Used by test fixture setup, which needs the exact deterministic
// name a real install would produce -- unlike cachedTestToolBinaryPath, which
// tries multiple candidates when looking up an existing cached binary.
func cachedTestToolBinaryName(binary string) string {
	return cachedTestToolBinaryNameForOS(binary, runtime.GOOS)
}

func cachedTestToolBinaryNameForOS(binary, goos string) string {
	if goos == "windows" {
		return binary + ".exe"
	}
	return binary
}

func cachedTestToolBinaryExists(binDir, binary string) bool {
	_, err := os.Stat(filepath.Join(binDir, cachedTestToolBinaryName(binary)))
	return err == nil
}

func cachedTestToolForBinary(binary string) (cachedTestTool, bool) {
	for _, tool := range cachedTestTools {
		if tool.Binary == binary {
			return tool, true
		}
	}
	return cachedTestTool{}, false
}
