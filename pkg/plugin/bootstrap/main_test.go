package bootstrap

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/auth/credentials/credtest"
)

func TestMain(m *testing.M) {
	// Tests that depend on CI behavior set inCI themselves; the host CI must not change the rest.
	inCI = func() bool { return false }
	// Never touch the real OS keychain: default the credential store to an in-memory keyring.
	os.Exit(credtest.IsolateKeyringMain(m))
}
