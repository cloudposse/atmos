package parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSkipReason(t *testing.T) {
	input := `{"Time":"2025-09-06T15:26:33.449262-06:00","Action":"run","Package":"github.com/cloudposse/atmos/internal/exec","Test":"TestCopyFile_FailChmod"}
{"Time":"2025-09-06T15:26:33.449323-06:00","Action":"output","Package":"github.com/cloudposse/atmos/internal/exec","Test":"TestCopyFile_FailChmod","Output":"--- SKIP: TestCopyFile_FailChmod (0.00s)\n"}
{"Time":"2025-09-06T15:26:33.449338-06:00","Action":"output","Package":"github.com/cloudposse/atmos/internal/exec","Test":"TestCopyFile_FailChmod","Output":"    vendor_utils_test.go:424: Skipping test on Windows: chmod simulation not supported\n"}
{"Time":"2025-09-06T15:26:33.449345-06:00","Action":"skip","Package":"github.com/cloudposse/atmos/internal/exec","Test":"TestCopyFile_FailChmod","Elapsed":0}`

	reader := strings.NewReader(input)
	summary, err := ParseTestJSON(reader, "", false)
	
	assert.NoError(t, err)
	assert.NotNil(t, summary)
	assert.Len(t, summary.Skipped, 1)
	
	if len(summary.Skipped) > 0 {
		skip := summary.Skipped[0]
		assert.Equal(t, "TestCopyFile_FailChmod", skip.Test)
		assert.Equal(t, "skip", skip.Status)
		// Check that the skip reason was captured
		assert.NotEmpty(t, skip.SkipReason, "Skip reason should not be empty")
		assert.Contains(t, skip.SkipReason, "Skipping test on Windows")
	}
}