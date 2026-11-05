package manage

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/discovery"
)

// Search uses the same auth and policy transport as installation.
func (m *Manager) Search(ctx context.Context, request discovery.Request) ([]discovery.Result, error) {
	defer perf.Track(m.Config, "pluginmanage.Search")()

	service := discovery.Service{Config: m.Config, Registry: m.Installer.Registry, Descriptors: m.Installer.Descriptors}
	return service.Search(ctx, request)
}
