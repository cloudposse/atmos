package cloudformation

import (
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/require"
)

func TestMissingProducerCannotBecomeAnEmptyCloudFormationParameter(t *testing.T) {
	for _, parameters := range []any{
		map[string]any{"MissingOutput": nil},
		map[string]any{"List": []any{"valid", nil}},
		[]any{map[string]any{"ParameterKey": "MissingOutput", "ParameterValue": nil}},
		[]any{map[string]any{"ParameterKey": "MissingValue"}},
	} {
		_, err := buildStackSpec(map[string]any{"stack_name": "consumer", "path": "template.yaml", "parameters": parameters})
		require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationParameters)
	}
	// Intentional empty strings and explicit UsePreviousValue retain their meanings.
	_, err := normalizeParameters(map[string]any{"Empty": ""})
	require.NoError(t, err)
	_, err = normalizeParameters([]any{map[string]any{"ParameterKey": "Previous", "UsePreviousValue": true}})
	require.NoError(t, err)
}
