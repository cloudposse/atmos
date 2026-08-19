package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExecMetadataParserFromOpts_RoundTrips verifies WithExecMetadataParser
// threads a closure through ShellCommandOption and execMetadataParserFromOpts
// extracts the same closure back out, mirroring
// WithInvokingCommand/invokingCommandFromOpts's existing shape (research.md
// Decision 18).
func TestExecMetadataParserFromOpts_RoundTrips(t *testing.T) {
	called := false
	fn := func(subCommand string) any {
		called = true
		return subCommand + "-data"
	}

	extracted := execMetadataParserFromOpts(WithExecMetadataParser(fn))
	require_ := assert.New(t)
	require_.NotNil(extracted)

	result := extracted("plan")
	require_.True(called)
	require_.Equal("plan-data", result)
}

// TestExecMetadataParserFromOpts_NilWhenNotSet verifies the extractor returns
// nil when no WithExecMetadataParser option was passed.
func TestExecMetadataParserFromOpts_NilWhenNotSet(t *testing.T) {
	assert.Nil(t, execMetadataParserFromOpts())
	assert.Nil(t, execMetadataParserFromOpts(WithInvokingCommand(nil)))
}
