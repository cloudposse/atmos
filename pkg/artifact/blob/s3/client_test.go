package s3

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	sdk "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestIdentityIsResolvedLazily(t *testing.T) {
	resolver := &fakeResolver{err: errors.New("sso expired")}
	s, err := New("bucket", WithIdentity("deploy", resolver), WithRegion("us-east-1"))
	require.NoError(t, err, "constructing the store must not touch credentials")
	assert.Zero(t, resolver.calls.Load())

	_, err = s.Stat(context.Background(), "k", blob.StatOptions{})
	require.Error(t, err)
	assert.Equal(t, int32(1), resolver.calls.Load())
}

func TestUnresolvableIdentityFailsClosed(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "ambient-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient-secret")
	t.Setenv("AWS_REGION", "us-east-1")

	cases := map[string]*fakeResolver{
		"resolver error":       {err: errors.New("sso expired")},
		"nil auth context":     {},
		"no resolver injected": nil,
	}
	for name, resolver := range cases {
		t.Run(name, func(t *testing.T) {
			var opt Option
			if resolver == nil {
				opt = WithIdentity("deploy", nil)
			} else {
				opt = WithIdentity("deploy", resolver)
			}
			s, err := New("bucket", opt)
			require.NoError(t, err)
			for range 2 {
				_, err = s.Get(context.Background(), "k", blob.GetOptions{})
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrBlobIdentityUnresolved, "never fall back to ambient credentials")
				assert.Contains(t, err.Error(), "deploy", "the error names the identity")
			}
			if resolver != nil {
				assert.Equal(t, int32(2), resolver.calls.Load(), "a failed resolution is retried by the next operation, not cached")
			}
		})
	}
}

func TestLegacyClientKeepsAmbientFallback(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "k")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "s")

	client, err := NewClient(context.Background(), &ClientConfig{Identity: "deploy", Legacy: true})
	require.NoError(t, err, "the legacy CI adapter keeps its historical fallback")
	assert.NotNil(t, client)

	_, err = NewClient(context.Background(), &ClientConfig{Identity: "deploy"})
	require.ErrorIs(t, err, errUtils.ErrBlobIdentityUnresolved, "the generic path never does")
	require.ErrorIs(t, err, errUtils.ErrAWSConfigLoadFailed)
}

func TestIdentityEndpointAndPathStyleRouteToTheEmulator(t *testing.T) {
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

	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	resolver := &fakeResolver{result: &store.AWSAuthConfig{Region: "us-east-1", EndpointURL: endpoint.String()}}
	s, err := New("artifacts", WithIdentity("floci", resolver))
	require.NoError(t, err)

	_, err = s.Stat(context.Background(), "templates/stack.yaml", blob.StatOptions{})
	require.NoError(t, err)
	request := <-requests
	assert.Equal(t, endpoint.Host, request.Host, "path-style: the bucket is not in the host name")
	assert.Equal(t, "/artifacts/templates/stack.yaml", request.URL.Path)
}

func TestExplicitEndpointWinsOverIdentityEndpoint(t *testing.T) {
	cfg := &ClientConfig{EndpointURL: "http://explicit:9000"}
	var opts sdk.Options
	for _, fn := range clientOptFns(cfg, &store.AWSAuthConfig{EndpointURL: "http://identity:4566"}) {
		fn(&opts)
	}
	assert.Equal(t, aws.String("http://explicit:9000"), opts.BaseEndpoint)
	assert.True(t, opts.UsePathStyle, "an endpoint override implies path-style addressing")

	pathStyle := false
	cfg.PathStyle = &pathStyle
	opts = sdk.Options{}
	for _, fn := range clientOptFns(cfg, nil) {
		fn(&opts)
	}
	assert.False(t, opts.UsePathStyle, "path-style can be turned off for a custom endpoint")
}

func TestGenericClientSkipsOptionalChecksumsButLegacyKeepsSDKDefaults(t *testing.T) {
	var generic, legacy sdk.Options
	for _, fn := range clientOptFns(&ClientConfig{}, nil) {
		fn(&generic)
	}
	for _, fn := range clientOptFns(&ClientConfig{Legacy: true}, nil) {
		fn(&legacy)
	}
	assert.Equal(t, aws.RequestChecksumCalculationWhenRequired, generic.RequestChecksumCalculation)
	assert.Equal(t, aws.RequestChecksumCalculation(0), legacy.RequestChecksumCalculation)
}

func TestBuildAuthConfigOpts(t *testing.T) {
	cases := []struct {
		name   string
		region string
		auth   *store.AWSAuthConfig
		want   config.LoadOptions
	}{
		{"nil", "", nil, config.LoadOptions{}},
		{
			"files and profile", "", &store.AWSAuthConfig{CredentialsFile: "/c", ConfigFile: "/f", Profile: "p"},
			config.LoadOptions{SharedCredentialsFiles: []string{"/c"}, SharedConfigFiles: []string{"/f"}, SharedConfigProfile: "p"},
		},
		{"store region wins", "us-east-1", &store.AWSAuthConfig{Region: "us-west-2"}, config.LoadOptions{Region: "us-east-1"}},
		{"identity region fills in", "", &store.AWSAuthConfig{Region: "eu-west-1"}, config.LoadOptions{Region: "eu-west-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got config.LoadOptions
			for _, opt := range BuildAuthConfigOpts(tc.region, tc.auth) {
				require.NoError(t, opt(&got))
			}
			assert.Equal(t, tc.want.Region, got.Region)
			assert.Equal(t, tc.want.SharedConfigProfile, got.SharedConfigProfile)
			assert.Equal(t, tc.want.SharedCredentialsFiles, got.SharedCredentialsFiles)
			assert.Equal(t, tc.want.SharedConfigFiles, got.SharedConfigFiles)
		})
	}
}

func TestBuildClientOptFnsMatchesLegacyEndpointBehavior(t *testing.T) {
	assert.Empty(t, BuildClientOptFns(nil))
	assert.Empty(t, BuildClientOptFns(&store.AWSAuthConfig{}))
	var opts sdk.Options
	for _, fn := range BuildClientOptFns(&store.AWSAuthConfig{EndpointURL: "http://localhost:4566"}) {
		fn(&opts)
	}
	assert.Equal(t, aws.String("http://localhost:4566"), opts.BaseEndpoint)
	assert.True(t, opts.UsePathStyle)
}

func TestHTTPClientInjectionReachesTheSDK(t *testing.T) {
	fake := newFake(t)
	var used bool
	client := httpClientFunc(func(r *http.Request) (*http.Response, error) {
		used = true
		return http.DefaultClient.Do(r)
	})
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	api, err := NewClient(context.Background(), &ClientConfig{Region: "us-east-1", EndpointURL: fake.Endpoint(), HTTPClient: client})
	require.NoError(t, err)
	s, err := NewFromClient(api, fake.Bucket())
	require.NoError(t, err)
	_, err = s.Put(context.Background(), "k", bytes.NewReader([]byte("x")), blob.PutOptions{Size: 1})
	require.NoError(t, err)
	assert.True(t, used)
}

type httpClientFunc func(*http.Request) (*http.Response, error)

func (f httpClientFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }
