package env

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func testBoolFlags(name string) bool {
	switch name {
	case "no-color", "logs-color", "v":
		return true
	default:
		return false
	}
}

func TestIsBoolLiteral(t *testing.T) {
	for value, want := range map[string]bool{
		"true": true, "false": true, "TRUE": true, "False": true,
		"1": false, "0": false, "t": false, "f": false, "yes": false, "": false, "version": false, "trues": false,
	} {
		assert.Equal(t, want, IsBoolLiteral(value), "value %q", value)
	}
}

func TestNormalizeBoolFlagValues(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want []string
	}{
		{"false", []string{"--logs-color", "false", "version"}, []string{"--logs-color=false", "version"}},
		{"true", []string{"--no-color", "true", "version"}, []string{"--no-color=true", "version"}},
		{"upper true", []string{"--no-color", "TRUE"}, []string{"--no-color=TRUE"}},
		{"mixed false", []string{"--logs-color", "False", "version"}, []string{"--logs-color=False", "version"}},
		{"shorthand", []string{"-v", "false", "version"}, []string{"-v=false", "version"}},
		{"multiple", []string{"--no-color", "false", "--logs-color", "true", "version"}, []string{"--no-color=false", "--logs-color=true", "version"}},
		{"after leading args", []string{"terraform", "plan", "vpc", "--logs-color", "false", "-s", "dev"}, []string{"terraform", "plan", "vpc", "--logs-color=false", "-s", "dev"}},
		{"subcommand untouched", []string{"--no-color", "version"}, []string{"--no-color", "version"}},
		{"component untouched", []string{"--logs-color", "mycomponent"}, []string{"--logs-color", "mycomponent"}},
		{"numeric one untouched", []string{"--no-color", "1"}, []string{"--no-color", "1"}},
		{"numeric zero untouched", []string{"--no-color", "0"}, []string{"--no-color", "0"}},
		{"t untouched", []string{"--no-color", "t"}, []string{"--no-color", "t"}},
		{"f untouched", []string{"--no-color", "f"}, []string{"--no-color", "f"}},
		{"flag at end", []string{"version", "--no-color"}, []string{"version", "--no-color"}},
		{"flag before flag", []string{"--no-color", "--logs-color", "false"}, []string{"--no-color", "--logs-color=false"}},
		{"equals untouched", []string{"--no-color=false", "version"}, []string{"--no-color=false", "version"}},
		{"unknown flag untouched", []string{"--unknown", "false", "version"}, []string{"--unknown", "false", "version"}},
		{"single dash long name untouched", []string{"-no-color", "false"}, []string{"-no-color", "false"}},
		{"long form of shorthand untouched", []string{"--v", "false"}, []string{"--v", "false"}},
		{"after separator untouched", []string{"version", "--", "--no-color", "false"}, []string{"version", "--", "--no-color", "false"}},
		{"rewrite then separator", []string{"--no-color", "false", "--", "--logs-color", "false"}, []string{"--no-color=false", "--", "--logs-color", "false"}},
		{"flag directly before separator", []string{"--no-color", "--", "false"}, []string{"--no-color", "--", "false"}},
		{"empty", []string{}, []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := append([]string{}, tt.args...)
			got := NormalizeBoolFlagValues(input, testBoolFlags)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.args, input, "input must not be mutated")
		})
	}

	t.Run("nil predicate returns args", func(t *testing.T) {
		args := []string{"--no-color", "false"}
		assert.Equal(t, args, NormalizeBoolFlagValues(args, nil))
	})
}

func TestColorOptionsFromArgs_SpaceSeparatedBoolValues(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want ColorOptions
	}{
		{"logs-color false", []string{"--logs-color", "false", "version"}, ColorOptions{LogsColor: "false"}},
		{"logs-color true", []string{"--logs-color", "TRUE", "version"}, ColorOptions{LogsColor: "true"}},
		{"no-color false", []string{"--no-color", "false", "version"}, ColorOptions{NoColorSet: true}},
		{"no-color true", []string{"--no-color", "true", "version"}, ColorOptions{NoColor: true, NoColorSet: true}},
		{"no-color before command", []string{"--no-color", "version"}, ColorOptions{NoColor: true, NoColorSet: true}},
		{"logs-color before command", []string{"--logs-color", "version"}, ColorOptions{LogsColor: "true"}},
		{"after separator", []string{"version", "--", "--logs-color", "false"}, ColorOptions{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", "")
			t.Setenv("ATMOS_NO_COLOR", "")
			t.Setenv("ATMOS_LOGS_COLOR", "")
			assert.Equal(t, tt.want, ColorOptionsFromArgs(tt.args))
		})
	}
}
