//go:build !darwin

package installer

// trustVerifierBinary is a no-op on non-macOS platforms: only macOS
// Gatekeeper/AMFI re-validates downloaded binaries' code-signing trust on
// every exec, so there is nothing to strip or re-sign on Linux/Windows.
func trustVerifierBinary(path string) error {
	return nil
}
