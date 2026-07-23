package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/ci"
	githubprovider "github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	testEnvFakeTerraform           = "_ATMOS_TEST_FAKE_TERRAFORM"
	testEnvFakeTerraformSelectFail = "_ATMOS_TEST_FAKE_TERRAFORM_SELECT_FAIL"
	testEnvRunLogGroupPipeline     = "_ATMOS_TEST_RUN_LOG_GROUP_PIPELINE"
	testEnvPipelineBackendType     = "_ATMOS_TEST_PIPELINE_BACKEND_TYPE"
	testEnvPipelineSkipInit        = "_ATMOS_TEST_PIPELINE_SKIP_INIT"
)

// TestMain is the entry point for the internal/exec test binary.
// It intercepts several env vars before any test runs, enabling tests to use
// the test binary itself as a portable subprocess — no Unix-only binaries required.
//
// Supported env vars (processed in declaration order):
//
//	_ATMOS_TEST_COUNTER_FILE=<path>  — if set, append one byte ("x") to <path>
//	                                   on every invocation (for single-invocation
//	                                   regression guard in terraform_execute_single_invocation_test.go).
//	_ATMOS_TEST_ARGS_FILE=<path>     — if set, write subprocess arguments and exit
//	                                   successfully (for command argument assertions).
//	_ATMOS_TEST_STDOUT=<text>         — if set, write text to stdout.
//	_ATMOS_TEST_STDERR=<text>         — if set, write text to stderr.
//	_ATMOS_TEST_EXIT_ONE=1           — if set, exit 1 immediately after the optional
//	                                   counter-file write (for workspace recovery tests).
func TestMain(m *testing.M) {
	// Initialize the I/O writer and ui formatter so data.Write*/ui.Write* calls
	// (used throughout internal/exec and its pkg/ci dependency, e.g. CI log
	// groups) don't panic or silently no-op — including in the
	// runLogGroupPipelineForTest subprocess re-exec path below, which is its
	// own process invocation with no other test's init to inherit.
	if ioCtx, err := iolib.NewContext(); err == nil {
		data.InitWriter(ioCtx)
		ui.InitFormatter(ioCtx)
	}

	if os.Getenv(testEnvFakeTerraform) == "1" {
		os.Exit(runFakeTerraformForTest())
	}
	if os.Getenv(testEnvRunLogGroupPipeline) == "1" {
		os.Exit(runLogGroupPipelineForTest())
	}

	// Write a single byte to the counter file on every invocation.
	// This lets tests count how many times the subprocess was spawned by reading
	// the file length: len(file) == number of invocations.
	if counterFile := os.Getenv("_ATMOS_TEST_COUNTER_FILE"); counterFile != "" {
		fd, err := os.OpenFile(counterFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fd.WriteString("x")
			_ = fd.Close()
		}
	}

	if argsFile := os.Getenv("_ATMOS_TEST_ARGS_FILE"); argsFile != "" {
		_ = os.WriteFile(argsFile, []byte(strings.Join(os.Args[1:], "\n")), 0o600)
		os.Exit(0)
	}

	wroteOutput := false
	if stdout := os.Getenv("_ATMOS_TEST_STDOUT"); stdout != "" {
		_, _ = os.Stdout.WriteString(stdout)
		wroteOutput = true
	}
	if stderr := os.Getenv("_ATMOS_TEST_STDERR"); stderr != "" {
		_, _ = os.Stderr.WriteString(stderr)
		wroteOutput = true
	}
	if wroteOutput {
		os.Exit(0)
	}

	// Subprocess helper: when the test binary is invoked as the "terraform" command,
	// this env var causes it to exit 1 immediately, simulating a failed workspace
	// command without requiring the POSIX "false" command.
	if os.Getenv("_ATMOS_TEST_EXIT_ONE") == "1" {
		os.Exit(1)
	}

	// Isolate the Terraform provider plugin cache for this package's tests.
	// Terraform's plugin cache is NOT safe for concurrent use, and `go test ./...`
	// runs package test binaries in parallel. Sharing the global XDG plugin cache
	// (TF_PLUGIN_CACHE_DIR -> ~/.cache/atmos/terraform/plugins) with other packages'
	// concurrent terraform invocations races on Windows, surfacing as
	// "plugin cache dir cannot be opened" / "Required plugins are not installed".
	// Redirecting XDG_CACHE_HOME to a per-binary temp dir removes the contention.
	// The real-terraform tests in this package run serially, so they safely share
	// this private cache (isolated and faster: a single provider download).
	// TestMain only has *testing.M, so t.TempDir/t.Setenv are unavailable here;
	// os.MkdirTemp/os.Setenv with explicit cleanup below is the only option.
	cacheDir, err := os.MkdirTemp("", "atmos-exec-xdg-cache-") //nolint:lintroller // no *testing.T in TestMain; cleaned up below.
	if err == nil {
		_ = os.Setenv("XDG_CACHE_HOME", cacheDir) //nolint:lintroller // no *testing.T in TestMain; process-level isolation for the whole binary.
	}

	code := m.Run()

	if cacheDir != "" {
		_ = os.RemoveAll(cacheDir) // os.Exit skips defers; clean up explicitly.
	}
	os.Exit(code)
}

func runFakeTerraformForTest() int {
	args := os.Args[1:]
	fmt.Printf("fake terraform %s\n", strings.Join(args, " "))
	if os.Getenv(testEnvFakeTerraformSelectFail) == "1" &&
		len(args) >= 3 &&
		args[0] == subcommandWorkspace &&
		args[1] == "select" {
		fmt.Fprintf(os.Stderr, "Workspace %q doesn't exist.\n", args[2])
		return 1
	}
	return 0
}

func runLogGroupPipelineForTest() int {
	ci.Register(githubprovider.NewProvider())

	componentPath := filepath.Join(os.TempDir(), fmt.Sprintf("atmos-log-group-pipeline-%d", os.Getpid()))
	if err := os.MkdirAll(componentPath, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "creating temp component path: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(componentPath) }()

	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolving test executable: %v\n", err)
		return 1
	}

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.CI.Enabled = true
	atmosConfig.CI.Groups.Mode = ci.GroupModeAuto

	componentEnv := []string{testEnvFakeTerraform + "=1"}
	if os.Getenv(testEnvFakeTerraformSelectFail) == "1" {
		componentEnv = append(componentEnv, testEnvFakeTerraformSelectFail+"=1")
	}

	info := schema.ConfigAndStacksInfo{
		SubCommand:           "plan",
		SkipInit:             os.Getenv(testEnvPipelineSkipInit) == "1",
		ComponentBackendType: os.Getenv(testEnvPipelineBackendType),
		TerraformWorkspace:   "dev",
		Command:              exePath,
		ComponentEnvList:     componentEnv,
	}
	execCtx := &componentExecContext{
		componentPath: componentPath,
		varFile:       "vars.tfvars",
		planFile:      "plan.tfplan",
		workingDir:    componentPath,
	}

	if err := executeCommandPipeline(&atmosConfig, &info, execCtx); err != nil {
		fmt.Fprintf(os.Stderr, "pipeline failed: %v\n", err)
		return 1
	}
	return 0
}
