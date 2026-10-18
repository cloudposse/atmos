package claudecode

import (
	"os"
	"testing"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestMain lets the test binary impersonate the `claude` CLI when the fake-scenario env var
// is set (see fake_claude_test.go and CLAUDE.md "Subprocess helpers in tests"). Otherwise it
// initializes the ui formatter, which NewClient needs to print MCP status, and runs the tests.
func TestMain(m *testing.M) {
	if scenario := os.Getenv(fakeScenarioEnv); scenario != "" {
		os.Exit(runFakeClaude(scenario))
	}

	ioCtx, err := iolib.NewContext()
	if err != nil {
		panic(err)
	}
	ui.InitFormatter(ioCtx)

	os.Exit(m.Run())
}
