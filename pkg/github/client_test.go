package github

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestHandleGitHubAPIError tests the handleGitHubAPIError function.
func TestHandleGitHubAPIError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		resp      *github.Response
		wantErr   bool
		errString string
	}{
		{
			name: "rate limit exceeded with response",
			err:  errors.New("API rate limit exceeded"),
			resp: &github.Response{
				Rate: github.Rate{
					Remaining: 0,
					Limit:     5000,
					Reset:     github.Timestamp{Time: time.Now().Add(30 * time.Minute)},
				},
			},
			wantErr:   true,
			errString: "rate limit exceeded",
		},
		{
			name:      "non-rate-limit error",
			err:       errors.New("other error"),
			resp:      &github.Response{Rate: github.Rate{Remaining: 100}},
			wantErr:   true,
			errString: "other error",
		},
		{
			name:      "nil response",
			err:       errors.New("network error"),
			resp:      nil,
			wantErr:   true,
			errString: "network error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handleGitHubAPIError(tt.err, tt.resp)
			if tt.wantErr {
				assert.Error(t, err)
				// For rate limit errors, check the wrapped error type.
				if tt.name == "rate limit exceeded with response" {
					assert.ErrorIs(t, err, errUtils.ErrGitHubRateLimitExceeded)
				}
				// For other errors, verify the error message is preserved.
				if tt.name != "rate limit exceeded with response" {
					assert.Contains(t, err.Error(), tt.errString)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
