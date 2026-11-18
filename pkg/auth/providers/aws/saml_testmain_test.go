package aws

import (
	"os"
	"testing"
)

// fakePlaywrightNodeEnv makes the test binary act as the Node.js runtime of a fake Playwright
// driver: invoked as `<test binary> cli.js --version`, it prints "Version <value>" and exits 0.
// Tests point PLAYWRIGHT_NODEJS_PATH at os.Executable() so playwright-go's driver checks run
// without a real Node.js.
const fakePlaywrightNodeEnv = "_ATMOS_TEST_FAKE_PLAYWRIGHT_NODE_VERSION"

func TestMain(m *testing.M) {
	// Only act as Node.js for the driver's `<cli.js> --version` probe, so a normal `go test` process that
	// inherits the variable still runs the tests.
	if version := os.Getenv(fakePlaywrightNodeEnv); version != "" && len(os.Args) == 3 && os.Args[2] == "--version" {
		_, _ = os.Stdout.WriteString("Version " + version + "\n")
		os.Exit(0)
	}
	os.Exit(m.Run())
}
