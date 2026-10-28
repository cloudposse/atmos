package install

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginpolicy "github.com/cloudposse/atmos/pkg/plugin/policy"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

// CheckPolicy reapplies current policy to verified cached bytes, never trusting
// metadata copied into a lockfile or a prior admission decision.
func (c *Cache) CheckPolicy(ctx context.Context, operation string, entry *pluginlock.Entry, platform string, descriptors *extension.Registry) error {
	defer perf.Track(nil, "plugininstall.Cache.CheckPolicy")()

	enabled, err := guard.Enabled("plugin")
	if err != nil || !enabled {
		return err
	}
	if err := pluginpolicy.Source(ctx, operation, entry.Source, pluginpolicy.WithCache()); err != nil {
		return err
	}
	directory, err := c.Installed(entry, platform)
	if err != nil {
		return err
	}
	definition, err := manifest.Load(directory)
	if err != nil {
		return err
	}
	return pluginpolicy.Package(ctx, operation, entry.Source, definition, pluginpolicy.WithDigest(entry.IndexDigest), pluginpolicy.WithCache(), pluginpolicy.WithExtensions(descriptors))
}

func (r *Request) policyOptions() []pluginpolicy.Option {
	if r.Layout != "" {
		return []pluginpolicy.Option{pluginpolicy.WithLayout()}
	}
	return nil
}
