package component

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func TestDiscoveryRejectsInvalidOwnershipIdentityAndProvenance(t *testing.T) {
	t.Parallel()
	p := fixtureProvider(t)
	unit := &pluginv1.DiscoveredComponent{
		Address: fixtureAddress(), Provider: p.name, Engine: "terraform", Source: "live/app.hcl",
		Sources:      []*pluginv1.ComponentSource{{Path: "root.hcl", Kind: pluginv1.ComponentSource_FILE}},
		Dependencies: []*pluginv1.ComponentDependency{{Target: fixtureAddress(), Kind: pluginv1.ComponentDependency_ORDER}},
	}
	require.NoError(t, p.checkDiscovery(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit}}))
	for name, change := range map[string]func(*pluginv1.DiscoveredComponent){
		"no address":         func(v *pluginv1.DiscoveredComponent) { v.Address = nil },
		"empty identity":     func(v *pluginv1.DiscoveredComponent) { v.Address.Component = "" },
		"different type":     func(v *pluginv1.DiscoveredComponent) { v.Address.ComponentType = "different" },
		"different provider": func(v *pluginv1.DiscoveredComponent) { v.Provider = "different" },
		"no engine":          func(v *pluginv1.DiscoveredComponent) { v.Engine = "" },
		"escaping source":    func(v *pluginv1.DiscoveredComponent) { v.Source = "../outside.hcl" },
		"escaping watch":     func(v *pluginv1.DiscoveredComponent) { v.Sources[0].Path = "../outside.hcl" },
		"unknown watch":      func(v *pluginv1.DiscoveredComponent) { v.Sources[0].Kind = pluginv1.ComponentSource_KIND_UNSPECIFIED },
		"nil watch":          func(v *pluginv1.DiscoveredComponent) { v.Sources[0] = nil },
		"unknown dependency": func(v *pluginv1.DiscoveredComponent) {
			v.Dependencies[0].Kind = pluginv1.ComponentDependency_KIND_UNSPECIFIED
		},
		"nil dependency": func(v *pluginv1.DiscoveredComponent) { v.Dependencies[0] = nil },
		"no target":      func(v *pluginv1.DiscoveredComponent) { v.Dependencies[0].Target = nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			item := proto.Clone(unit).(*pluginv1.DiscoveredComponent)
			change(item)
			require.ErrorIs(t, p.checkDiscovery(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{item}}), errUtils.ErrPluginProtocol)
		})
	}
	require.ErrorIs(t, p.checkDiscovery(nil), errUtils.ErrPluginProtocol)
	require.ErrorIs(t, p.checkDiscovery(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{nil}}), errUtils.ErrPluginProtocol)
	require.ErrorIs(t, p.checkDiscovery(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit, unit}}), errUtils.ErrDuplicatePluginExtension)
	// Diagnostics never invalidate the response; the discovery session interprets them per unit.
	require.NoError(t, p.checkDiscovery(&pluginv1.DiscoverComponentsResponse{Diagnostics: []*pluginv1.Diagnostic{{Message: "failure"}}}))
}

func TestPreparationRejectsUnsafeArtifactsAndMissingStateIdentity(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	valid := &pluginv1.PrepareComponentResponse{Engine: "terraform", WorkingDirectory: root, ModuleSource: "./module", State: fixtureState(root), Files: []*pluginv1.ComponentGeneratedFile{{Path: "backend.tf", IfExists: "error"}}}
	require.NoError(t, checkPreparation(valid))
	for name, change := range map[string]func(*pluginv1.PrepareComponentResponse){
		"no engine":         func(v *pluginv1.PrepareComponentResponse) { v.Engine = "" },
		"no module":         func(v *pluginv1.PrepareComponentResponse) { v.ModuleSource = "" },
		"no workdir":        func(v *pluginv1.PrepareComponentResponse) { v.WorkingDirectory = "relative" },
		"no state":          func(v *pluginv1.PrepareComponentResponse) { v.State = nil },
		"empty backend":     func(v *pluginv1.PrepareComponentResponse) { v.State.BackendType = "" },
		"empty workspace":   func(v *pluginv1.PrepareComponentResponse) { v.State.Workspace = "" },
		"relative backend":  func(v *pluginv1.PrepareComponentResponse) { v.State.WorkingDirectory = "relative" },
		"escaping artifact": func(v *pluginv1.PrepareComponentResponse) { v.Files[0].Path = "../outside.tf" },
		"absolute artifact": func(v *pluginv1.PrepareComponentResponse) { v.Files[0].Path = filepath.Join(root, "absolute.tf") },
		"unknown ownership": func(v *pluginv1.PrepareComponentResponse) { v.Files[0].IfExists = "unknown" },
		"nil artifact":      func(v *pluginv1.PrepareComponentResponse) { v.Files[0] = nil },
		"duplicate destination": func(v *pluginv1.PrepareComponentResponse) {
			v.Files = append(v.Files, &pluginv1.ComponentGeneratedFile{Path: filepath.Join("sub", "..", "backend.tf"), IfExists: "error"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			value := proto.Clone(valid).(*pluginv1.PrepareComponentResponse)
			change(value)
			require.ErrorIs(t, checkPreparation(value), errUtils.ErrPluginProtocol)
		})
	}
	require.ErrorIs(t, checkPreparation(nil), errUtils.ErrPluginProtocol)
	require.ErrorIs(t, checkPreparation(&pluginv1.PrepareComponentResponse{Diagnostics: []*pluginv1.Diagnostic{{Message: "failure"}}}), errUtils.ErrPluginInvalidConfig)
}
