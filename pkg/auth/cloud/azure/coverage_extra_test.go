package azure

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	httpClient "github.com/cloudposse/atmos/pkg/http"
)

func TestFirstValue(t *testing.T) {
	assert.Equal(t, "x", firstValue(map[string]string{"a": "x"}))
	// Empty map returns the zero value.
	assert.Equal(t, "", firstValue(map[string]string{}))
	assert.Equal(t, 0, firstValue(map[int]int{}))
}

func TestContextWithAKSServerID_Empty(t *testing.T) {
	ctx := context.Background()
	// Empty server ID returns the context unchanged and falls back to the default scope.
	got := ContextWithAKSServerID(ctx, "")
	assert.Equal(t, ctx, got)
	assert.Equal(t, AKSServerScope, AKSServerScopeFromContext(got))
}

func TestGetAuthorizationToken_DoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := httpClient.NewMockClient(ctrl)
	client.EXPECT().Do(gomock.Any()).Return(nil, errors.New("network down"))

	origClient := acrHTTPClient
	acrHTTPClient = client
	t.Cleanup(func() { acrHTTPClient = origClient })

	creds := &types.AzureCredentials{AccessToken: "aad-token", TenantID: "tenant-123"}
	_, err := GetAuthorizationToken(context.Background(), creds, "myregistry.azurecr.io")
	require.Error(t, err)
}

func TestGetAuthorizationToken_MalformedJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := httpClient.NewMockClient(ctrl)
	client.EXPECT().Do(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("this is not json")),
	}, nil)

	origClient := acrHTTPClient
	acrHTTPClient = client
	t.Cleanup(func() { acrHTTPClient = origClient })

	creds := &types.AzureCredentials{AccessToken: "aad-token", TenantID: "tenant-123"}
	_, err := GetAuthorizationToken(context.Background(), creds, "myregistry.azurecr.io")
	require.Error(t, err)
}
