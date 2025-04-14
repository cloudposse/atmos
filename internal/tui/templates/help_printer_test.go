package templates

import (
	"bytes"
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
