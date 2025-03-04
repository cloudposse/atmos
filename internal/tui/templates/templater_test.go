package templates

import (
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestWrappedFlagUsages_DoubleDashAtEnd(t *testing.T) {
	// Create a new FlagSet with various flags
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

	// Add regular flags
	fs.StringP("input", "i", "default.txt", "input file path")
	fs.BoolP("quiet", "q", false, "suppress output")
	// Add double dash flag
	fs.StringP("", "", "", "separates flags from arguments")

	// Execute the function
	output := WrappedFlagUsages(fs)

	// Split the output into individual flag entries (assuming double newline separation)
	entries := strings.Split(strings.TrimSpace(output), "\n\n")
	assert.Greater(t, len(entries), 1, "should have multiple flag entries")

	// Find the last non-empty entry
	var lastEntry string
	for i := len(entries) - 1; i >= 0; i-- {
		if strings.TrimSpace(entries[i]) != "" {
			lastEntry = entries[i]
			break
		}
	}
	assert.NotEmpty(t, lastEntry, "should have a non-empty last entry")

	// Verify the double dash is in the last entry
	assert.Contains(t, lastEntry, "-- ", "last entry should contain double dash")
	assert.Contains(t, lastEntry, "separates flags from arguments",
		"last entry should contain double dash usage")

	// Verify other flags appear before the double dash
	inputIndex := -1
	quietIndex := -1
	doubleDashIndex := -1

	for i, entry := range entries {
		if strings.Contains(entry, "--input") {
			inputIndex = i
		}
		if strings.Contains(entry, "--quiet") {
			quietIndex = i
		}
		if strings.Contains(entry, "-- ") {
			doubleDashIndex = i
		}
	}

	assert.Greater(t, doubleDashIndex, inputIndex,
		"double dash should appear after input flag")
	assert.Greater(t, doubleDashIndex, quietIndex,
		"double dash should appear after quiet flag")
	assert.Equal(t, len(entries)-1, doubleDashIndex,
		"double dash should be in the last entry")
}
