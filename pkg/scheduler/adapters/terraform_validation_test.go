package adapters

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateTerraformGraphMatchesExecution(t *testing.T) {
	for _, modern := range []bool{false, true} {
		for _, targetPresent := range []bool{false, true} {
			component := map[string]any{}
			entries := []any{map[string]any{"component": "database"}}
			if modern {
				component["dependencies"] = map[string]any{"components": entries}
			} else {
				component["settings"] = map[string]any{"depends_on": entries}
			}
			components := map[string]any{"app": component}
			if targetPresent {
				components["database"] = map[string]any{}
			}
			stacks := map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": components}}}
			_, executionErr := BuildTerraformGraph(stacks)
			validationErr := ValidateTerraformGraph(stacks, "")
			if executionErr == nil {
				require.NoError(t, validationErr)
			} else {
				require.EqualError(t, validationErr, executionErr.Error())
			}
			warnings := 0
			_, err := buildTerraformGraphWithWarnings(stacks, "", nil, nil, func(any, ...any) { warnings++ })
			if modern && !targetPresent {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if !modern && !targetPresent {
				require.Equal(t, 1, warnings)
			} else {
				require.Zero(t, warnings)
			}
		}
	}
}
