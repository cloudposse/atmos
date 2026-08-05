package infracost

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
)

// writeOutputFile creates an ATMOS_OUTPUT_FILE-style file with the given
// content and returns its path. Cleanup is automatic via t.TempDir.
func writeOutputFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "output")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestResultHandler_NoOutputFile(t *testing.T) {
	s, err := ResultHandler(&hooks.ExecContext{})
	require.NoError(t, err)
	assert.Nil(t, s)
}

func TestResultHandler_EmptyFile(t *testing.T) {
	ctx := &hooks.ExecContext{OutputFile: writeOutputFile(t, "")}
	s, err := ResultHandler(ctx)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, hooks.StatusSuccess, s.Status)
	assert.Equal(t, "no cost data", s.Title)
}

func TestResultHandler_PositiveCostDiff(t *testing.T) {
	body := `{
		"currency": "USD",
		"totalMonthlyCost": "47.20",
		"pastTotalMonthlyCost": "0",
		"diffTotalMonthlyCost": "47.20",
		"projects": [{
			"name": "vpc",
			"breakdown": {
				"resources": [
					{"name": "aws_nat_gateway.this", "resourceType": "aws_nat_gateway", "monthlyCost": "32.85"},
					{"name": "aws_eip.nat", "resourceType": "aws_eip", "monthlyCost": "3.65"}
				],
				"totalMonthlyCost": "36.50"
			}
		}]
	}`
	ctx := &hooks.ExecContext{OutputFile: writeOutputFile(t, body)}
	s, err := ResultHandler(ctx)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "infracost", s.Kind)
	assert.Equal(t, hooks.StatusWarning, s.Status)
	assert.Equal(t, "+$47.20/mo", s.Title)
	assert.Equal(t, 2, s.Counts["resources"])
	assert.Contains(t, s.Body, "infracost")
	assert.Contains(t, s.Body, "aws_nat_gateway.this")
	assert.Contains(t, s.Body, "$32.85")
	assert.Contains(t, s.Body, "+$47.20")
}

func TestResultHandler_ZeroCostDiff(t *testing.T) {
	body := `{
		"currency": "USD",
		"totalMonthlyCost": "0",
		"pastTotalMonthlyCost": "0",
		"diffTotalMonthlyCost": "0",
		"projects": [{"name": "vpc", "breakdown": {"resources": [], "totalMonthlyCost": "0"}}]
	}`
	ctx := &hooks.ExecContext{OutputFile: writeOutputFile(t, body)}
	s, err := ResultHandler(ctx)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, hooks.StatusSuccess, s.Status)
	assert.Equal(t, "no cost change", s.Title)
	assert.Contains(t, s.Body, "No priced resources")
}

func TestResultHandler_NegativeCostDiff(t *testing.T) {
	body := `{
		"currency": "USD",
		"totalMonthlyCost": "20.00",
		"pastTotalMonthlyCost": "100.00",
		"diffTotalMonthlyCost": "-80.00",
		"projects": [{"name": "vpc", "breakdown": {"resources": [{"name": "x", "resourceType": "t", "monthlyCost": "20"}]}}]
	}`
	ctx := &hooks.ExecContext{OutputFile: writeOutputFile(t, body)}
	s, err := ResultHandler(ctx)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "-$80.00/mo", s.Title)
	assert.Contains(t, s.Body, "-$80.00")
}

func TestResultHandler_DefaultsCurrencyToUSD(t *testing.T) {
	body := `{"totalMonthlyCost": "5.00", "diffTotalMonthlyCost": "5.00", "projects": []}`
	ctx := &hooks.ExecContext{OutputFile: writeOutputFile(t, body)}
	s, err := ResultHandler(ctx)
	require.NoError(t, err)
	assert.Equal(t, "+$5.00/mo", s.Title)
}

func TestResultHandler_NonUSDCurrency(t *testing.T) {
	body := `{
		"currency": "EUR",
		"totalMonthlyCost": "5.00",
		"diffTotalMonthlyCost": "5.00",
		"projects": []
	}`
	ctx := &hooks.ExecContext{OutputFile: writeOutputFile(t, body)}
	s, err := ResultHandler(ctx)
	require.NoError(t, err)
	assert.Equal(t, "+€5.00/mo", s.Title)
}

func TestResultHandler_InvalidJSON(t *testing.T) {
	ctx := &hooks.ExecContext{OutputFile: writeOutputFile(t, "{not-json")}
	_, err := ResultHandler(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrParseFile), "expected ErrParseFile, got %v", err)
}

func TestResultHandler_ReadErrorUsesStaticError(t *testing.T) {
	ctx := &hooks.ExecContext{OutputFile: filepath.Join(t.TempDir(), "missing.json")}
	_, err := ResultHandler(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrReadFile), "expected ErrReadFile, got %v", err)
}

func TestKindIsRegistered(t *testing.T) {
	k, ok := hooks.GetKind("infracost")
	require.True(t, ok, "infracost kind must self-register via init()")
	assert.Equal(t, "infracost", k.Command)
	assert.Equal(t, hooks.OnFailureWarn, k.OnFailure)
	assert.NotNil(t, k.ResultHandler)
	assert.Contains(t, k.DefaultArgs, "breakdown")
	assert.Contains(t, k.DefaultArgs, "$ATMOS_OUTPUT_FILE")
}
