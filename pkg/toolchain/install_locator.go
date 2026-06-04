package toolchain

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_install_locator_test.go -package=toolchain . InstallLocator

// InstallLocator defines methods for locating installed tool binaries.
// This interface allows for mocking in tests and decouples path resolution from the concrete Installer type.
type InstallLocator interface {
	// ParseToolSpec parses a tool name into owner/repo pair.
	ParseToolSpec(tool string) (owner, repo string, err error)
	// FindBinaryPath finds the path to an installed binary for a given owner/repo/version.
	FindBinaryPath(owner, repo, version string, binaryName ...string) (string, error)
}
