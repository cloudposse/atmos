package filemanager

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
)

//go:generate go run go.uber.org/mock/mockgen@latest -source=interface.go -destination=mock_interface_test.go -package=filemanager

// FileManager manages a specific toolchain file type.
type FileManager interface {
	// Enabled returns true if this file manager is enabled by configuration.
	Enabled() bool

	// AddTool adds or updates a tool version.
	AddTool(ctx context.Context, tool, version string, opts ...AddOption) error

	// RemoveTool removes a tool version.
	RemoveTool(ctx context.Context, tool, version string) error

	// SetDefault sets a tool version as default.
	SetDefault(ctx context.Context, tool, version string) error

	// GetTools returns all tools managed by this file.
	GetTools(ctx context.Context) (map[string][]string, error)

	// Verify verifies the integrity of the managed file.
	Verify(ctx context.Context) error

	// Name returns the manager name for logging.
	Name() string
}

// AddOption configures tool addition behavior.
type AddOption func(*AddConfig)

// AddConfig holds configuration for adding tools.
type AddConfig struct {
	AsDefault bool
	Platform  string
	Checksum  string
	URL       string
	Size      int64
}

// WithAsDefault adds the tool as the default version.
func WithAsDefault() AddOption {
	defer perf.Track(nil, "filemanager.WithAsDefault")()

	return func(c *AddConfig) {
		c.AsDefault = true
	}
}

// WithPlatform specifies the platform for multi-platform lock files.
func WithPlatform(platform string) AddOption {
	defer perf.Track(nil, "filemanager.WithPlatform")()

	return func(c *AddConfig) {
		c.Platform = platform
	}
}

// WithChecksum adds checksum for lock file.
func WithChecksum(checksum string) AddOption {
	defer perf.Track(nil, "filemanager.WithChecksum")()

	return func(c *AddConfig) {
		c.Checksum = checksum
	}
}

// WithURL adds download URL for lock file.
func WithURL(url string) AddOption {
	defer perf.Track(nil, "filemanager.WithURL")()

	return func(c *AddConfig) {
		c.URL = url
	}
}

// WithSize adds file size for lock file.
func WithSize(size int64) AddOption {
	defer perf.Track(nil, "filemanager.WithSize")()

	return func(c *AddConfig) {
		c.Size = size
	}
}
