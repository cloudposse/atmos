package templates

import (
	"bytes"
	"io"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestPrintHelpFlagPrinter_PrintHelpFlag(t *testing.T) {
	tests := []struct {
		name           string
		flag           *pflag.Flag
		expectedOutput string
	}{
		{
			name: "empty name flag (double dash)",
			flag: &pflag.Flag{
				Name:     "",
				Usage:    "separates flags from arguments",
				Value:    &boolValue{value: false},
				DefValue: "",
			},
			expectedOutput: "        --              separates flags from arguments\n\n",
		},
		{
			name: "help flag without default",
			flag: &pflag.Flag{
				Name:      "help",
				Shorthand: "h",
				Usage:     "show help information",
				Value:     &boolValue{value: false},
				DefValue:  "false",
			},
			expectedOutput: "    -h, --help          show help information\n\n",
		},
		{
			name: "string flag with default",
			flag: &pflag.Flag{
				Name:      "output",
				Shorthand: "o",
				Usage:     "output file path",
				Value:     &stringValue{value: "out.txt"},
				DefValue:  "out.txt",
			},
			expectedOutput: "    -o, --output string output file path (default out.txt)\n\n",
		},
		{
			name: "bool flag without type",
			flag: &pflag.Flag{
				Name:      "verbose",
				Shorthand: "v",
				Usage:     "enable verbose output",
				Value:     &boolValue{value: false},
				DefValue:  "false",
			},
			expectedOutput: "    -v, --verbose       enable verbose output (default false)\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup printer
			var buf bytes.Buffer
			printer := &HelpFlagPrinter{
				out:        &buf,
				wrapLimit:  80,
				maxFlagLen: 20, // Adjust based on longest flag name length
			}

			// Execute
			printer.PrintHelpFlag(tt.flag)

			// Verify
			assert.Equal(t, tt.expectedOutput, buf.String(), "Output should match expected format")
		})
	}
}

// Helper types for flag values (since pflag.Value interface needs to be implemented).
type stringValue struct {
	value string
}

func (s *stringValue) String() string   { return s.value }
func (s *stringValue) Set(string) error { return nil }
func (s *stringValue) Type() string     { return "string" }

type boolValue struct {
	value bool
}

func (b *boolValue) String() string   { return "false" }
func (b *boolValue) Set(string) error { return nil }
func (b *boolValue) Type() string     { return "bool" }

// assertErrorCase is a helper function to assert error cases in NewHelpFlagPrinter tests.
func assertErrorCase(t *testing.T, err error, printer *HelpFlagPrinter, expectedMsg string) {
	t.Helper()
	assert.Error(t, err)
	assert.Nil(t, printer)
	if expectedMsg != "" {
		assert.Contains(t, err.Error(), expectedMsg)
	}
}

// assertSuccessCase is a helper function to assert success cases in NewHelpFlagPrinter tests.
func assertSuccessCase(t *testing.T, err error, printer *HelpFlagPrinter, wrapLimit uint) {
	t.Helper()
	assert.NoError(t, err)
	assert.NotNil(t, printer)
	if wrapLimit < minWidth {
		assert.Equal(t, uint(minWidth), printer.wrapLimit)
	} else {
		assert.Equal(t, wrapLimit, printer.wrapLimit)
	}
}

func TestNewHelpFlagPrinter(t *testing.T) {
	tests := []struct {
		name        string
		setupOut    func() io.Writer
		wrapLimit   uint
		flags       *pflag.FlagSet
		expectError bool
		expectedMsg string
	}{
		{
			name: "valid printer with standard width",
			setupOut: func() io.Writer {
				return &bytes.Buffer{}
			},
			wrapLimit: 120,
			flags:     pflag.NewFlagSet("test", pflag.ContinueOnError),
		},
		{
			name: "nil output writer",
			setupOut: func() io.Writer {
				return nil
			},
			wrapLimit:   80,
			flags:       pflag.NewFlagSet("test", pflag.ContinueOnError),
			expectError: true,
			expectedMsg: "invalid argument: output writer cannot be nil",
		},
		{
			name: "nil flag set",
			setupOut: func() io.Writer {
				return &bytes.Buffer{}
			},
			wrapLimit:   80,
			flags:       nil,
			expectError: true,
			expectedMsg: "invalid argument: flag set cannot be nil",
		},
		{
			name: "below minimum width uses default",
			setupOut: func() io.Writer {
				return &bytes.Buffer{}
			},
			wrapLimit: 50, // Below minWidth (80)
			flags:     pflag.NewFlagSet("test", pflag.ContinueOnError),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := tt.setupOut()
			printer, err := NewHelpFlagPrinter(out, tt.wrapLimit, tt.flags)

			if tt.expectError {
				assertErrorCase(t, err, printer, tt.expectedMsg)
				return
			}

			assertSuccessCase(t, err, printer, tt.wrapLimit)
		})
	}
}

func TestCalculateMaxFlagLength(t *testing.T) {
	tests := []struct {
		name           string
		setupFlags     func(*pflag.FlagSet)
		expectedMaxLen int
		description    string
	}{
		{
			name: "empty flag set",
			setupFlags: func(fs *pflag.FlagSet) {
				// No flags added.
			},
			expectedMaxLen: 0,
			description:    "should return 0 for empty flag set",
		},
		{
			name: "single bool flag with shorthand",
			setupFlags: func(fs *pflag.FlagSet) {
				fs.BoolP("verbose", "v", false, "enable verbose output")
			},
			expectedMaxLen: len("    -v, --verbose"),
			description:    "bool flag with shorthand",
		},
		{
			name: "string flag with shorthand",
			setupFlags: func(fs *pflag.FlagSet) {
				fs.StringP("output", "o", "", "output file")
			},
			expectedMaxLen: len("    -o, --output string"),
			description:    "string flag with shorthand and type",
		},
		{
			name: "bool flag without shorthand",
			setupFlags: func(fs *pflag.FlagSet) {
				fs.Bool("debug", false, "enable debug mode")
			},
			expectedMaxLen: len("        --debug"),
			description:    "bool flag without shorthand",
		},
		{
			name: "string flag without shorthand",
			setupFlags: func(fs *pflag.FlagSet) {
				fs.String("config", "", "config file path")
			},
			expectedMaxLen: len("        --config string"),
			description:    "string flag without shorthand",
		},
		{
			name: "mixed flags returns longest",
			setupFlags: func(fs *pflag.FlagSet) {
				fs.BoolP("verbose", "v", false, "enable verbose")
				fs.StringP("configuration-file", "c", "", "config file path")
			},
			expectedMaxLen: len("    -c, --configuration-file string"),
			description:    "should return length of longest flag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			tt.setupFlags(fs)

			maxLen := calculateMaxFlagLength(fs)
			assert.Equal(t, tt.expectedMaxLen, maxLen, tt.description)
		})
	}
}

func TestPrintHelpFlag_EmptyLinesAfterFirstLineRemoval(t *testing.T) {
	// This test validates behavior for empty/whitespace/newline usage values.
	// It should not panic and should still print the flag row.
	tests := []struct {
		name       string
		flag       *pflag.Flag
		wrapLimit  uint
		maxFlagLen int
	}{
		{
			name: "empty usage results in single empty line after split",
			flag: &pflag.Flag{
				Name:      "test",
				Shorthand: "t",
				Usage:     "",
				Value:     &boolValue{value: false},
				DefValue:  "",
			},
			wrapLimit:  80,
			maxFlagLen: 20,
		},
		{
			name: "whitespace only usage",
			flag: &pflag.Flag{
				Name:      "whitespace",
				Shorthand: "w",
				Usage:     "   ",
				Value:     &boolValue{value: false},
				DefValue:  "",
			},
			wrapLimit:  80,
			maxFlagLen: 20,
		},
		{
			name: "newline only usage",
			flag: &pflag.Flag{
				Name:      "newline",
				Shorthand: "n",
				Usage:     "\n",
				Value:     &boolValue{value: false},
				DefValue:  "",
			},
			wrapLimit:  80,
			maxFlagLen: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := &HelpFlagPrinter{
				out:        &buf,
				wrapLimit:  tt.wrapLimit,
				maxFlagLen: tt.maxFlagLen,
			}

			// This should not panic due to the empty lines check.
			assert.NotPanics(t, func() {
				printer.PrintHelpFlag(tt.flag)
			}, "PrintHelpFlag should not panic with empty or minimal content")

			// Verify trailing newline is always written.
			output := buf.String()
			assert.True(t, len(output) > 0, "output should not be empty")
			assert.True(t, output[len(output)-1] == '\n', "output should end with newline")
		})
	}
}

func TestPrintHelpFlag_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		flag        *pflag.Flag
		wrapLimit   uint
		maxFlagLen  int
		description string
	}{
		{
			name: "flag without shorthand string type",
			flag: &pflag.Flag{
				Name:     "config",
				Usage:    "configuration file path",
				Value:    &stringValue{value: "config.yaml"},
				DefValue: "config.yaml",
			},
			wrapLimit:   80,
			maxFlagLen:  25,
			description: "should format flag without shorthand with type",
		},
		{
			name: "flag without shorthand bool type",
			flag: &pflag.Flag{
				Name:     "debug",
				Usage:    "enable debug mode",
				Value:    &boolValue{value: false},
				DefValue: "false",
			},
			wrapLimit:   80,
			maxFlagLen:  25,
			description: "should format bool flag without shorthand",
		},
		{
			name: "narrow width triggers multi-line layout",
			flag: &pflag.Flag{
				Name:      "very-long-flag-name",
				Shorthand: "l",
				Usage:     "this is a long description that should wrap",
				Value:     &stringValue{value: "default"},
				DefValue:  "default",
			},
			wrapLimit:   60,
			maxFlagLen:  50,
			description: "should handle narrow width gracefully",
		},
		{
			name: "empty description after markdown rendering",
			flag: &pflag.Flag{
				Name:      "empty",
				Shorthand: "e",
				Usage:     "",
				Value:     &boolValue{value: false},
				DefValue:  "",
			},
			wrapLimit:   80,
			maxFlagLen:  20,
			description: "should handle empty lines after first line removal without panic",
		},
		{
			name: "single character description",
			flag: &pflag.Flag{
				Name:      "single",
				Shorthand: "s",
				Usage:     "x",
				Value:     &boolValue{value: false},
				DefValue:  "",
			},
			wrapLimit:   80,
			maxFlagLen:  20,
			description: "should handle single character description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := &HelpFlagPrinter{
				out:        &buf,
				wrapLimit:  tt.wrapLimit,
				maxFlagLen: tt.maxFlagLen,
			}

			printer.PrintHelpFlag(tt.flag)

			// Verify output was written.
			assert.NotEmpty(t, buf.String(), "output should not be empty")
		})
	}
}
