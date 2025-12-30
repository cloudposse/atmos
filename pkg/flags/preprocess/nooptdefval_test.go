package preprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockFlag implements FlagInfo for testing.
type mockFlag struct {
	name        string
	shorthand   string
	noOptDefVal string
}

func (f *mockFlag) GetName() string        { return f.name }
func (f *mockFlag) GetShorthand() string   { return f.shorthand }
func (f *mockFlag) GetNoOptDefVal() string { return f.noOptDefVal }

func TestNewNoOptDefValPreprocessor(t *testing.T) {
	t.Parallel()

	flags := []FlagInfo{&mockFlag{name: "test"}}
	preprocessor := NewNoOptDefValPreprocessor(flags)

	assert.NotNil(t, preprocessor)
	assert.Equal(t, flags, preprocessor.flags)
}

func TestNoOptDefValPreprocessor_Preprocess_NilFlags(t *testing.T) {
	t.Parallel()

	preprocessor := &NoOptDefValPreprocessor{flags: nil}
	args := []string{"--identity", "prod"}

	result := preprocessor.Preprocess(args)

	assert.Equal(t, args, result)
}

func TestNoOptDefValPreprocessor_Preprocess_NoNoOptDefValFlags(t *testing.T) {
	t.Parallel()

	flags := []FlagInfo{
		&mockFlag{name: "stack", shorthand: "s", noOptDefVal: ""},
	}
	preprocessor := NewNoOptDefValPreprocessor(flags)
	args := []string{"--stack", "dev"}

	result := preprocessor.Preprocess(args)

	assert.Equal(t, args, result)
}

func TestNoOptDefValPreprocessor_Preprocess_SpaceSeparated(t *testing.T) {
	t.Parallel()

	flags := []FlagInfo{
		&mockFlag{name: "identity", shorthand: "i", noOptDefVal: "__SELECT__"},
	}
	preprocessor := NewNoOptDefValPreprocessor(flags)
	args := []string{"--identity", "prod", "plan"}

	result := preprocessor.Preprocess(args)

	// Should rewrite to equals syntax.
	assert.Equal(t, []string{"--identity=prod", "plan"}, result)
}

func TestNoOptDefValPreprocessor_Preprocess_EqualsSyntaxUnchanged(t *testing.T) {
	t.Parallel()

	flags := []FlagInfo{
		&mockFlag{name: "identity", shorthand: "i", noOptDefVal: "__SELECT__"},
	}
	preprocessor := NewNoOptDefValPreprocessor(flags)
	args := []string{"--identity=prod", "plan"}

	result := preprocessor.Preprocess(args)

	// Should remain unchanged.
	assert.Equal(t, []string{"--identity=prod", "plan"}, result)
}

func TestNoOptDefValPreprocessor_Preprocess_PagerWithEquals(t *testing.T) {
	t.Parallel()

	// This is the regression test for the hasSeparatedValue bug.
	// Previously, --pager=more was incorrectly detected as NOT having equals syntax.
	flags := []FlagInfo{
		&mockFlag{name: "pager", shorthand: "", noOptDefVal: "true"},
	}
	preprocessor := NewNoOptDefValPreprocessor(flags)
	args := []string{"describe", "component", "--pager=more", "--format", "yaml"}

	result := preprocessor.Preprocess(args)

	// --pager=more should remain unchanged (not transformed).
	assert.Equal(t, []string{"describe", "component", "--pager=more", "--format", "yaml"}, result)
}

func TestNoOptDefValPreprocessor_Preprocess_Shorthand(t *testing.T) {
	t.Parallel()

	flags := []FlagInfo{
		&mockFlag{name: "identity", shorthand: "i", noOptDefVal: "__SELECT__"},
	}
	preprocessor := NewNoOptDefValPreprocessor(flags)
	args := []string{"-i", "prod", "plan"}

	result := preprocessor.Preprocess(args)

	// Should rewrite shorthand to equals syntax.
	assert.Equal(t, []string{"-i=prod", "plan"}, result)
}

func TestNoOptDefValPreprocessor_Preprocess_FlagWithoutValue(t *testing.T) {
	t.Parallel()

	flags := []FlagInfo{
		&mockFlag{name: "identity", shorthand: "i", noOptDefVal: "__SELECT__"},
	}
	preprocessor := NewNoOptDefValPreprocessor(flags)
	// --identity without a value (should use NoOptDefVal).
	args := []string{"--identity", "plan"}

	result := preprocessor.Preprocess(args)

	// "plan" looks like a value (not a flag), so it gets combined.
	assert.Equal(t, []string{"--identity=plan"}, result)
}

func TestNoOptDefValPreprocessor_Preprocess_FlagFollowedByFlag(t *testing.T) {
	t.Parallel()

	flags := []FlagInfo{
		&mockFlag{name: "identity", shorthand: "i", noOptDefVal: "__SELECT__"},
	}
	preprocessor := NewNoOptDefValPreprocessor(flags)
	// --identity followed by another flag (no value).
	args := []string{"--identity", "--verbose"}

	result := preprocessor.Preprocess(args)

	// Should not combine with --verbose (it's another flag).
	assert.Equal(t, []string{"--identity", "--verbose"}, result)
}

func TestNoOptDefValPreprocessor_Preprocess_FlagAtEnd(t *testing.T) {
	t.Parallel()

	flags := []FlagInfo{
		&mockFlag{name: "identity", shorthand: "i", noOptDefVal: "__SELECT__"},
	}
	preprocessor := NewNoOptDefValPreprocessor(flags)
	// --identity at end of args (no value).
	args := []string{"plan", "--identity"}

	result := preprocessor.Preprocess(args)

	// Should remain unchanged (no value to combine).
	assert.Equal(t, []string{"plan", "--identity"}, result)
}

func TestNoOptDefValPreprocessor_Preprocess_MultipleFlags(t *testing.T) {
	t.Parallel()

	flags := []FlagInfo{
		&mockFlag{name: "identity", shorthand: "i", noOptDefVal: "__SELECT__"},
		&mockFlag{name: "pager", shorthand: "", noOptDefVal: "true"},
	}
	preprocessor := NewNoOptDefValPreprocessor(flags)
	args := []string{"--identity", "prod", "--pager", "less", "plan"}

	result := preprocessor.Preprocess(args)

	// Both flags should be rewritten.
	assert.Equal(t, []string{"--identity=prod", "--pager=less", "plan"}, result)
}

func TestNoOptDefValPreprocessor_Preprocess_NonNoOptDefValFlagsUnchanged(t *testing.T) {
	t.Parallel()

	flags := []FlagInfo{
		&mockFlag{name: "identity", shorthand: "i", noOptDefVal: "__SELECT__"},
		&mockFlag{name: "stack", shorthand: "s", noOptDefVal: ""}, // No NoOptDefVal
	}
	preprocessor := NewNoOptDefValPreprocessor(flags)
	args := []string{"--stack", "dev", "--identity", "prod"}

	result := preprocessor.Preprocess(args)

	// --stack should remain space-separated, --identity should be rewritten.
	assert.Equal(t, []string{"--stack", "dev", "--identity=prod"}, result)
}

// Test helper functions.

func TestIsFlagArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		arg      string
		expected bool
	}{
		{"--flag", true},
		{"-f", true},
		{"value", false},
		{"", false},
		{"-", true}, // Edge case: single dash is still a flag-like arg.
	}

	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			t.Parallel()
			result := isFlagArg(tt.arg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasSeparatedValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		arg      string
		expected bool
	}{
		{"--flag=value", true},
		{"-f=value", true},
		{"--flag", false},
		{"-f", false},
		{"value", false},
		{"--pager=more", true}, // This was failing before the bug fix!
		{"--identity=prod", true},
		{"-i=prod", true},
	}

	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			t.Parallel()
			result := hasSeparatedValue(tt.arg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractFlagName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		arg      string
		expected string
	}{
		{"--flag", "flag"},
		{"-f", "f"},
		{"--identity", "identity"},
		{"-i", "i"},
		{"value", "value"}, // No dashes to strip.
	}

	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			t.Parallel()
			result := extractFlagName(tt.arg)
			assert.Equal(t, tt.expected, result)
		})
	}
}
