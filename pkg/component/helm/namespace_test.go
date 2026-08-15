package helm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Issue #4: a chart whose manifests carry no metadata.namespace must install into the
// component's configured namespace, not the kubeconfig-default namespace. Helm derives the
// namespace for namespace-less objects from the settings/RESTClientGetter, so the action
// context must set the namespace on the settings (not only on the install action).
func TestNewActionContext_SetsSettingsNamespace(t *testing.T) {
	actx, err := newActionContext("echo-server-public")
	require.NoError(t, err)
	require.Equal(t, "echo-server-public", actx.settings.Namespace(),
		"newActionContext must set the namespace on the Helm settings so namespace-less manifests install into it")
}

// An empty namespace must leave the settings namespace at Helm's own default (no override).
func TestNewActionContext_EmptyNamespaceKeepsDefault(t *testing.T) {
	actx, err := newActionContext("")
	require.NoError(t, err)
	require.Equal(t, "default", actx.settings.Namespace())
}
