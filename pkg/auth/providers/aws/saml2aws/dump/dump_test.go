package dump

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failure") }

func TestContentEnable(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "true", want: true},
		{value: "TRUE", want: false},
		{value: "1", want: false},
		{value: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("DUMP_CONTENT", tt.value)
			assert.Equal(t, tt.want, ContentEnable())
		})
	}
}

func TestRequestString(t *testing.T) {
	tests := []struct {
		name     string
		dump     string
		wantBody bool
	}{
		{name: "headers and body", dump: "true", wantBody: true},
		{name: "headers only", dump: "false", wantBody: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DUMP_CONTENT", tt.dump)
			req, err := http.NewRequest(http.MethodPost, "https://idp.example.com/login", strings.NewReader("user=alice"))
			require.NoError(t, err)

			out := RequestString(req)
			assert.Contains(t, out, "POST /login HTTP/1.1")
			assert.Contains(t, out, "Host: idp.example.com")
			if tt.wantBody {
				assert.Contains(t, out, "user=alice")
			} else {
				assert.NotContains(t, out, "user=alice")
			}
		})
	}
}

func TestRequestStringError(t *testing.T) {
	t.Setenv("DUMP_CONTENT", "true")
	req, err := http.NewRequest(http.MethodGet, "ftp://idp.example.com/login", nil)
	require.NoError(t, err)

	assert.Empty(t, RequestString(req))
}

func TestResponseString(t *testing.T) {
	tests := []struct {
		name     string
		dump     string
		wantBody bool
	}{
		{name: "headers and body", dump: "true", wantBody: true},
		{name: "headers only", dump: "false", wantBody: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DUMP_CONTENT", tt.dump)
			res := &http.Response{
				Status:        "200 OK",
				StatusCode:    http.StatusOK,
				Proto:         "HTTP/1.1",
				ProtoMajor:    1,
				ProtoMinor:    1,
				Header:        http.Header{"X-Test": {"yes"}},
				Body:          io.NopCloser(strings.NewReader("<html>ok</html>")),
				ContentLength: int64(len("<html>ok</html>")),
			}

			out := ResponseString(res)
			assert.Contains(t, out, "HTTP/1.1 200 OK")
			assert.Contains(t, out, "X-Test: yes")
			if tt.wantBody {
				assert.Contains(t, out, "<html>ok</html>")
				// The body is restored so callers can still read it.
				body, err := io.ReadAll(res.Body)
				require.NoError(t, err)
				assert.Equal(t, "<html>ok</html>", string(body))
			} else {
				assert.NotContains(t, out, "<html>ok</html>")
			}
		})
	}
}

func TestResponseStringError(t *testing.T) {
	t.Setenv("DUMP_CONTENT", "true")
	res := &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{},
		Body:       io.NopCloser(failingReader{}),
	}

	assert.Empty(t, ResponseString(res))
}
