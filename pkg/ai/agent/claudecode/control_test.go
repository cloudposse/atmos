package claudecode

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeUserMessage(t *testing.T) {
	line, err := encodeUserMessage("say \"hi\"\nplease")
	require.NoError(t, err)
	assert.Equal(t, byte('\n'), line[len(line)-1])
	assert.Equal(t, 1, countNewlines(line), "must be a single line")

	var got map[string]any
	require.NoError(t, json.Unmarshal(line, &got))
	assert.Equal(t, map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": []any{map[string]any{"type": "text", "text": "say \"hi\"\nplease"}},
		},
	}, got)
}

func countNewlines(b []byte) int {
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n
}

func TestEncodeControlResponses(t *testing.T) {
	input := map[string]any{"command": "touch x", "n": float64(2)}
	tests := []struct {
		name string
		enc  func() ([]byte, error)
		want string
	}{
		{
			name: "allow echoes the input",
			enc:  func() ([]byte, error) { return encodeAllow("req-1", input) },
			want: `{"type":"control_response","response":{"subtype":"success","request_id":"req-1","response":{"behavior":"allow","updatedInput":{"command":"touch x","n":2}}}}`,
		},
		{
			name: "allow with nil input sends an empty object",
			enc:  func() ([]byte, error) { return encodeAllow("req-1", nil) },
			want: `{"type":"control_response","response":{"subtype":"success","request_id":"req-1","response":{"behavior":"allow","updatedInput":{}}}}`,
		},
		{
			name: "deny",
			enc:  func() ([]byte, error) { return encodeDeny("req-2", "nope", false) },
			want: `{"type":"control_response","response":{"subtype":"success","request_id":"req-2","response":{"behavior":"deny","message":"nope","interrupt":false}}}`,
		},
		{
			name: "deny with interrupt",
			enc:  func() ([]byte, error) { return encodeDeny("req-3", "stop", true) },
			want: `{"type":"control_response","response":{"subtype":"success","request_id":"req-3","response":{"behavior":"deny","message":"stop","interrupt":true}}}`,
		},
		{
			name: "deny defaults the message",
			enc:  func() ([]byte, error) { return encodeDeny("req-4", "", false) },
			want: `{"type":"control_response","response":{"subtype":"success","request_id":"req-4","response":{"behavior":"deny","message":"` + defaultDenyMessage + `","interrupt":false}}}`,
		},
		{
			name: "unsupported request",
			enc:  func() ([]byte, error) { return encodeControlError("req-5", "unsupported") },
			want: `{"type":"control_response","response":{"subtype":"error","request_id":"req-5","error":"unsupported"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, err := tt.enc()
			require.NoError(t, err)
			assert.Equal(t, byte('\n'), line[len(line)-1])
			assert.JSONEq(t, tt.want, string(line))
		})
	}
}
