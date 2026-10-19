package stack

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Blank imports register the built-in lifecycle handlers and command providers.
	_ "github.com/cloudposse/atmos/cmd/aws"
	_ "github.com/cloudposse/atmos/cmd/helm"
	_ "github.com/cloudposse/atmos/cmd/helmfile"
	"github.com/cloudposse/atmos/cmd/internal"
	_ "github.com/cloudposse/atmos/cmd/kubernetes"
	_ "github.com/cloudposse/atmos/cmd/terraform"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
)

// TestBuiltInLifecycleHandlersResolveToRealCommands guards against a handler
// that names a command which does not exist. Discovery silently drops such
// handlers, so the component kind would be skipped without any diagnostic.
func TestBuiltInLifecycleHandlersResolveToRealCommands(t *testing.T) {
	root := &cobra.Command{Use: "atmos"}
	require.NoError(t, internal.RegisterAll(root))

	for _, event := range []lifecycle.Event{lifecycle.Deploy, lifecycle.Destroy} {
		handlers := lifecycle.DefaultRegistry.Handlers(event)
		require.NotEmpty(t, handlers, "no built-in %s handlers registered", event)
		for kind, handler := range handlers {
			t.Run(string(event)+"/"+kind, func(t *testing.T) {
				found, rest, err := root.Find(handler.Path)
				require.NoError(t, err)
				assert.Empty(t, rest, "handler path %v has unresolved arguments", handler.Path)
				last := handler.Path[len(handler.Path)-1]
				assert.True(t, found.Name() == last || found.HasAlias(last), "%v resolved to %q", handler.Path, found.CommandPath())
				assert.True(t, found.Runnable(), "%v is not runnable", handler.Path)
			})
		}
	}
}

func TestHelmfileDeployMapsToApply(t *testing.T) {
	handler, ok := lifecycle.DefaultRegistry.Lookup("helmfile", lifecycle.Deploy)
	require.True(t, ok)
	assert.Equal(t, []string{"helmfile", "apply"}, handler.Path)
	assert.Equal(t, []string{"before.helmfile.apply", "after.helmfile.apply"}, handler.HookEvents)

	destroy, ok := lifecycle.DefaultRegistry.Lookup("helmfile", lifecycle.Destroy)
	require.True(t, ok)
	assert.Equal(t, []string{"helmfile", "destroy"}, destroy.Path)
	assert.Equal(t, []string{"before.helmfile.destroy", "after.helmfile.destroy"}, destroy.HookEvents)
}
