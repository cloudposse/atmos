package aws

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestPermissionSetIdentity_resolveAccountID_Pagination(t *testing.T) {
	// Reproduce an account beyond the first ten results without a live SSO session.
	accounts := make([]string, 10)
	for idx := range accounts {
		accounts[idx] = fmt.Sprintf(`{"accountId":"%012d","accountName":"APP/TEST/%d"}`, idx+1, idx+1)
	}
	firstPage := `{"accountList":[` + strings.Join(accounts, ",") + `],"nextToken":"page-2"}`
	const matchPage = `{"accountList":[{"accountId":"123456789012","accountName":"SVC/TEST/1"}]}`
	const errorPage = `{"message":"session expired"}`

	type page struct {
		token  string
		body   string
		status int
	}
	tests := []struct {
		name         string
		pages        []page
		wantID       string
		wantError    string
		wantAPIError bool
	}{
		{
			name:   "match on first page stops before fetching next page",
			pages:  []page{{body: `{"accountList":[{"accountId":"123456789012","accountName":"SVC/TEST/1"}],"nextToken":"unused"}`}},
			wantID: "123456789012",
		},
		{
			name:   "match beyond first ten accounts",
			pages:  []page{{body: firstPage}, {token: "page-2", body: matchPage}},
			wantID: "123456789012",
		},
		{
			name: "continues through empty intermediate page",
			pages: []page{
				{body: firstPage},
				{token: "page-2", body: `{"accountList":[],"nextToken":"page-3"}`},
				{token: "page-3", body: matchPage},
			},
			wantID: "123456789012",
		},
		{
			name:      "not found after all pages",
			pages:     []page{{body: firstPage}, {token: "page-2", body: `{"accountList":[]}`}},
			wantError: `account "SVC/TEST/1" not found`,
		},
		{
			name:      "empty account list",
			pages:     []page{{body: `{"accountList":[]}`}},
			wantError: `account "SVC/TEST/1" not found`,
		},
		{
			name:      "empty next token ends pagination",
			pages:     []page{{body: `{"accountList":[],"nextToken":""}`}},
			wantError: `account "SVC/TEST/1" not found`,
		},
		{
			name:         "first page API error",
			pages:        []page{{body: errorPage, status: http.StatusUnauthorized}},
			wantError:    "failed to list accounts",
			wantAPIError: true,
		},
		{
			name:         "later page API error",
			pages:        []page{{body: firstPage}, {token: "page-2", body: errorPage, status: http.StatusUnauthorized}},
			wantError:    "failed to list accounts",
			wantAPIError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				idx := int(requests.Add(1)) - 1
				if idx >= len(tt.pages) {
					t.Error("unexpected extra ListAccounts request")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				response := tt.pages[idx]
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/assignment/accounts", r.URL.Path)
				assert.Equal(t, response.token, r.URL.Query().Get("next_token"))
				assert.Equal(t, "test-access-token", r.Header.Get("x-amz-sso_bearer_token"))
				w.Header().Set("Content-Type", "application/json")
				if response.status != 0 {
					w.Header().Set("x-amzn-errortype", "UnauthorizedException")
					w.WriteHeader(response.status)
				}
				_, err := w.Write([]byte(response.body))
				assert.NoError(t, err)
			}))
			t.Cleanup(server.Close)
			client := sso.New(sso.Options{
				Region:       "us-east-1",
				BaseEndpoint: aws.String(server.URL),
				Credentials:  aws.AnonymousCredentials{},
				HTTPClient:   server.Client(),
				Retryer:      aws.NopRetryer{},
			})
			identity := &permissionSetIdentity{name: "svc-test-1"}
			id, err := identity.resolveAccountID(context.Background(), client, "SVC/TEST/1", "", "test-access-token")
			assert.Equal(t, len(tt.pages), int(requests.Load()))
			assert.Equal(t, tt.wantID, id)
			if tt.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrAwsAuth)
			assert.ErrorContains(t, err, tt.wantError)
			if tt.wantAPIError {
				var apiErr *ssotypes.UnauthorizedException
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, "session expired", apiErr.ErrorMessage())
			}
		})
	}
}

func TestPermissionSetIdentity_resolveAccountID_ProvidedIDTakesPrecedence(t *testing.T) {
	identity := &permissionSetIdentity{name: "svc-test-1"}
	// A nil client ensures supplying both name and ID never makes an API call.
	id, err := identity.resolveAccountID(context.Background(), nil, "SVC/TEST/1", "123456789012", "token")
	require.NoError(t, err)
	assert.Equal(t, "123456789012", id)
}
