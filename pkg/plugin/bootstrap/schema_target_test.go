package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/auth"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIdentitySchemaTargetMetadataSelectsValidatedConfiguration(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"", "principal", "spec"} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			principal := map[string]any{"subject": "principal"}
			spec := map[string]any{"tenant": "spec"}
			metadata := map[string]any{"accepts": []any{"oidc/v1"}}
			if target != "" {
				metadata["schema_target"] = target
			}
			var validated any
			definition := &adapter.Definition{
				Outputs: []string{"environment/v1"},
				Schema:  func(value any) error { validated = value; return nil },
			}
			constructors := newConstructors(&adapter.Services{})
			require.NoError(t, constructors.identity(extension.Binding{Name: "example/env", Metadata: metadata}, definition))
			identity, err := constructors.Batch.Identities["example/env"]("session", &schema.Identity{
				Kind: "example/env", Via: &schema.IdentityVia{Provider: "credentials"}, Principal: principal, Spec: spec,
			})
			require.NoError(t, err)
			require.NoError(t, identity.Validate())
			if target == "spec" {
				require.Equal(t, spec, validated)
			} else {
				require.Equal(t, principal, validated)
			}
		})
	}
}

func TestIdentityRejectsInvalidSchemaTargetBeforeRegistration(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		value any
	}{
		{name: "unknown target", value: "credentials"},
		{name: "empty explicit target", value: ""},
		{name: "boolean", value: true},
		{name: "null"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			constructors := newConstructors(&adapter.Services{})
			binding := extension.Binding{Name: "example/env", Metadata: map[string]any{
				"accepts": []any{"oidc/v1"}, "schema_target": test.value,
			}}
			err := constructors.identity(binding, &adapter.Definition{Outputs: []string{"environment/v1"}})
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			require.Empty(t, constructors.Batch.Identities, "invalid metadata must never install a usable identity constructor")
		})
	}
}
