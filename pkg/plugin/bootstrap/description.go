package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// DescribePackage derives the required runtime handshake from validated package metadata.
func DescribePackage(directory string, definition *manifest.Config, bindings []extension.Binding) (*pluginv1.DescribeResponse, error) {
	defer perf.Track(nil, "bootstrap.DescribePackage")()

	raw, err := manifest.ReadFile(directory, "plugin.yaml")
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	result := &pluginv1.DescribeResponse{PluginName: definition.Metadata.Name, PluginVersion: definition.Metadata.Version, ManifestDigest: "sha256:" + hex.EncodeToString(digest[:])}
	services := make(map[string]bool)
	for _, binding := range bindings {
		result.Exports = append(result.Exports, &pluginv1.Export{Service: binding.Capability, Kind: binding.Name})
		services[binding.Capability] = true
	}
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result.Services = append(result.Services, &pluginv1.ServiceVersionRange{Service: name, Min: 1, Max: 1})
	}
	return result, nil
}
