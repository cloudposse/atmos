package markdown

import (
	"bytes"
	"testing"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestWriteSkippedTestsTable_WithReasons(t *testing.T) {
	tests := []struct {
		name     string
		skipped  []types.TestResult
		expected []string
	}{
		{
			name: "test with skip reason",
			skipped: []types.TestResult{
				{
					Package:    "github.com/cloudposse/atmos/internal/exec",
					Test:       "TestCopyFile_FailChmod",
					Status:     "skip",
					SkipReason: "Skipping test on Windows: chmod simulation not supported",
				},
			},
			expected: []string{
				"⏭️ Skipped Tests (1)",
				"| Test | Package | Reason |",
				"| `TestCopyFile_FailChmod` | exec | Skipping test on Windows: chmod simulation not supported |",
			},
		},
		{
			name: "test without skip reason",
			skipped: []types.TestResult{
				{
					Package:    "github.com/cloudposse/atmos/internal/exec",
					Test:       "TestSomeOther",
					Status:     "skip",
					SkipReason: "",
				},
			},
			expected: []string{
				"⏭️ Skipped Tests (1)",
				"| Test | Package | Reason |",
				"| `TestSomeOther` | exec | _No reason provided_ |",
			},
		},
		{
			name: "mixed tests",
			skipped: []types.TestResult{
				{
					Package:    "github.com/cloudposse/atmos/internal/exec",
					Test:       "TestWithReason",
					Status:     "skip",
					SkipReason: "Platform specific test",
				},
				{
					Package:    "github.com/cloudposse/atmos/internal/exec",
					Test:       "TestWithoutReason",
					Status:     "skip",
					SkipReason: "",
				},
			},
			expected: []string{
				"⏭️ Skipped Tests (2)",
				"| `TestWithReason` | exec | Platform specific test |",
				"| `TestWithoutReason` | exec | _No reason provided_ |",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			WriteSkippedTestsTable(&buf, tt.skipped)
			output := buf.String()

			for _, exp := range tt.expected {
				assert.Contains(t, output, exp, "Expected to find: %s", exp)
			}

			// Debug output if test fails
			if t.Failed() {
				t.Logf("Full output:\n%s", output)
				// Also log the skip reasons from input
				for _, test := range tt.skipped {
					t.Logf("Test %s has SkipReason: %q", test.Test, test.SkipReason)
				}
			}
		})
	}
}
