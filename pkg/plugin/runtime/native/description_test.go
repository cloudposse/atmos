package native_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/plugin/runtime/native"
)

func TestDescriptionBindsProcessToManifest(t *testing.T) {
	expected := &pluginv1.DescribeResponse{PluginName: "example/oidc", PluginVersion: "0.1.0", ManifestDigest: "sha256:fixture", Services: []*pluginv1.ServiceVersionRange{{Service: "auth.provider", Min: 1, Max: 1}}, Exports: []*pluginv1.Export{{Service: "auth.provider", Kind: "example/oidc"}}}
	require.NoError(t, native.CheckDescription(expected, proto.Clone(expected).(*pluginv1.DescribeResponse)))
	for name, modify := range map[string]func(*pluginv1.DescribeResponse){
		"name":     func(value *pluginv1.DescribeResponse) { value.PluginName = "other/plugin" },
		"version":  func(value *pluginv1.DescribeResponse) { value.PluginVersion = "0.2.0" },
		"manifest": func(value *pluginv1.DescribeResponse) { value.ManifestDigest = "changed" },
		"extra export": func(value *pluginv1.DescribeResponse) {
			value.Exports = append(value.Exports, &pluginv1.Export{Service: "command", Kind: "undeclared"})
		},
		"missing export":      func(value *pluginv1.DescribeResponse) { value.Exports = nil },
		"duplicate export":    func(value *pluginv1.DescribeResponse) { value.Exports = append(value.Exports, value.Exports[0]) },
		"unsupported version": func(value *pluginv1.DescribeResponse) { value.Services[0].Min, value.Services[0].Max = 2, 2 },
		"missing service":     func(value *pluginv1.DescribeResponse) { value.Services = nil },
	} {
		t.Run(name, func(t *testing.T) {
			actual := proto.Clone(expected).(*pluginv1.DescribeResponse)
			modify(actual)
			require.ErrorIs(t, native.CheckDescription(expected, actual), errUtils.ErrPluginProtocol)
		})
	}
}
