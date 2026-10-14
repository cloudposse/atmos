package starlark

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestNumericBuiltins(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, expression, want string }{
		{"sum integers", "sum([1, 2, 3])", "6"},
		{"sum empty", "sum([])", "0"},
		{"sum start", "sum([1, 2], 10)", "13"},
		{"sum keyword start", "sum([1, 2], start=10)", "13"},
		{"sum keyword iterable", "sum(iterable=[1, 2])", "3"},
		{"sum float start", "sum([], 1.5)", "1.5"},
		{"sum mixed", "sum([1, 2.5, -1])", "2.5"},
		{"sum tuple", "sum((1, 2, 3))", "6"},
		{"sum range", "sum(range(100))", "4950"},
		{"sum set", "sum(set([1, 2, 2]))", "3"},
		{"sum dictionary keys", "sum({1: 'a', 2: 'b'})", "3"},
		{"sum arbitrary precision", "sum([1 << 100, 1])", "1267650600228229401496703205377"},
		{"round integer", "round(7)", "7"},
		{"round down", "round(1.4)", "1"},
		{"round up", "round(1.6)", "2"},
		{"round ties", "[round(0.5), round(1.5), round(2.5), round(3.5)]", "[0,2,2,4]"},
		{"round negative ties", "[round(-0.5), round(-1.5), round(-2.5)]", "[0,-2,-2]"},
		{"round fractional", "round(3.14159, 2)", "3.14"},
		{"round exact ties", "[round(1.25, 1), round(1.75, 1)]", "[1.2,1.8]"},
		{"round binary near tie", "round(2.675, 2)", "2.67"},
		{"round keyword", "round(number=3.14159, ndigits=2)", "3.14"},
		{"round None digits", "round(2.5, None)", "2"},
		{"round negative places", "round(1234.5, -2)", "1200"},
		{"round integer ties", "[round(15, -1), round(25, -1), round(-15, -1), round(-25, -1)]", "[20,20,-20,-20]"},
		{"round integer positive places", "round(1234, 2)", "1234"},
		{"round large integer", "round(123456789012345678901234567895, -1)", "123456789012345678901234567900"},
		{"round large float to integer", "round(1e20)", "100000000000000000000"},
		{"round preserves types", "[type(round(2.5)), type(round(2.5, 0)), type(round(2, 0))]", `["int","float","int"]`},
		{"round negative zero", "str(round(-0.1, 0))", "-0.0"},
		{"round subnormal", "round(5e-324, 323)", "0"},
		{"round tiny positive places", "round(1.23456e-300, 303)", "1.235e-300"},
		{"round extreme places", "[round(1.5, 1 << 100), round(1.5, -(1 << 100)), round(7, -(1 << 100)), round(7, 1 << 100)]", "[1.5,0,0,7]"},
		{"round bounded places", "[round(1.5, 324), round(1.5, -309), round(7, -1000)]", "[1.5,0,0]"},
		{"round nonfinite with places", "[str(round(float('inf'), 2)), str(round(float('nan'), 2))]", `["+inf","nan"]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := runSource(t, "output = "+tc.expression)
			require.NoError(t, err)
			switch tc.name {
			case "round negative zero", "sum arbitrary precision", "round large integer", "round large float to integer":
				assert.Equal(t, tc.want, result.Value)
			default:
				assert.JSONEq(t, tc.want, result.Value)
			}
		})
	}
}

func TestNumericBuiltinErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ source, want string }{
		{"sum()", "sum:"},
		{"sum(3)", "iterable"},
		{"sum([1, '2'])", "items must be int or float"},
		{"sum([True])", "items must be int or float"},
		{"sum([], '')", "start must be an int or float"},
		{"sum([], [])", "start must be an int or float"},
		{"sum([], unknown=1)", "unexpected keyword"},
		{"round()", "round:"},
		{"round('1')", "number must be an int or float"},
		{"round(True)", "number must be an int or float"},
		{"round(1.2, 0.5)", "ndigits must be an int or None"},
		{"round(1.2, True)", "ndigits must be an int or None"},
		{"round(1, unknown=1)", "unexpected keyword"},
		{"round(float('inf'))", "non-finite"},
		{"round(float('nan'))", "non-finite"},
		{"round(1.79e308, -308)", "exceeds the float range"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			t.Parallel()
			_, err := runSource(t, tc.source)
			require.ErrorIs(t, err, errUtils.ErrStarlark)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestNumericBuiltinsInLoadedAndParallelFunctions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "numbers.star"), []byte("def total():\n    return round(sum([1.25, 2.25]))\n"), 0o600))
	result, err := New().Execute(t.Context(), script.Spec{
		WorkingDirectory: dir,
		Source:           "load(\"numbers.star\", \"total\")\noutput = steps.parallel(functions=[total, total])",
	})
	require.NoError(t, err)
	assert.JSONEq(t, "[4,4]", result.Value)
}

func TestNumericBuiltinsDryRun(t *testing.T) {
	t.Parallel()
	_, err := New().Execute(t.Context(), script.Spec{DryRun: true, Source: "round(sum(['invalid']))"})
	require.NoError(t, err)
}

func TestSumCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	_, err := New().Execute(ctx, script.Spec{Source: "sum(range(1000000000))"})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
