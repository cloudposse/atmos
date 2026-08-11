package proexec

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTruncateIfNeeded_UnderLimit_NoOp(t *testing.T) {
	req := &dtos.ExecUploadRequest{
		Command: "atmos version",
		Data:    json.RawMessage(`{"warnings":["a","b"]}`),
	}
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.MaxPayloadBytes = 4096

	out, err := truncateIfNeeded(req, atmosConfig)
	require.NoError(t, err)
	assert.JSONEq(t, `{"warnings":["a","b"]}`, string(out.Data))
}

func TestTruncateIfNeeded_TrimsLargeWarningsArray(t *testing.T) {
	warnings := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		warnings = append(warnings, strings.Repeat("w", 200))
	}
	dataBytes, err := json.Marshal(map[string]any{"warnings": warnings})
	require.NoError(t, err)

	req := &dtos.ExecUploadRequest{
		Command: "atmos terraform plan",
		Data:    dataBytes,
	}
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.MaxPayloadBytes = 2048 // small enough to force trimming

	out, err := truncateIfNeeded(req, atmosConfig)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(out.Data, &decoded))

	if _, isMarker := decoded["truncated"]; isMarker {
		// Even after trimming warnings, the record didn't fit — falling back
		// to the truncation marker is an acceptable outcome too.
		assert.Equal(t, true, decoded["truncated"])
		return
	}

	trimmedWarnings, ok := decoded["warnings"].([]any)
	require.True(t, ok)
	assert.LessOrEqual(t, len(trimmedWarnings), maxWarningsKept+1)
}

func TestTruncateIfNeeded_FallsBackToMarkerWhenUntrimmable(t *testing.T) {
	// A single oversized string field (not "warnings") can't be selectively
	// trimmed by the known-field heuristic, so it must fall back to the
	// truncation marker rather than being sent oversized or chunked.
	dataBytes, err := json.Marshal(map[string]any{"raw_log": strings.Repeat("x", 10000)})
	require.NoError(t, err)

	req := &dtos.ExecUploadRequest{
		Command: "atmos terraform apply",
		Data:    dataBytes,
	}
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.Pro.MaxPayloadBytes = 512

	out, err := truncateIfNeeded(req, atmosConfig)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(out.Data, &decoded))
	assert.Equal(t, true, decoded["truncated"])
}
