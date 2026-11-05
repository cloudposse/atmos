package conformance

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

func TestDetailErrorKeepsIdentityAndCarriesEvidence(t *testing.T) {
	cause := errors.New("cause")
	err := withDetail(cause, "exit code differs: expected %d, actual %d", 0, 3)
	require.ErrorIs(t, err, cause)
	assert.EqualError(t, err, "cause", "the detail never changes the error message")
	assert.Equal(t, "exit code differs: expected 0, actual 3", detailOf(err))
	assert.Equal(t, "exit code differs: expected 0, actual 3", detailOf(errors.Join(errUtils.ErrTestsFailed, err)))
	assert.Empty(t, detailOf(cause))
	assert.Empty(t, detailOf(nil))
	require.NoError(t, withDetail(nil, "ignored"))
}

func TestClipTruncatesOnRuneBoundariesAndReportsTheRemainder(t *testing.T) {
	short := "plain"
	assert.Equal(t, short, clip(short))
	long := strings.Repeat("é", maxDetailValue) // Two bytes per rune.
	clipped := clip(long)
	assert.True(t, utf8.ValidString(clipped), "a multibyte rune must not be split")
	assert.LessOrEqual(t, len(clipped), maxDetailValue+64)
	assert.Contains(t, clipped, "truncated")
	assert.Contains(t, clipped, "more bytes")
}

func TestClipAppliesRepositorySecretMasking(t *testing.T) {
	const secret = "conformance-detail-secret-value-7f3a"
	iolib.RegisterSecret(secret)
	masked := clip("token=" + secret)
	assert.NotContains(t, masked, secret)
}

func TestCommandMismatchNamesEveryDifferingFixtureField(t *testing.T) {
	fixture := &CommandFixture{Name: "about", Stdout: "expected out\n", Stderr: "", ExitCode: 0}
	detail := commandMismatch(fixture, "actual out\n", "boom\n", 2)
	assert.Contains(t, detail, "stdout does not match the fixture")
	assert.Contains(t, detail, "expected out")
	assert.Contains(t, detail, "actual out")
	assert.Contains(t, detail, "stderr does not match the fixture")
	assert.Contains(t, detail, "boom")
	assert.Contains(t, detail, "exit_code does not match the fixture: expected 0, actual 2")

	onlyStdout := commandMismatch(fixture, "different\n", "", 0)
	assert.Contains(t, onlyStdout, "stdout does not match the fixture")
	assert.NotContains(t, onlyStdout, "stderr")
	assert.NotContains(t, onlyStdout, "exit_code")
}

func TestResultDetailIsOmittedFromJSONWhenEmptyAndNamedWhenPresent(t *testing.T) {
	raw, err := json.Marshal(Result{Name: "a", Passed: true})
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"a","passed":true}`, string(raw))
	raw, err = json.Marshal(Result{Name: "b", Detail: "why"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"b","passed":false,"detail":"why"}`, string(raw))
}

func TestSafeFailureKeepsTheNoMatchSentinelButHidesPluginErrors(t *testing.T) {
	noMatch := errUtils.Build(errUtils.ErrPluginTestsNotMatched).WithHint("Available tests: a.").Err()
	require.ErrorIs(t, safeFailure(noMatch), errUtils.ErrPluginTestsNotMatched)
	require.NotErrorIs(t, safeFailure(errors.New("rpc error with credential")), errUtils.ErrPluginTestsNotMatched)
	assert.NotContains(t, safeFailure(errors.New("rpc error with credential")).Error(), "credential")
}
