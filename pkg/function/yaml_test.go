package function

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
)

func TestEvaluateDeferredYAML(t *testing.T) {
	t.Cleanup(ci.SwapRegistryForTest())
	for _, tc := range []struct {
		input string
		skip  []string
		want  any
	}{
		{"!ci.labels", nil, []any{}},
		{"!ci.labels [preview]", nil, false},
		{"!ci.labels\t[preview]", nil, false},
		{"!ci.labels [preview]", []string{"ci.labels"}, "!ci.labels [preview]"},
		{"!ci.labels []", []string{"ci.labels"}, "!ci.labels []"},
		{"!ci.labels []", []string{"!ci.labels"}, "!ci.labels []"},
		{"!ci.labels[preview]", nil, "!ci.labels[preview]"},
		{"!ci.labels.other", nil, "!ci.labels.other"},
		{"!ci.labels-extra", nil, "!ci.labels-extra"},
		{"!CI.LABELS", nil, "!CI.LABELS"},
		{"!exec must-not-run", nil, "!exec must-not-run"},
		{"plain value", nil, "plain value"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := EvaluateDeferredYAML(context.Background(), tc.input, tc.skip, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
	_, err := EvaluateDeferredYAML(context.Background(), "!ci.labels []", nil, nil)
	require.ErrorIs(t, err, errUtils.ErrInvalidArguments)
}
