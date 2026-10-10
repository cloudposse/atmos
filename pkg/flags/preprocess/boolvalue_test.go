package preprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockBoolFlag is a FlagInfo that declares itself boolean.
type mockBoolFlag struct {
	mockFlag
	isBool bool
}

func (f *mockBoolFlag) IsBool() bool { return f.isBool }

func TestBoolValuePreprocessor_Preprocess(t *testing.T) {
	t.Parallel()

	flags := []FlagInfo{
		&mockBoolFlag{mockFlag: mockFlag{name: "logs-color"}, isBool: true},
		&mockBoolFlag{mockFlag: mockFlag{name: "verbose", shorthand: "v"}, isBool: true},
		&mockBoolFlag{mockFlag: mockFlag{name: "not-bool"}, isBool: false},
		// Plain FlagInfo without IsBool (string flag with NoOptDefVal) must be untouched.
		&mockFlag{name: "pager", noOptDefVal: "true"},
		&mockFlag{name: "stack", shorthand: "s"},
	}

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"false", []string{"--logs-color", "false", "version"}, []string{"--logs-color=false", "version"}},
		{"True", []string{"--logs-color", "True"}, []string{"--logs-color=True"}},
		{"shorthand", []string{"-v", "false", "version"}, []string{"-v=false", "version"}},
		{"long name of shorthand flag", []string{"--verbose", "false", "version"}, []string{"--verbose=false", "version"}},
		{"subcommand untouched", []string{"--logs-color", "version"}, []string{"--logs-color", "version"}},
		{"component untouched", []string{"--logs-color", "mycomponent"}, []string{"--logs-color", "mycomponent"}},
		{"numeric untouched", []string{"--logs-color", "1"}, []string{"--logs-color", "1"}},
		{"flag at end", []string{"version", "--logs-color"}, []string{"version", "--logs-color"}},
		{"flag then flag", []string{"--logs-color", "--verbose", "false"}, []string{"--logs-color", "--verbose=false"}},
		{"already equals", []string{"--logs-color=false", "version"}, []string{"--logs-color=false", "version"}},
		{"IsBool false untouched", []string{"--not-bool", "false"}, []string{"--not-bool", "false"}},
		{"NoOptDefVal string flag untouched", []string{"--pager", "false"}, []string{"--pager", "false"}},
		{"string flag untouched", []string{"--stack", "false"}, []string{"--stack", "false"}},
		{"unknown flag untouched", []string{"--unknown", "false"}, []string{"--unknown", "false"}},
		{"after separator untouched", []string{"--", "--logs-color", "false"}, []string{"--", "--logs-color", "false"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := append([]string{}, tt.args...)
			got := NewBoolValuePreprocessor(flags).Preprocess(input)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.args, input, "input must not be mutated")
		})
	}
}

func TestBoolValuePreprocessor_NoBoolFlags(t *testing.T) {
	t.Parallel()

	args := []string{"--logs-color", "false"}
	assert.Equal(t, args, NewBoolValuePreprocessor([]FlagInfo{&mockFlag{name: "stack"}}).Preprocess(args))
	assert.Equal(t, args, NewBoolValuePreprocessor(nil).Preprocess(args))
}

func TestPipeline_NoOptDefValThenBoolValue(t *testing.T) {
	t.Parallel()

	flags := []FlagInfo{
		&mockFlag{name: "identity", shorthand: "i", noOptDefVal: "__SELECT__"},
		&mockBoolFlag{mockFlag: mockFlag{name: "logs-color"}, isBool: true},
	}
	pipeline := NewPipeline(NewNoOptDefValPreprocessor(flags), NewBoolValuePreprocessor(flags))

	got := pipeline.Run([]string{"--identity", "prod", "--logs-color", "false", "version"})
	assert.Equal(t, []string{"--identity=prod", "--logs-color=false", "version"}, got)

	// A bare bool flag keeps the subcommand, even next to a NoOptDefVal flag.
	got = pipeline.Run([]string{"--logs-color", "version"})
	assert.Equal(t, []string{"--logs-color", "version"}, got)
}
