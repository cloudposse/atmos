package yaml

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
