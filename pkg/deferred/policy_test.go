package deferred

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=../merge/yaml_processor.go -destination=mock_yaml_processor_test.go -package=deferred

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/merge"
)

func TestValueProcessorRecoveryPolicy(t *testing.T) {
	for _, tc := range []struct {
		name        string
		err         error
		warn        bool
		wantWarning bool
	}{
		{name: "resolved sibling", warn: true},
		{name: "unavailable value", err: errUtils.ErrAuthenticationUnavailable, warn: true, wantWarning: true},
		{name: "strict value", err: errUtils.ErrAuthenticationUnavailable},
		{name: "invalid config stays fatal", err: errUtils.ErrInvalidAuthConfig, warn: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inner := NewMockYAMLFunctionProcessor(gomock.NewController(t))
			inner.EXPECT().ProcessYAMLFunctionString("!requested").Return("resolved", tc.err)
			processor := &ValueProcessor{Inner: inner, Recoverable: func(err error) bool { return errors.Is(err, errUtils.ErrAuthenticationUnavailable) }}
			warnings := 0
			if tc.warn {
				processor.Warn = func(value string, err error) {
					warnings++
					require.Equal(t, "!requested", value)
					require.ErrorIs(t, err, tc.err)
				}
			}
			value, err := processor.ProcessYAMLFunctionString("!requested")
			if tc.wantWarning {
				require.NoError(t, err)
				require.Equal(t, degradation.AtmosComputedValue{}, value)
				require.Equal(t, 1, warnings)
			} else {
				require.ErrorIs(t, err, tc.err)
				require.Equal(t, "resolved", value)
				require.Zero(t, warnings)
			}
		})
	}
}

func TestCanRecoverPreservesExistingPolicy(t *testing.T) {
	existing := func(err error) bool { return errors.Is(err, errUtils.ErrTerraformStateNotProvisioned) }
	for _, enabled := range []bool{false, true} {
		require.True(t, CanRecover(errUtils.ErrTerraformStateNotProvisioned, enabled, existing))
		require.False(t, CanRecover(errUtils.ErrInvalidAuthConfig, enabled, existing))
		require.Equal(t, enabled, CanRecover(errUtils.ErrAuthenticationUnavailable, enabled, existing))
	}
}

func TestExcludeComputedFieldsPreservesResolvableSiblings(t *testing.T) {
	dctx := merge.NewDeferredMergeContext()
	dctx.AddDeferred([]string{"vars", "computed"}, "!unavailable")
	dctx.AddDeferred([]string{"vars", "literal"}, "!resolvable")
	dctx.AddDeferred([]string{"vars", "literal", "child"}, "!resolvable")
	dctx.GetDeferredValues()["empty"] = nil
	section := map[string]any{"vars": map[string]any{"computed": degradation.AtmosComputedValue{}, "literal": "literal"}}
	ExcludeComputedFields(dctx, section)
	require.NotContains(t, dctx.GetDeferredValues(), "vars.computed")
	require.Contains(t, dctx.GetDeferredValues(), "vars.literal")
	require.Contains(t, dctx.GetDeferredValues(), "vars.literal.child")
	require.Contains(t, dctx.GetDeferredValues(), "empty")
}
