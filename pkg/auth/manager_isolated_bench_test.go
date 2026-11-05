package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// BenchmarkStoreIdentityResolution compares the two ways a store identity has been
// resolved. "shared" is the pre-#3544 path: Authenticate on the component's manager
// and read its stack info back (which is what leaked the identity). "isolated" is the
// fixed path: AuthenticateIsolated, which builds a separate manager per call. Both run
// against a warm memory keyring, so the difference is the per-call construction cost.
func BenchmarkStoreIdentityResolution(b *testing.B) {
	manager := isolatedTestManager(b)
	_, err := manager.Authenticate(b.Context(), "deployer")
	require.NoError(b, err)

	b.Run("shared", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := manager.Authenticate(b.Context(), "automation"); err != nil {
				b.Fatal(err)
			}
			if manager.GetStackInfo().AuthContext.AWS == nil {
				b.Fatal("missing AWS context")
			}
		}
	})

	b.Run("isolated", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			ctx, err := manager.AuthenticateIsolated(b.Context(), "automation")
			if err != nil {
				b.Fatal(err)
			}
			if ctx.AWS == nil {
				b.Fatal("missing AWS context")
			}
		}
	})
}
