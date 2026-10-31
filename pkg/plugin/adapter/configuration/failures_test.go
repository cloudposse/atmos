package configuration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

func discoverWith(t *testing.T, response *pluginv1.DiscoverComponentsResponse) (*Session, map[string]map[string]any, error) {
	t.Helper()
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(response, nil)
	session, err := New(t.Context(), enabledConfig(t), "plan", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	definitions, err := session.Discover(t.Context())
	return session, definitions, err
}

func failureDiagnostic(stack, name string) *pluginv1.Diagnostic {
	return &pluginv1.Diagnostic{Detail: &pluginv1.ErrorDetail{
		Code: "unsupported_setting", Summary: "Unsupported Terragrunt setting `terraform_binary`", Stack: stack, Unit: name, ConfigKey: "terraform_binary",
		Location: &pluginv1.SourceLocation{File: "live/" + name + "/terragrunt.hcl"}, Hints: []string{"Remove it."}, UserSafe: true, Operation: "discover",
	}}
}

// One failing unit leaves the healthy units usable and fails only the commands that target it.
func TestFailedUnitStaysVisibleAndFailsOnlyItsOwnCommands(t *testing.T) {
	session, definitions, err := discoverWith(t, &pluginv1.DiscoverComponentsResponse{
		Components:  []*pluginv1.DiscoveredComponent{unit(t, "good")},
		Diagnostics: []*pluginv1.Diagnostic{failureDiagnostic("live", "broken")},
	})
	require.NoError(t, err)
	components := definitions["live"]["components"].(map[string]any)["terraform"].(map[string]any)
	require.Contains(t, components, "good")
	require.Contains(t, components, "broken", "a failed unit stays listed so it can be fixed")
	configuration := components["broken"].(map[string]any)["settings"].(map[string]any)["configuration"].(map[string]any)
	assert.Equal(t, true, configuration["failed"])
	assert.Equal(t, "live/broken/terragrunt.hcl", configuration["source"])

	require.NoError(t, session.ComponentFailure(t.Context(), "live", "good"))
	require.NoError(t, session.ComponentFailure(t.Context(), "live", "unknown"))
	failure := session.ComponentFailure(t.Context(), "live", "broken")
	require.ErrorIs(t, failure, errUtils.ErrPluginInvalidConfig)
	assert.Equal(t, "Unsupported Terragrunt setting `terraform_binary`", failure.Error())

	_, err = session.Prepare(t.Context(), &schema.ConfigAndStacksInfo{Stack: "live", ComponentFromArg: "broken", Command: "tofu", SubCommand: "plan", ComponentSettingsSection: map[string]any{"configuration": true}})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	assert.Equal(t, failure.Error(), err.Error())
}

func TestWarningDiagnosticsNeverFailDiscovery(t *testing.T) {
	warning := &pluginv1.Diagnostic{Detail: &pluginv1.ErrorDetail{Code: "skipped", Summary: "Directory skipped", Severity: pluginv1.Severity_SEVERITY_WARNING, UserSafe: true}}
	session, definitions, err := discoverWith(t, &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit(t, "good")}, Diagnostics: []*pluginv1.Diagnostic{warning}})
	require.NoError(t, err)
	require.Contains(t, definitions, "live")
	require.NoError(t, session.ComponentFailure(t.Context(), "live", "good"))
}

func TestDiagnosticWithoutAUnitFailsTheProject(t *testing.T) {
	_, _, err := discoverWith(t, &pluginv1.DiscoverComponentsResponse{Diagnostics: []*pluginv1.Diagnostic{{Field: "discovery", Message: "legacy"}}})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

// A unit that depends on a failed or out-of-scope unit fails alone, and the failure names the dependency.
func TestDependentsOfFailedOrMissingUnitsFailToo(t *testing.T) {
	app, db, outside := unit(t, "app"), unit(t, "db"), unit(t, "lonely")
	app.Dependencies = []*pluginv1.ComponentDependency{{Target: &pluginv1.ComponentAddress{Stack: "live", Component: "broken", ComponentType: "terraform"}}}
	db.Dependencies = []*pluginv1.ComponentDependency{{Target: app.Address}}
	outside.Dependencies = []*pluginv1.ComponentDependency{{Target: &pluginv1.ComponentAddress{Stack: "other", Component: "network", ComponentType: "terraform"}}}
	session, _, err := discoverWith(t, &pluginv1.DiscoverComponentsResponse{
		Components:  []*pluginv1.DiscoveredComponent{app, db, outside, unit(t, "good")},
		Diagnostics: []*pluginv1.Diagnostic{failureDiagnostic("live", "broken")},
	})
	require.NoError(t, err)
	for _, name := range []string{"app", "db", "lonely", "broken"} {
		require.Error(t, session.ComponentFailure(t.Context(), "live", name), name)
	}
	require.NoError(t, session.ComponentFailure(t.Context(), "live", "good"))
	text := session.ComponentFailure(t.Context(), "live", "app").Error() + " " + session.ComponentFailure(t.Context(), "live", "lonely").Error()
	assert.Contains(t, text, "")
	require.NotEmpty(t, errUtils.AllHints(session.ComponentFailure(t.Context(), "live", "lonely")))
	require.Contains(t, errUtils.AllDetails(session.ComponentFailure(t.Context(), "live", "db"))[0], "depends on `live/app`")
}
