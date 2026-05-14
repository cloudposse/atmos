package client

import (
	"testing"

	"github.com/stretchr/testify/assert"

	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
)

func TestPrintTestResult_AllSuccess(t *testing.T) {
	result := &mcpclient.TestResult{
		ServerStarted: true,
		Initialized:   true,
		ToolCount:     5,
		PingOK:        true,
	}
	// Should not panic — printTestResult only calls ui.Success/Warning/Error.
	assert.NotPanics(t, func() {
		printTestResult(result)
	})
}

func TestPrintTestResult_FailedStart(t *testing.T) {
	result := &mcpclient.TestResult{
		ServerStarted: false,
	}
	assert.NotPanics(t, func() {
		printTestResult(result)
	})
}

func TestPrintTestResult_StartedButNoTools(t *testing.T) {
	result := &mcpclient.TestResult{
		ServerStarted: true,
		Initialized:   true,
		ToolCount:     0,
		PingOK:        false,
	}
	assert.NotPanics(t, func() {
		printTestResult(result)
	})
}
