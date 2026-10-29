package native

import (
	"context"
	"maps"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/version"
)

// WithDescription binds the process handshake to statically verified metadata.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func WithDescription(expected *pluginv1.DescribeResponse) Option {
	return func(settings *options) { settings.expected = expected }
}

func (h *Handle) describe(ctx context.Context, expected *pluginv1.DescribeResponse) error {
	if expected == nil {
		return nil
	}
	actual, err := h.Info.Describe(ctx, &pluginv1.DescribeRequest{AtmosVersion: version.Version, HostSupported: expected.Services})
	if err != nil {
		return errUtils.ErrPluginProtocol
	}
	return CheckDescription(expected, actual)
}

// CheckDescription rejects binaries that disagree with their installed manifest.
func CheckDescription(expected, actual *pluginv1.DescribeResponse) error {
	defer perf.Track(nil, "native.CheckDescription")()

	if expected == nil || actual == nil || expected.PluginName != actual.PluginName || expected.PluginVersion != actual.PluginVersion || expected.ManifestDigest != actual.ManifestDigest {
		return errUtils.ErrPluginProtocol
	}
	want, err := exportSet(expected.Exports)
	if err != nil {
		return err
	}
	got, err := exportSet(actual.Exports)
	if err != nil {
		return err
	}
	if !maps.Equal(want, got) {
		return errUtils.ErrPluginProtocol
	}
	return checkServices(expected.Services, actual.Services)
}

func checkServices(expected, actual []*pluginv1.ServiceVersionRange) error {
	services, err := serviceSet(actual)
	if err != nil {
		return err
	}
	for _, service := range expected {
		provided := services[service.Service]
		if provided == nil || provided.Min > service.Max || provided.Max < service.Min {
			return errUtils.ErrPluginProtocol
		}
	}
	return nil
}

func exportSet(exports []*pluginv1.Export) (map[string]bool, error) {
	result := make(map[string]bool)
	for _, export := range exports {
		if export == nil || export.Service == "" || export.Kind == "" {
			return nil, errUtils.ErrPluginProtocol
		}
		key := export.Service + ":" + export.Kind
		if result[key] {
			return nil, errUtils.ErrPluginProtocol
		}
		result[key] = true
	}
	return result, nil
}

func serviceSet(actual []*pluginv1.ServiceVersionRange) (map[string]*pluginv1.ServiceVersionRange, error) {
	services := make(map[string]*pluginv1.ServiceVersionRange)
	for _, service := range actual {
		if service == nil || service.Service == "" || service.Min == 0 || service.Max < service.Min || services[service.Service] != nil {
			return nil, errUtils.ErrPluginProtocol
		}
		services[service.Service] = service
	}
	return services, nil
}
