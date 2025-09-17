package parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSkipReasonComprehensive(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedReason string
	}{
		{
			name: "Skip reason BEFORE --- SKIP line (t.Skipf pattern)",
			input: `{"Time":"2025-09-15T18:18:10.902176-05:00","Action":"run","Package":"github.com/cloudposse/gotcha/test","Test":"TestSkipReason"}
{"Time":"2025-09-15T18:18:10.902178-05:00","Action":"output","Package":"github.com/cloudposse/gotcha/test","Test":"TestSkipReason","Output":"=== RUN   TestSkipReason\n"}
{"Time":"2025-09-15T18:18:10.902187-05:00","Action":"output","Package":"github.com/cloudposse/gotcha/test","Test":"TestSkipReason","Output":"    skip_test.go:20: Skipping test: example skip with formatted reason - test number 42\n"}
{"Time":"2025-09-15T18:18:10.902198-05:00","Action":"output","Package":"github.com/cloudposse/gotcha/test","Test":"TestSkipReason","Output":"--- SKIP: TestSkipReason (0.00s)\n"}
{"Time":"2025-09-15T18:18:10.902199-05:00","Action":"skip","Package":"github.com/cloudposse/gotcha/test","Test":"TestSkipReason","Elapsed":0}`,
			expectedReason: "Skipping test: example skip with formatted reason - test number 42",
		},
		{
			name: "Skip reason AFTER --- SKIP line (subtest pattern)",
			input: `{"Time":"2025-09-06T15:26:33.449262-06:00","Action":"run","Package":"github.com/cloudposse/atmos/internal/exec","Test":"TestCopyFile_FailChmod"}
{"Time":"2025-09-06T15:26:33.449323-06:00","Action":"output","Package":"github.com/cloudposse/atmos/internal/exec","Test":"TestCopyFile_FailChmod","Output":"--- SKIP: TestCopyFile_FailChmod (0.00s)\n"}
{"Time":"2025-09-06T15:26:33.449338-06:00","Action":"output","Package":"github.com/cloudposse/atmos/internal/exec","Test":"TestCopyFile_FailChmod","Output":"    vendor_utils_test.go:424: Skipping test on Windows: chmod simulation not supported\n"}
{"Time":"2025-09-06T15:26:33.449345-06:00","Action":"skip","Package":"github.com/cloudposse/atmos/internal/exec","Test":"TestCopyFile_FailChmod","Elapsed":0}`,
			expectedReason: "Skipping test on Windows: chmod simulation not supported",
		},
		{
			name: "Skip reason with multiple colons",
			input: `{"Time":"2025-09-15T18:18:10.902176-05:00","Action":"run","Package":"test/pkg","Test":"TestMultiColon"}
{"Time":"2025-09-15T18:18:10.902178-05:00","Action":"output","Package":"test/pkg","Test":"TestMultiColon","Output":"=== RUN   TestMultiColon\n"}
{"Time":"2025-09-15T18:18:10.902187-05:00","Action":"output","Package":"test/pkg","Test":"TestMultiColon","Output":"    test.go:10: Skipping: requirement not met: missing dependency: libfoo\n"}
{"Time":"2025-09-15T18:18:10.902198-05:00","Action":"output","Package":"test/pkg","Test":"TestMultiColon","Output":"--- SKIP: TestMultiColon (0.00s)\n"}
{"Time":"2025-09-15T18:18:10.902199-05:00","Action":"skip","Package":"test/pkg","Test":"TestMultiColon","Elapsed":0}`,
			expectedReason: "Skipping: requirement not met: missing dependency: libfoo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			summary, err := ParseTestJSON(reader, "", false)

			assert.NoError(t, err)
			assert.NotNil(t, summary)
			assert.Len(t, summary.Skipped, 1, "Should have exactly one skipped test")

			if len(summary.Skipped) > 0 {
				skip := summary.Skipped[0]
				assert.Equal(t, "skip", skip.Status)
				assert.NotEmpty(t, skip.SkipReason, "Skip reason should not be empty")
				assert.Equal(t, tt.expectedReason, skip.SkipReason, "Skip reason should match expected")
			}
		})
	}
}
