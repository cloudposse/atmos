package exec

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/schema"
)

// errCrossAccountAccessDenied is the failure a `!terraform.state` read produces when the
// component's backend lives in an account the current identity cannot reach — the exact
// shape reported in https://github.com/cloudposse/atmos/issues/2566.
var errCrossAccountAccessDenied = fmt.Errorf(
	"%w for component `global` in stack `dev-pen`: operation error S3: GetObject, "+
		"https response error StatusCode: 403, api error AccessDenied: Access Denied",
	errUtils.ErrReadTerraformState,
)

// installFailingStateGetter swaps the package-level stateGetter for a generated mock that
// always fails with the supplied error, standing in for an unreachable cross-account backend
// without any network or credentials. MinTimes(1) enforces that the read is actually
// attempted — the property that distinguishes degrading a failed read from skipping it.
func installFailingStateGetter(t *testing.T, err error) {
	t.Helper()

	ctrl := gomock.NewController(t)
	mock := NewMockTerraformStateGetter(ctrl)
	mock.EXPECT().
		GetState(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, err).
		MinTimes(1)

	original := stateGetter
	t.Cleanup(func() { stateGetter = original })
	stateGetter = mock
}

// TestProcessNodesWithContext_DegradesCrossAccountAccessDenied is the #2566 regression guard
// at the level the maintainers asked for: rather than skipping the read, a backend failure
// the current identity cannot avoid must degrade to the `(computed)` placeholder and let the
// walk finish, so an inventory listing still renders.
func TestProcessNodesWithContext_DegradesCrossAccountAccessDenied(t *testing.T) {
	installFailingStateGetter(t, errCrossAccountAccessDenied)

	var warnings []DegradationWarning
	data := map[string]any{
		"vars": map[string]any{
			"data_bucket_name": "!terraform.state global dev-pen data_bucket_name",
			"plain_value":      "untouched",
		},
	}

	result, err := processNodesWithContext(
		&schema.AtmosConfiguration{}, data, "dev-pen", nil, nil,
		&schema.ConfigAndStacksInfo{Component: "customer"},
		func(w DegradationWarning) { warnings = append(warnings, w) },
	)

	// The mock's MinTimes(1) already enforces that the read was attempted rather than
	// skipped — the property separating this fix from the skip-based approach it replaced.
	require.NoError(t, err, "a cross-account AccessDenied must not abort the whole walk in warn mode")

	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok, "vars section must survive the degradation")
	assert.Equal(t, degradation.AtmosComputedValue{}, vars["data_bucket_name"],
		"the unresolvable value must become the computed placeholder")
	assert.Equal(t, "(computed)", fmt.Sprintf("%v", vars["data_bucket_name"]),
		"the placeholder must render as `(computed)` in output")
	assert.Equal(t, "untouched", vars["plain_value"],
		"unrelated values must be unaffected")

	require.Len(t, warnings, 1, "the degradation must be reported exactly once")
	assert.Equal(t, "dev-pen", warnings[0].Stack)
	assert.Equal(t, "customer", warnings[0].Component)
	assert.Contains(t, warnings[0].Reason, "AccessDenied",
		"the warning must carry the real cause so --logs-level=Debug can explain it")
}

// TestProcessNodesWithContext_StrictModeStillFailsOnAccessDenied is the negative path:
// widening warn mode must not weaken `--error-mode=strict`, which is what a user selects
// precisely to see these failures.
func TestProcessNodesWithContext_StrictModeStillFailsOnAccessDenied(t *testing.T) {
	installFailingStateGetter(t, errCrossAccountAccessDenied)

	data := map[string]any{
		"vars": map[string]any{"data_bucket_name": "!terraform.state global dev-pen data_bucket_name"},
	}

	// A nil onWarning is what strict mode passes.
	_, err := processNodesWithContext(
		&schema.AtmosConfiguration{}, data, "dev-pen", nil, nil,
		&schema.ConfigAndStacksInfo{Component: "customer"}, nil,
	)

	require.Error(t, err, "strict mode must still surface a backend failure")
	assert.True(t, errors.Is(err, errUtils.ErrReadTerraformState),
		"the original cause must remain matchable by errors.Is")
}

// TestProcessNodesWithContext_FatalErrorsStillAbortInWarnMode proves the widening is scoped:
// a manifest defect (bad YQ expression against state Atmos did retrieve) must still fail the
// command even with a warn-mode callback installed, rather than silently becoming
// `(computed)` and hiding the mistake.
func TestProcessNodesWithContext_FatalErrorsStillAbortInWarnMode(t *testing.T) {
	installFailingStateGetter(t, fmt.Errorf("%w: .bad[", errUtils.ErrEvaluateTerraformBackendVariable))

	var warnings []DegradationWarning
	data := map[string]any{
		"vars": map[string]any{"data_bucket_name": "!terraform.state global dev-pen data_bucket_name"},
	}

	_, err := processNodesWithContext(
		&schema.AtmosConfiguration{}, data, "dev-pen", nil, nil,
		&schema.ConfigAndStacksInfo{Component: "customer"},
		func(w DegradationWarning) { warnings = append(warnings, w) },
	)

	require.Error(t, err, "a manifest defect must not be degraded away even in warn mode")
	assert.True(t, errors.Is(err, errUtils.ErrEvaluateTerraformBackendVariable))
	assert.Empty(t, warnings, "no degradation should be reported for a fatal error")
}

// TestProcessNodesWithContext_ManifestDefectsAbortInWarnMode drives the manifest-defect
// classification through the same path a real `!terraform.state` takes, using the shapes
// GetTerraformState actually produces.
//
// These all arrive wrapped in ErrReadTerraformState, indistinguishable from the cross-account
// AccessDenied above unless the cause survives the wrap. Degrading them would report success
// while emitting `(computed)` for what is really a typo in the stack manifest or a corrupt
// state file — the user would get a plausible-looking value and a zero exit code with no
// signal that anything was wrong.
func TestProcessNodesWithContext_ManifestDefectsAbortInWarnMode(t *testing.T) {
	tests := []struct {
		name     string
		readErr  error
		sentinel error
	}{
		{
			name: "typo'd backend_type",
			readErr: fmt.Errorf("%w for component `global` in stack `dev-pen`: %w: `s4`",
				errUtils.ErrReadTerraformState, errUtils.ErrUnsupportedBackendType),
			sentinel: errUtils.ErrUnsupportedBackendType,
		},
		{
			name: "corrupt state file",
			readErr: fmt.Errorf("%w for component `global` in stack `dev-pen`: %w",
				errUtils.ErrReadTerraformState, errUtils.ErrProcessTerraformStateFile),
			sentinel: errUtils.ErrProcessTerraformStateFile,
		},
		{
			name: "static backend missing the requested output",
			readErr: fmt.Errorf("%w: %w `data_bucket_name`",
				errUtils.ErrReadTerraformState, errUtils.ErrStaticRemoteStateOutputMissing),
			sentinel: errUtils.ErrStaticRemoteStateOutputMissing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installFailingStateGetter(t, tt.readErr)

			var warnings []DegradationWarning
			data := map[string]any{
				"vars": map[string]any{"data_bucket_name": "!terraform.state global dev-pen data_bucket_name"},
			}

			_, err := processNodesWithContext(
				&schema.AtmosConfiguration{}, data, "dev-pen", nil, nil,
				&schema.ConfigAndStacksInfo{Component: "customer"},
				func(w DegradationWarning) { warnings = append(warnings, w) },
			)

			require.Error(t, err, "a manifest defect must abort even though it is wrapped in ErrReadTerraformState")
			assert.True(t, errors.Is(err, tt.sentinel), "the specific cause must remain matchable")
			assert.True(t, errors.Is(err, errUtils.ErrReadTerraformState),
				"the outer wrapper must still be present — that is what makes the narrowing necessary")
			assert.Empty(t, warnings, "a fatal error must not also be reported as a degradation")
		})
	}
}
