package toolchain

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {
	// Initialize Viper environment variable bindings for tests.
	// This ensures that GITHUB_TOKEN from CI is properly available to the tests.
	// Bind both ATMOS_GITHUB_TOKEN and GITHUB_TOKEN to the "github-token" key.
	viper.BindEnv("github-token", "ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN")

	// Run tests.
	os.Exit(m.Run())
}

// executeCommand is a helper function for testing cobra commands.
// Currently unused but kept for potential future test expansion.
func executeCommand(root *cobra.Command, args ...string) (output string, err error) {
	buf := new(strings.Builder)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err = root.Execute()
	return buf.String(), err
}
