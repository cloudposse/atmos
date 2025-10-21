package filesystem

import (
	"github.com/cloudposse/atmos/pkg/config/homedir"
)

// HomeDirProvider defines the interface for resolving home directories.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type HomeDirProvider interface {
	Dir() (string, error)
	Expand(path string) (string, error)
}

// OSHomeDirProvider is the default implementation using Atmos's vendored homedir package.
type OSHomeDirProvider struct{}

// NewOSHomeDirProvider creates a new OS homedir provider.
func NewOSHomeDirProvider() *OSHomeDirProvider {
	return &OSHomeDirProvider{}
}

// Dir returns the home directory for the current user.
func (h *OSHomeDirProvider) Dir() (string, error) {
	return homedir.Dir()
}

// Expand expands the path to include the home directory if the path begins with `~`.
func (h *OSHomeDirProvider) Expand(path string) (string, error) {
	return homedir.Expand(path)
}
