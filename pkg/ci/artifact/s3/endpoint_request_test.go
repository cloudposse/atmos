package s3

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	sdk "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/artifact"
)

func TestIdentityEndpointUsesBucketPath(t *testing.T) {
	requests := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)
	_, port, err := net.SplitHostPort(endpoint.Host)
	require.NoError(t, err)
	endpoint.Host = net.JoinHostPort("localhost", port)
	client := sdk.NewFromConfig(aws.Config{
		Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
	}, buildClientOptFns(&artifact.AWSAuthConfig{EndpointURL: endpoint.String()})...)
	_, err = client.HeadObject(t.Context(), &sdk.HeadObjectInput{Bucket: aws.String("artifacts"), Key: aws.String("templates/stack.yaml")})
	require.NoError(t, err)
	select {
	case request := <-requests:
		require.Equal(t, endpoint.Host, request.Host)
		require.Equal(t, "/artifacts/templates/stack.yaml", request.URL.Path)
	default:
		t.Fatal("SDK request did not reach the emulator endpoint")
	}
}

func TestAdapterIdentityResolverRoutesToEndpoint(t *testing.T) {
	requests := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	resolver := &fakeAuthContextResolver{awsResult: &artifact.AWSAuthConfig{Region: "us-east-1", EndpointURL: server.URL}}
	backend, err := NewStore(artifact.StoreOptions{Identity: "deploy", Options: map[string]any{"bucket": "artifacts"}})
	require.NoError(t, err)
	store := backend.(*Store)
	store.SetAuthContext(resolver, "")
	exists, err := store.Exists(t.Context(), "path/object")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, "deploy", resolver.requestedIDName)
	select {
	case request := <-requests:
		require.Equal(t, "/artifacts/path/object", request.URL.Path)
	default:
		t.Fatal("identity endpoint received no request")
	}
}
