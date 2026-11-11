package auth

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIdentitySchemaTargets(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		target string
		kind   string
	}{
		{name: "default validates principal"},
		{name: "explicit principal", target: "principal"},
		{name: "plugin-defined spec", target: "spec", kind: "example/identity"},
		{name: "secret environment spec", target: "spec", kind: "secrets/env"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			principal := map[string]any{"subject": "principal"}
			spec := map[string]any{"env": map[string]any{"TOKEN": "{{ .credentials.token }}"}}
			definition := &Definition{SchemaTarget: test.target, Schema: func(value any) error {
				if test.target == "spec" {
					require.Equal(t, spec, value)
				} else {
					require.Equal(t, principal, value)
				}
				return nil
			}}
			i := &identity{definition: definition, config: &schema.Identity{Kind: test.kind, Via: &schema.IdentityVia{Provider: "test"}, Principal: principal, Spec: spec}}
			require.NoError(t, i.Validate())
		})
	}
}

func TestSecretEnvironmentLegacySyntaxMigration(t *testing.T) {
	t.Parallel()
	i := &identity{definition: &Definition{SchemaTarget: "spec"}, config: &schema.Identity{
		Kind: "secrets/env", Via: &schema.IdentityVia{Provider: "test"}, Principal: map[string]any{"environment": map[string]any{"TOKEN": "token"}},
	}}
	err := i.Validate()
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.Contains(t, errUtils.AllHints(err), "Use spec.env with explicit .credentials templates.")
}
