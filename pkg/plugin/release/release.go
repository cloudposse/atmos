// Package release owns the build, test, package, and publish lifecycle.
package release

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

// Backend supplies the release operations. Test must execute only the current
// host's binary; Pack verifies the payloads for every declared platform.
//
//go:generate go run go.uber.org/mock/mockgen -source=release.go -destination=mock_backend_test.go -package=release
type Backend interface {
	Validate(context.Context, string) error
	Build(context.Context, string) error
	Test(context.Context, string) error
	Pack(context.Context, string, string) error
	Publish(context.Context, string) (*pluginoci.Release, error)
}

type settings struct{ temporaryDirectory string }

// Option configures local release staging.
type Option func(*settings)

// WithTemporaryDirectory selects the parent for temporary layouts.
func WithTemporaryDirectory(directory string) Option {
	defer perf.Track(nil, "pluginrelease.WithTemporaryDirectory")()

	return func(config *settings) { config.temporaryDirectory = directory }
}

// Run validates source, builds all targets, tests the host target, and publishes
// a verified layout. Temporary layouts are removed on success and failure.
func Run(ctx context.Context, directory string, backend Backend, options ...Option) (*pluginoci.Release, error) {
	defer perf.Track(nil, "pluginrelease.Run")()

	if backend == nil {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	config := settings{}
	for _, option := range options {
		option(&config)
	}
	operations := []struct {
		name string
		run  func(context.Context, string) error
	}{
		{"validate", backend.Validate}, {"build", backend.Build}, {"test", backend.Test},
	}
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := operation.run(ctx, directory); err != nil {
			return nil, fmt.Errorf("plugin release %s: %w", operation.name, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	temporary, err := os.MkdirTemp(config.temporaryDirectory, "atmos-plugin-release-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporary)
	layout := filepath.Join(temporary, "layout")
	if err := backend.Pack(ctx, directory, layout); err != nil {
		return nil, fmt.Errorf("plugin release pack: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return backend.Publish(ctx, layout)
}
