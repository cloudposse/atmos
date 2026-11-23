package acr

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/auth/credentials/credtest"
)

// TestMain keeps every test in this package away from the real OS keychain: the credential store
// defaults to an in-memory keyring. If another TestMain is ever added to this package, move the
// credtest.IsolateKeyringMain call into it instead of declaring a second TestMain.
func TestMain(m *testing.M) {
	// When re-executed as a fake docker-credential-* helper, speak the helper
	// protocol instead of running the tests (see fake_credential_helper_test.go).
	if mode := os.Getenv(fakeACRHelperModeEnv); mode != "" {
		os.Exit(runFakeACRCredentialHelper(mode, os.Args[1:]))
	}
	os.Exit(credtest.IsolateKeyringMain(m))
}
