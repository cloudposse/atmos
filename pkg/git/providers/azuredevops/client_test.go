package azuredevops

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pkghttp "github.com/cloudposse/atmos/pkg/http"
)

// TestDoRequestWrapsErrors proves each failure point inside doRequest -- marshaling the request
// body, constructing the *http.Request, and the underlying HTTP round trip itself -- is wrapped
// under errUtils.ErrPullRequestReconciliation rather than returned bare or silently swallowed.
func TestDoRequestWrapsErrors(t *testing.T) {
	tests := []struct {
		name    string
		buildP  func(t *testing.T) *Provider
		request restRequest
	}{
		{
			name: "unmarshalable body",
			buildP: func(t *testing.T) *Provider {
				t.Helper()
				return New(WithToken("s3cr3t-pat"))
			},
			// Channels cannot be JSON-marshaled, so this always fails inside json.Marshal.
			request: restRequest{method: http.MethodPost, url: "https://dev.azure.com/x", token: "t", body: make(chan int)},
		},
		{
			name: "invalid HTTP method",
			buildP: func(t *testing.T) *Provider {
				t.Helper()
				return New(WithToken("s3cr3t-pat"))
			},
			// A space is not a valid HTTP token character, so http.NewRequestWithContext rejects it.
			request: restRequest{method: "BAD METHOD", url: "https://dev.azure.com/x", token: "t"},
		},
		{
			name: "transport failure",
			buildP: func(t *testing.T) *Provider {
				t.Helper()
				ctrl := gomock.NewController(t)
				mockClient := pkghttp.NewMockClient(ctrl)
				mockClient.EXPECT().Do(gomock.Any()).Return(nil, assert.AnError)
				return New(WithHTTPClient(mockClient), WithToken("s3cr3t-pat"))
			},
			request: restRequest{method: http.MethodGet, url: "https://dev.azure.com/x", token: "t"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.buildP(t)
			err := p.doRequest(context.Background(), tt.request, nil)
			assert.ErrorIs(t, err, errUtils.ErrPullRequestReconciliation)
		})
	}
}

// TestDecodeResponseDecodeError proves a 2xx response whose body isn't valid JSON surfaces a
// wrapped, actionable error instead of a bare json.Decoder error or a silent zero-value out.
func TestDecodeResponseDecodeError(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("not-json"))}

	err := decodeResponse(resp, &pullRequest{})

	assert.ErrorIs(t, err, errUtils.ErrPullRequestReconciliation)
}
