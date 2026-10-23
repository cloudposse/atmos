package gke

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/auth/credentials/credtest"
)

// TestMain keeps every test in this package away from the real OS keychain: the credential store
// defaults to an in-memory keyring. If another TestMain is ever added to this package, move the
// credtest.IsolateKeyringMain call into it instead of declaring a second TestMain.
func TestMain(m *testing.M) {
	os.Exit(credtest.IsolateKeyringMain(m))
}
