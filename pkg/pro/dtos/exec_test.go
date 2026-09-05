package dtos

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time guard: ExecUploadRequest must have ExecutionID; the retired
// DataItems/BatchID/BatchIndex/BatchTotal fields must no longer exist (a
// struct literal referencing any of them would fail to compile) —
// research.md Decision 15/16.
var _ = ExecUploadRequest{ExecutionID: "x"}

func TestExecUploadRequest_MarshalsExecutionID(t *testing.T) {
	req := ExecUploadRequest{ExecutionID: "11111111-1111-4111-8111-111111111111", Command: "terraform plan"}

	b, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(b, &decoded))

	assert.Equal(t, "11111111-1111-4111-8111-111111111111", decoded["execution_id"])
	_, hasBatchID := decoded["batch_id"]
	assert.False(t, hasBatchID, "batch_id must no longer appear on ExecUploadRequest")
	_, hasDataItems := decoded["data_items"]
	assert.False(t, hasDataItems, "data_items must no longer appear on ExecUploadRequest")
}

func TestExecDataUploadRequest_MarshalsExecutionIDAndData(t *testing.T) {
	req := ExecDataUploadRequest{
		ExecutionID: "22222222-2222-4222-8222-222222222222",
		Data:        json.RawMessage(`{"foo":"bar"}`),
	}

	b, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(b, &decoded))

	assert.Equal(t, "22222222-2222-4222-8222-222222222222", decoded["execution_id"])
	dataMap, ok := decoded["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "bar", dataMap["foo"])
}

func TestExecDataUploadResponse_UnmarshalsSuccessAndURL(t *testing.T) {
	body := []byte(`{"success":true,"url":"https://blob.example/data.json"}`)

	var resp ExecDataUploadResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.True(t, resp.Success)
	assert.Equal(t, "https://blob.example/data.json", resp.URL)
}
