package shell

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExecMetadataParserFromOpts_RoundTrips verifies WithExecMetadataParser
// threads a closure through ShellCommandOption and ExecMetadataParserFromOpts
// extracts the same closure back out, mirroring
// WithInvokingCommand/InvokingCommandFromOpts's existing shape (research.md
// Decision 18).
func TestExecMetadataParserFromOpts_RoundTrips(t *testing.T) {
	t.Parallel()

	called := false
	fn := func(subCommand string, exitCode int, output string) any {
		called = true
		return subCommand + "-data"
	}

	extracted := ExecMetadataParserFromOpts(WithExecMetadataParser(fn))
	require_ := assert.New(t)
	require_.NotNil(extracted)

	result := extracted("plan", 0, "")
	require_.True(called)
	require_.Equal("plan-data", result)
}

// TestExecMetadataParserFromOpts_NilWhenNotSet verifies the extractor returns
// nil when no WithExecMetadataParser option was passed.
func TestExecMetadataParserFromOpts_NilWhenNotSet(t *testing.T) {
	t.Parallel()

	assert.Nil(t, ExecMetadataParserFromOpts())
	assert.Nil(t, ExecMetadataParserFromOpts(WithInvokingCommand(nil)))
}
