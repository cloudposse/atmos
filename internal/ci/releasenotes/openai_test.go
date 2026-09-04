package releasenotes

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// errReader simulates a connection dropping mid-read: a real failure mode
// for response bodies, distinct from a non-200 status or a transport error
// on Do itself.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, assert.AnError }
func (errReader) Close() error             { return nil }

func brokenBodyResponse(status int) *http.Response {
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: errReader{}}
}

func TestSummarize(t *testing.T) {
	entries := []PREntry{
		{Summary: "feat: add widget @alice (#101)", Number: 101, Body: "adds a widget"},
		{Summary: "fix: correct gizmo @bob (#102)", Number: 102, Body: "fixes a gizmo"},
	}

	ctrl := gomock.NewController(t)
	client := NewMockHTTPClient(ctrl)
	client.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "https://api.openai.com/v1/chat/completions", req.URL.String())
		assert.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"[{\"number\":101,\"summary\":\"Adds a widget.\"},{\"number\":102,\"summary\":\"Fixes a gizmo.\"}]"}}]}`), nil
	})

	got, err := Summarize(context.Background(), client, "test-key", "gpt-4o-mini", entries)
	require.NoError(t, err)
	assert.Equal(t, []string{"Adds a widget.", "Fixes a gizmo."}, got)
}

func TestSummarize_ModelWrapsResponseInProse(t *testing.T) {
	entries := []PREntry{{Summary: "feat: x @a (#1)", Number: 1, Body: "x"}}

	ctrl := gomock.NewController(t)
	client := NewMockHTTPClient(ctrl)
	client.EXPECT().Do(gomock.Any()).Return(
		jsonResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"Sure, here you go:\n[{\"number\":1,\"summary\":\"Does x.\"}]\nHope that helps!"}}]}`), //nolint:bodyclose // closed by the code under test, not this fixture.
		nil,
	)

	got, err := Summarize(context.Background(), client, "test-key", "gpt-4o-mini", entries)
	require.NoError(t, err)
	assert.Equal(t, []string{"Does x."}, got)
}

func TestSummarize_Errors(t *testing.T) {
	entries := []PREntry{{Summary: "feat: x @a (#1)", Number: 1, Body: "x"}}

	tests := []struct {
		name    string
		resp    *http.Response
		doErr   error
		wantErr string
	}{
		{
			name:    "transport error",
			doErr:   assert.AnError,
			wantErr: "call OpenAI",
		},
		{
			name:    "non-200 status",
			resp:    jsonResponse(http.StatusTooManyRequests, `{"error":"rate limited"}`), //nolint:bodyclose // closed by the code under test, not this fixture.
			wantErr: "OpenAI returned",
		},
		{
			name:    "response body read fails",
			resp:    brokenBodyResponse(http.StatusOK), //nolint:bodyclose // closed by the code under test, not this fixture.
			wantErr: "read OpenAI response",
		},
		{
			name:    "invalid response envelope",
			resp:    jsonResponse(http.StatusOK, `not json`), //nolint:bodyclose // closed by the code under test, not this fixture.
			wantErr: "decode OpenAI response",
		},
		{
			name:    "no choices",
			resp:    jsonResponse(http.StatusOK, `{"choices":[]}`), //nolint:bodyclose // closed by the code under test, not this fixture.
			wantErr: "no pull request entries",
		},
		{
			name:    "content is not valid JSON",
			resp:    jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"not an array"}}]}`), //nolint:bodyclose // closed by the code under test, not this fixture.
			wantErr: "decode model response",
		},
		{
			name:    "summary count mismatch",
			resp:    jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"[]"}}]}`), //nolint:bodyclose // closed by the code under test, not this fixture.
			wantErr: "summary count",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockHTTPClient(ctrl)
			client.EXPECT().Do(gomock.Any()).Return(tt.resp, tt.doErr)

			_, err := Summarize(context.Background(), client, "test-key", "gpt-4o-mini", entries)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestBuildPrompt_TruncatesLongBodies(t *testing.T) {
	longBody := strings.Repeat("x", maxEntryBodyChars+500)
	entries := []PREntry{{Summary: "feat: x @a (#1)", Number: 1, Body: longBody}}

	prompt := buildPrompt(entries)
	assert.LessOrEqual(t, len(prompt), maxEntryBodyChars+100)
	assert.Contains(t, prompt, "…")
}

func TestExtractJSONArray(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "clean array", input: `[{"a":1}]`, want: `[{"a":1}]`},
		{name: "wrapped in prose", input: "Here:\n[{\"a\":1}]\nDone.", want: `[{"a":1}]`},
		{name: "no array present", input: "no array here", want: "no array here"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractJSONArray(tt.input))
		})
	}
}
