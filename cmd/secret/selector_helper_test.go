package secret

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const selectorHelperArg = "--atmos-secret-selector-helper"

// TestMain provides an empty-output executable for selector side-effect assertions.
// Handling the sentinel before testing flags are parsed keeps the helper independent
// of the caller's test selection, shuffle order, and the host's installed commands.
func TestMain(m *testing.M) {
	if len(os.Args) == 3 && os.Args[1] == selectorHelperArg {
		if err := os.WriteFile(os.Args[2], []byte("selector executed"), 0o600); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func selectorHelperCommand(t *testing.T, marker string) string {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	// Paths stay in environment values, so neither YAML nor template rendering
	// interprets their quotes or backslashes. Double-quoted shell expansions
	// preserve each complete path as one argument on every supported platform.
	markerVariable := fmt.Sprintf("ATMOS_SECRET_TEST_MARKER_%X", sha256.Sum256([]byte(marker)))
	t.Setenv("ATMOS_SECRET_TEST_EXECUTABLE", executable)
	t.Setenv(markerVariable, marker)
	return fmt.Sprintf(`"${ATMOS_SECRET_TEST_EXECUTABLE}" %s "${%s}"`, selectorHelperArg, markerVariable)
}
