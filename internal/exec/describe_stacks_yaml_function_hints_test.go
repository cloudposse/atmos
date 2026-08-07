package exec

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// errStateRead stands in for the backend failure a cross-account `!terraform.state` read
// produces (an S3 403 AccessDenied wrapped by ErrReadTerraformState).
var errStateRead = fmt.Errorf(
	"%w for component `global` in stack `dev-pen`: operation error S3: GetObject, StatusCode: 403, api error AccessDenied",
	errUtils.ErrReadTerraformState,
)

// TestExplainRepositoryWideYAMLFunctionFailure_AddsHintsOnUnfilteredScan is the guard for the
// `describe stacks` half of https://github.com/cloudposse/atmos/issues/2566: an unfiltered
// describe walks every stack in every account, so a backend AccessDenied must tell the user how
// to scope or skip the read instead of surfacing a bare S3 error.
func TestExplainRepositoryWideYAMLFunctionFailure_AddsHintsOnUnfilteredScan(t *testing.T) {
	p := &describeStacksProcessor{filterByStack: ""}

	err := p.explainRepositoryWideYAMLFunctionFailure(errStateRead, "global", "dev-pen")

	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrReadTerraformState,
		"enriching the error must not break errors.Is on the original chain")
	assert.Contains(t, err.Error(), "AccessDenied", "the underlying cause must still be visible")

	hints := strings.Join(cockroachErrors.GetAllHints(err), "\n")
	assert.Contains(t, hints, "--error-mode=strict", "must point at warn mode, which degrades rather than fails")
	assert.Contains(t, hints, "--stack dev-pen", "must suggest scoping to the failing stack")
	assert.Contains(t, hints, "--skip terraform.state", "must suggest skipping the credential-backed function")
	assert.Contains(t, hints, "--process-functions=false", "must suggest disabling YAML function processing")
	assert.Contains(t, hints, "`global`", "must name the component that failed")
}

// TestExplainRepositoryWideYAMLFunctionFailure_NoHintsWhenScoped is the negative path: when the
// user already scoped the run with `--stack`, they asked for that stack specifically, so the
// underlying error is the whole answer and the scoping hints would be noise.
func TestExplainRepositoryWideYAMLFunctionFailure_NoHintsWhenScoped(t *testing.T) {
	p := &describeStacksProcessor{filterByStack: "dev-pen"}

	err := p.explainRepositoryWideYAMLFunctionFailure(errStateRead, "global", "dev-pen")

	require.Error(t, err)
	assert.Equal(t, errStateRead, err, "a scoped describe must return the error unchanged")
	assert.Empty(t, cockroachErrors.GetAllHints(err), "no hints should be attached to a scoped run")
}

// TestExplainRepositoryWideYAMLFunctionFailure_SkipCombinations covers how the skip list
// gates the hints. Only skipping BOTH terraform functions makes a pass credential-free —
// that is what an inventory `list` run looks like, and the only case that should suppress
// the `describe stacks` guidance. A partial skip still resolves the other function against
// a remote backend, so the guidance must survive; suppressing it there would drop the
// advice from exactly the case where a user is part-way through applying it.
func TestExplainRepositoryWideYAMLFunctionFailure_SkipCombinations(t *testing.T) {
	state := strings.TrimPrefix(u.AtmosYamlFuncTerraformState, "!")
	output := strings.TrimPrefix(u.AtmosYamlFuncTerraformOutput, "!")

	tests := []struct {
		name      string
		skip      []string
		wantHints bool
	}{
		{name: "nothing skipped", skip: nil, wantHints: true},
		{name: "only terraform.state skipped", skip: []string{state}, wantHints: true},
		{name: "only terraform.output skipped", skip: []string{output}, wantHints: true},
		{name: "unrelated function skipped", skip: []string{"exec"}, wantHints: true},
		{name: "both skipped (inventory list run)", skip: []string{state, output}, wantHints: false},
		{name: "both skipped among others", skip: []string{"exec", state, "store", output}, wantHints: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &describeStacksProcessor{filterByStack: "", skip: tt.skip}

			err := p.explainRepositoryWideYAMLFunctionFailure(errStateRead, "global", "dev-pen")

			require.Error(t, err)
			hints := cockroachErrors.GetAllHints(err)
			if tt.wantHints {
				assert.NotEmpty(t, hints, "a pass that still resolves a terraform function must keep the guidance")
				assert.Contains(t, strings.Join(hints, "\n"), "--skip terraform.state")
			} else {
				assert.Empty(t, hints, "a credential-free pass must get the error unchanged")
				assert.Equal(t, errStateRead, err)
			}
		})
	}
}

// TestExplainRepositoryWideYAMLFunctionFailure_NoHintsInWarnMode is the gate added after
// degradation landed: a non-nil onWarning means the caller is in warn/silent mode, where the
// lead hint ("drop --error-mode=strict") would be nonsense advice. Reaching here in warn mode
// means the error was non-recoverable — a manifest defect, not something an error mode fixes.
func TestExplainRepositoryWideYAMLFunctionFailure_NoHintsInWarnMode(t *testing.T) {
	p := &describeStacksProcessor{
		filterByStack: "",
		onWarning:     func(DegradationWarning) {},
	}

	err := p.explainRepositoryWideYAMLFunctionFailure(errStateRead, "global", "dev-pen")

	require.Error(t, err)
	assert.Equal(t, errStateRead, err, "warn mode must get the error back untouched")
	assert.Empty(t, cockroachErrors.GetAllHints(err))
}

// TestExplainRepositoryWideYAMLFunctionFailure_StackHintDoesNotOverpromise guards a wording
// correction: `--stack` narrows which components are evaluated, but a `!terraform.state`
// inside the selected stack can still name another stack and account, so the hint must not
// claim scoping avoids cross-account reads.
func TestExplainRepositoryWideYAMLFunctionFailure_StackHintDoesNotOverpromise(t *testing.T) {
	p := &describeStacksProcessor{filterByStack: ""}

	err := p.explainRepositoryWideYAMLFunctionFailure(errStateRead, "global", "dev-pen")

	hints := strings.Join(cockroachErrors.GetAllHints(err), "\n")
	require.Contains(t, hints, "--stack dev-pen")
	assert.Contains(t, hints, "only limits which components are evaluated",
		"the --stack hint must state its real effect")
}

// TestExplainRepositoryWideYAMLFunctionFailure_PassesThroughNil pins the success path: the
// helper sits on the hot path of every component, so a nil error must stay nil.
func TestExplainRepositoryWideYAMLFunctionFailure_PassesThroughNil(t *testing.T) {
	p := &describeStacksProcessor{filterByStack: ""}

	assert.NoError(t, p.explainRepositoryWideYAMLFunctionFailure(nil, "global", "dev-pen"))
}

// TestExplainRepositoryWideYAMLFunctionFailure_PreservesWrappedSentinels proves the enrichment is
// transparent to callers that branch on specific sentinels (e.g. the circular-dependency check
// that `!terraform.state` cycles rely on).
func TestExplainRepositoryWideYAMLFunctionFailure_PreservesWrappedSentinels(t *testing.T) {
	p := &describeStacksProcessor{filterByStack: ""}
	cause := fmt.Errorf("%w: %w", errUtils.ErrDescribeComponent, errUtils.ErrCircularDependency)

	err := p.explainRepositoryWideYAMLFunctionFailure(cause, "global", "dev-pen")

	require.True(t, errors.Is(err, errUtils.ErrDescribeComponent))
	require.True(t, errors.Is(err, errUtils.ErrCircularDependency))
}
