package toolchain

import (
	"os"
	"testing"

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
