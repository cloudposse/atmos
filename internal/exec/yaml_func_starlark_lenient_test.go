package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStarlarkYAMLLenientDependencies(t *testing.T) {
	getter := NewMockTerraformStateGetter(gomock.NewController(t))
	original := stateGetter
	stateGetter = getter
	t.Cleanup(func() { stateGetter = original })
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	getter.EXPECT().GetState(config, gomock.Any(), "dev", "vpc", "name", false, gomock.Any(), gomock.Any()).Return(nil, errUtils.ErrTerraformStateNotProvisioned).Times(2)
	input := map[string]any{"vars": map[string]any{
		"bucket":  `!terraform.state vpc dev name`,
		"derived": `!starlark return ctx.vars["bucket"] + "-name"`,
		"sibling": `!starlark return 3`,
	}}
	var warnings []DegradationWarning
	result, err := ProcessCustomYamlTagsLenient(config, input, "dev", nil, nil, func(w DegradationWarning) { warnings = append(warnings, w) })
	require.NoError(t, err)
	assert.Equal(t, degradation.AtmosComputedValue{}, result["vars"].(map[string]any)["derived"])
	assert.Equal(t, int64(3), result["vars"].(map[string]any)["sibling"])
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].Function, "!terraform.state")
	_, err = ProcessCustomYamlTags(config, input, "dev", nil, nil)
	require.ErrorIs(t, err, errUtils.ErrTerraformStateNotProvisioned)
}
