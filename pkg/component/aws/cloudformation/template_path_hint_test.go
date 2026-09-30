package cloudformation

import (
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/require"
)

func TestValidateComponentConfig_TemplateFileHint(t *testing.T) {
	for _, path := range []string{"template.yaml", "templates/stack.yml", "stack.JSON", " stack.template "} {
		t.Run(path, func(t *testing.T) {
			err := validateComponentConfig(map[string]any{"stack_name": "vpc", "template": path})
			require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
			require.Contains(t, strings.Join(cockroachErrors.GetAllHints(err), "\n"), "Use path:")
		})
	}
	for _, template := range []any{"Description: stack.yaml", "bad\nstack.yaml", "bad\rstack.yaml", "not a file", map[string]any{"Description": "hello"}} {
		err := validateComponentConfig(map[string]any{"stack_name": "vpc", "template": template})
		require.Error(t, err)
		require.NotContains(t, strings.Join(cockroachErrors.GetAllHints(err), "\n"), "Use path:")
	}
	require.NoError(t, validateComponentConfig(map[string]any{"stack_name": "vpc", "template": "Resources: {}"}))
}
