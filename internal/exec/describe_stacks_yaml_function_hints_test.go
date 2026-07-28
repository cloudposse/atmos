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

// TestExplainRepositoryWideYAMLFunctionFailure_NoHintsWhenStateReadsAlreadySkipped is the second
// negative path: the inventory `list` commands share this processor but always skip the terraform
// state/output functions, so they must never receive `atmos describe stacks …` advice.
func TestExplainRepositoryWideYAMLFunctionFailure_NoHintsWhenStateReadsAlreadySkipped(t *testing.T) {
	p := &describeStacksProcessor{
		filterByStack: "",
		skip: []string{
			strings.TrimPrefix(u.AtmosYamlFuncTerraformState, "!"),
			strings.TrimPrefix(u.AtmosYamlFuncTerraformOutput, "!"),
		},
	}

	err := p.explainRepositoryWideYAMLFunctionFailure(errStateRead, "global", "dev-pen")

	require.Error(t, err)
	assert.Equal(t, errStateRead, err, "a caller that already skips state reads must get the error unchanged")
	assert.Empty(t, cockroachErrors.GetAllHints(err))
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
