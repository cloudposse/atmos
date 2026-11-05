package starlark

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/script"
)

// swapDefaultLogger captures the process-wide default logger at Warn level for the test.
func swapDefaultLogger(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := log.Default()
	log.SetDefault(newTestLogger(&buf, log.WarnLevel))
	t.Cleanup(func() { log.SetDefault(previous) })
	return &buf
}

// TestMissingCIReporterWarns proves a host that supplies no CI reporter is visible in the logs,
// so a wiring regression (every gate silently reading off) cannot go unnoticed again.
func TestMissingCIReporterWarns(t *testing.T) {
	// Not parallel: swaps the process-wide default logger.
	logs := swapDefaultLogger(t)

	_, err := New().Execute(context.Background(), script.Spec{Name: "s", Source: `x = 1`})
	require.NoError(t, err)
	assert.Contains(t, logs.String(), "script host supplied no CI reporter; CI gates read as off")
}

// TestSuppliedCIReporterDoesNotWarn is the negative path: a host that wires a reporter is silent.
func TestSuppliedCIReporterDoesNotWarn(t *testing.T) {
	// Not parallel: swaps the process-wide default logger.
	logs := swapDefaultLogger(t)

	_, err := New().Execute(context.Background(), script.Spec{Name: "s", Source: `x = 1`, CI: newCIMock(t)})
	require.NoError(t, err)
	assert.NotContains(t, logs.String(), "no CI reporter")
}
