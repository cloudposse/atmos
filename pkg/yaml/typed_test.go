package yaml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLooksNonString(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{"bool true", "true", true},
		{"bool false", "false", true},
		{"bool mixed case", "True", true},
		{"integer", "42", true},
		{"negative integer", "-5", true},
		{"float", "3.14", true},
		{"scientific notation", "1e3", true},
		{"explicit positive integer", "+5", true},
		{"leading-dot float", ".5", true},
		{"trailing-dot float", "5.", true},
		{"region string", "us-east-1", false},
		{"plain word", "production", false},
		{"empty string", "", false},
		{"string containing a number", "v1.2.3", false},
		{"bare exponent letter", "e", false},
		// Regression: underscore-separated digits and "nan" are accepted by
		// strconv.ParseFloat today, so warning for them is safe and accurate.
		{"underscore-separated integer", "1_000", true},
		{"underscore-separated float", "1_000.5", true},
		{"nan lowercase", "nan", true},
		{"NaN mixed case", "NaN", true},
		// Regression: hex/octal/.inf are deliberately NOT warned about, since
		// strconv.ParseInt/ParseFloat at base 10 reject them -- warning would
		// send a user toward --type=int/float only to hit a parser error.
		{"hex literal", "0x1A", false},
		{"octal literal", "0o17", false},
		{"leading-dot inf", ".inf", false},
		{"negative leading-dot inf", "-.inf", false},
		// Regression (CodeRabbit finding on PR #2897): the fractional branch
		// must reject misplaced underscores -- a trailing underscore right
		// after the last fraction digit, or one immediately after the
		// decimal point with no digit before it -- since strconv.ParseFloat
		// rejects both. Only underscores strictly between two digits are a
		// valid Go/YAML digit separator.
		{"trailing underscore after fraction digit", "1.2_", false},
		{"underscore immediately after decimal point", "1._2", false},
		{"underscore between fraction digits", "1.2_3", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, LooksNonString(tt.raw))
		})
	}
}

// TestBuildValidatedRHS_IntCanonicalization is a regression test: a
// leading-zero decimal string like "010" is valid input to
// strconv.ParseInt(v, 10, 64) (parses as 10), but writing it verbatim and
// unquoted let yaml.v3's default resolver re-parse it as octal (8) on the
// next read -- a silent round-trip corruption. The canonical decimal form
// must be written instead.
func TestBuildValidatedRHS_IntCanonicalization(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"leading zero", "010", "10"},
		{"negative leading zero", "-010", "-10"},
		{"already canonical", "42", "42"},
		{"explicit plus sign", "+7", "7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildValidatedRHS(tt.value, TypeInt)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBuildValidatedRHS_IntOverflow is a regression test: an integer that
// overflows int64 (strconv.ErrRange) used to share the same "is not an
// integer" message as a genuine syntax error, which is misleading -- it is
// an integer, just out of range.
func TestBuildValidatedRHS_IntOverflow(t *testing.T) {
	_, err := buildValidatedRHS("99999999999999999999999", TypeInt)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidTypedValue)
	assert.Contains(t, err.Error(), "out of range")
}

// TestBuildValidatedRHS_FloatCanonicalization covers formatYAMLFloat's whole-
// number case: a finite whole-number float (e.g. 10) must keep an explicit
// decimal point so it round-trips with the !!float tag, not !!int.
func TestBuildValidatedRHS_FloatCanonicalization(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"whole number gets a decimal point", "10", "10.0"},
		{"ordinary decimal preserved", "3.14", "3.14"},
		{"negative whole number", "-5", "-5.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildValidatedRHS(tt.value, TypeFloat)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBuildValidatedRHS_FloatRejectsNaNAndInf is a regression test: Go's
// strconv.ParseFloat accepts bare "nan"/"inf"/"infinity" (case-insensitive,
// optionally signed), but YAML only recognizes the dotted spelling
// (".nan"/".inf"/"-.inf"). Writing ".nan" as a raw yq assignment RHS doesn't
// work either -- SetRaw builds "path = rhs" and evaluates rhs as a yq
// expression, so the leading "." is parsed as yq's path-navigation operator
// ("look up field nan"), not a literal scalar, silently writing null instead
// of NaN. Since there's no verified-safe way to write these through this
// code path, buildValidatedRHS must refuse instead of writing the wrong
// value.
func TestBuildValidatedRHS_FloatRejectsNaNAndInf(t *testing.T) {
	for _, value := range []string{"nan", "NaN", "inf", "Inf", "-inf", "+Inf", "Infinity"} {
		t.Run(value, func(t *testing.T) {
			_, err := buildValidatedRHS(value, TypeFloat)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidTypedValue)
		})
	}
}

// TestGuessNumericType is a regression test for the "auto nag" fix: --type=auto
// used to only warn that a brand-new value looked numeric, never actually
// infer int/float from it. GuessNumericType is the function that now does
// that inference, reusing buildValidatedRHS so a guess is guaranteed to
// write successfully.
func TestGuessNumericType(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		want   string
		wantOK bool
	}{
		{"plain int", "5", TypeInt, true},
		{"negative int", "-5", TypeInt, true},
		{"leading-zero int canonicalizes", "010", TypeInt, true},
		{"plain float", "5.5", TypeFloat, true},
		{"leading-dot float", ".5", TypeFloat, true},
		{"trailing-dot float", "5.", TypeFloat, true},
		{"scientific notation", "1e3", TypeFloat, true},
		// Go's strconv.ParseInt(v, 10, 64) rejects underscore digit
		// separators at base 10 (they require base 0), but
		// strconv.ParseFloat accepts them unconditionally -- so this lands
		// on TypeFloat, not TypeInt. Pre-existing buildValidatedRHS quirk,
		// documented here rather than "fixed", since GuessNumericType is
		// defined to reuse that exact validation.
		{"underscore-separated int lands on float", "1_000", TypeFloat, true},
		{"bare nan fails closed (rejected by buildValidatedRHS)", "nan", "", false},
		{"bare inf fails closed", "Infinity", "", false},
		{"not numeric at all", "hello", "", false},
		{"empty string", "", "", false},
		{"region-like string", "us-east-1", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := GuessNumericType(tt.raw)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestGuessScalarType is a regression test for the "auto nag" fix:
// GuessScalarType is what effectiveStackValueType/effectiveValueType now
// call as the last inference tier before falling back to string, so a
// brand-new key like `vars.replicas 5` infers TypeInt directly instead of
// warning and storing "5" as a literal string.
func TestGuessScalarType(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		want   string
		wantOK bool
	}{
		{"bool true", "true", TypeBool, true},
		{"bool false lowercase", "false", TypeBool, true},
		{"bool mixed case", "True", TypeBool, true},
		{"int", "5", TypeInt, true},
		{"float", "3.14", TypeFloat, true},
		// "1"/"0" must NOT be stolen by a strconv.ParseBool-style bool
		// check -- they're digits, so they belong to the int branch.
		{"digit one is int not bool", "1", TypeInt, true},
		{"digit zero is int not bool", "0", TypeInt, true},
		// The narrow case where LooksNonString says "looks numeric" but the
		// validated guess still fails closed: bare "nan", never a signed or
		// dotted form (those don't match LooksNonString to begin with).
		{"bare nan fails closed", "nan", "", false},
		{"NaN mixed case fails closed", "NaN", "", false},
		{"not non-string at all", "hello", "", false},
		{"region-like string", "us-east-1", "", false},
		{"hex literal (LooksNonString excludes it)", "0x1A", "", false},
		{"leading-dot inf (LooksNonString excludes it)", ".inf", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := GuessScalarType(tt.raw)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
