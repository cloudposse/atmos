package atmosbridge

import (
	"crypto/sha1" //nolint:gosec // The SDK names its SSO token cache file after a SHA-1.
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_roundtripper_test.go -package=atmosbridge net/http RoundTripper

const testAccount = "123456789012"

// stsServer answers the two STS actions Terragrunt issues and records who signed each request.
type stsServer struct {
	*httptest.Server
	mu       sync.Mutex
	signers  []string
	actions  []string
	hosts    []string
	requests []*http.Request
	bodies   [][]byte
}

var credentialPattern = regexp.MustCompile(`Credential=([^/]+)/`)

func newSTSServer(t *testing.T) *stsServer {
	t.Helper()
	server := &stsServer{}
	server.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		signer := ""
		if match := credentialPattern.FindStringSubmatch(r.Header.Get("Authorization")); match != nil {
			signer = match[1]
		}
		action := "GetCallerIdentity"
		if strings.Contains(string(body), "Action=AssumeRole") {
			action = "AssumeRole"
		}
		server.mu.Lock()
		server.signers, server.actions, server.hosts = append(server.signers, signer), append(server.actions, action), append(server.hosts, r.Host)
		server.requests, server.bodies = append(server.requests, r.Clone(r.Context())), append(server.bodies, body)
		server.mu.Unlock()
		w.Header().Set("Content-Type", "text/xml")
		if action == "AssumeRole" {
			expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
			fmt.Fprintf(w, `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleResult><Credentials><AccessKeyId>ASIAASSUMED</AccessKeyId><SecretAccessKey>assumed-secret</SecretAccessKey><SessionToken>assumed-token</SessionToken><Expiration>%s</Expiration></Credentials></AssumeRoleResult></AssumeRoleResponse>`, expires)
			return
		}
		fmt.Fprintf(w, `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><GetCallerIdentityResult><Arn>arn:aws:iam::%s:user/atmos</Arn><UserId>AIDATEST</UserId><Account>%s</Account></GetCallerIdentityResult></GetCallerIdentityResponse>`, testAccount, testAccount)
	}))
	t.Cleanup(server.Close)
	return server
}

func (s *stsServer) seen() (signers, actions []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.signers), slices.Clone(s.actions)
}

// writeProfile creates a shared credentials file with one profile and an empty config file.
func writeProfile(t *testing.T, profile, key string) (credentialsFile, configFile string) {
	t.Helper()
	dir := t.TempDir()
	credentialsFile, configFile = filepath.Join(dir, "credentials"), filepath.Join(dir, "config")
	content := fmt.Sprintf("[%s]\naws_access_key_id = %s\naws_secret_access_key = secret-%s\naws_session_token = token-%s\n", profile, key, key, key)
	require.NoError(t, os.WriteFile(credentialsFile, []byte(content), 0o600))
	require.NoError(t, os.WriteFile(configFile, nil, 0o600))
	return credentialsFile, configFile
}

// accountID asks upstream for the caller identity exactly as get_aws_account_id() does.
func accountID(t *testing.T, scoped *venv.Venv) (string, error) {
	t.Helper()
	logger := log.New(log.WithOutput(io.Discard))
	cfg, err := awshelper.NewAWSConfigBuilder().Build(t.Context(), logger, scoped)
	require.NoError(t, err)
	return awshelper.GetAWSAccountID(t.Context(), &cfg)
}

func TestAWSHonorsProfileFilesAndEndpointFromInvocationOnly(t *testing.T) {
	server := newSTSServer(t)
	credentials, configFile := writeProfile(t, "atmos-identity", "AKIAINVOCATION")
	// A known, valid process profile, so the test does not depend on the developer's own AWS_PROFILE.
	processCredentials, processConfig := writeProfile(t, "process-profile", "AKIAPROCESS")
	t.Setenv("AWS_PROFILE", "process-profile")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", processCredentials)
	t.Setenv("AWS_CONFIG_FILE", processConfig)
	before := os.Environ()
	env := map[string]string{
		"AWS_PROFILE": "atmos-identity", "AWS_SHARED_CREDENTIALS_FILE": credentials, "AWS_CONFIG_FILE": configFile,
		"AWS_ENDPOINT_URL_STS": server.URL, "AWS_REGION": "eu-west-1", "AWS_EC2_METADATA_DISABLED": "true",
	}

	account, err := accountID(t, AWS(t.Context(), Environment(env, io.Discard, io.Discard)))

	require.NoError(t, err)
	assert.Equal(t, testAccount, account)
	signers, actions := server.seen()
	assert.Equal(t, []string{"AKIAINVOCATION"}, signers, "the profile of the invocation signs the call")
	assert.Equal(t, []string{"GetCallerIdentity"}, actions)
	assert.Equal(t, before, os.Environ(), "the plugin process environment is unchanged")
	assert.Equal(t, "process-profile", os.Getenv("AWS_PROFILE"))
	assert.NotContains(t, env, "AWS_ACCESS_KEY_ID", "the caller's map is never mutated")
}

func TestAWSIgnoresProcessProfileWhenInvocationSelectsAnother(t *testing.T) {
	server := newSTSServer(t)
	processCredentials, processConfig := writeProfile(t, "process-profile", "AKIAPROCESS")
	t.Setenv("AWS_PROFILE", "process-profile")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", processCredentials)
	t.Setenv("AWS_CONFIG_FILE", processConfig)
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAAMBIENT")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient")
	credentials, configFile := writeProfile(t, "selected", "AKIASELECTED")
	env := map[string]string{
		"AWS_PROFILE": "selected", "AWS_SHARED_CREDENTIALS_FILE": credentials, "AWS_CONFIG_FILE": configFile,
		"AWS_ENDPOINT_URL": server.URL, "AWS_EC2_METADATA_DISABLED": "true",
	}

	_, err := accountID(t, AWS(t.Context(), Environment(env, io.Discard, io.Discard)))

	require.NoError(t, err)
	signers, _ := server.seen()
	assert.Equal(t, []string{"AKIASELECTED"}, signers, "ambient process credentials never outrank the selected identity")
	assert.Equal(t, "process-profile", os.Getenv("AWS_PROFILE"))
}

// Shared files without a profile select their default profile; static keys and a web identity token file
// in the plugin process environment never outrank them.
func TestAWSIgnoresProcessCredentialsWhenInvocationSuppliesOnlyFiles(t *testing.T) {
	server := newSTSServer(t)
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_DEFAULT_PROFILE", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAAMBIENT")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient")
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", filepath.Join(t.TempDir(), "ambient-token"))
	t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/ambient")
	credentials, configFile := writeProfile(t, "default", "AKIAFILES")
	env := map[string]string{
		"AWS_SHARED_CREDENTIALS_FILE": credentials, "AWS_CONFIG_FILE": configFile,
		"AWS_ENDPOINT_URL": server.URL, "AWS_EC2_METADATA_DISABLED": "true",
	}

	_, err := accountID(t, AWS(t.Context(), Environment(env, io.Discard, io.Discard)))

	require.NoError(t, err)
	signers, actions := server.seen()
	assert.Equal(t, []string{"AKIAFILES"}, signers, "the invocation's files sign the call, not the process keys")
	assert.Equal(t, []string{"GetCallerIdentity"}, actions, "the process web identity role is never assumed")
	assert.Equal(t, "AKIAAMBIENT", os.Getenv("AWS_ACCESS_KEY_ID"), "the plugin process environment is unchanged")
}

func TestAWSKeepsStaticKeysAndRoutesEndpointOnly(t *testing.T) {
	server := newSTSServer(t)
	env := map[string]string{"AWS_ACCESS_KEY_ID": "AKIASTATIC", "AWS_SECRET_ACCESS_KEY": "secret", "AWS_ENDPOINT_URL_STS": server.URL, "AWS_PROFILE": "ignored"}

	_, err := accountID(t, AWS(t.Context(), Environment(env, io.Discard, io.Discard)))

	require.NoError(t, err)
	signers, _ := server.seen()
	assert.Equal(t, []string{"AKIASTATIC"}, signers)
}

func TestAWSAssumesCommandLevelRoleOnTopOfProfile(t *testing.T) {
	server := newSTSServer(t)
	credentials, configFile := writeProfile(t, "base", "AKIABASE")
	env := map[string]string{
		"AWS_PROFILE": "base", "AWS_SHARED_CREDENTIALS_FILE": credentials, "AWS_CONFIG_FILE": configFile,
		"AWS_ENDPOINT_URL_STS": server.URL, "AWS_EC2_METADATA_DISABLED": "true",
		"TG_IAM_ASSUME_ROLE": "arn:aws:iam::123456789012:role/deploy", "TG_IAM_ASSUME_ROLE_SESSION_NAME": "atmos",
	}

	_, err := accountID(t, AWS(t.Context(), Environment(env, io.Discard, io.Discard)))

	require.NoError(t, err)
	signers, actions := server.seen()
	assert.Equal(t, []string{"AssumeRole", "GetCallerIdentity"}, actions)
	assert.Equal(t, []string{"AKIABASE", "ASIAASSUMED"}, signers, "the role is assumed with the profile, and the assumed session answers the identity call")
}

func TestAWSNamesTheCauseWhenCredentialsCannotBeResolved(t *testing.T) {
	server := newSTSServer(t)
	credentials, configFile := writeProfile(t, "present", "AKIAPRESENT")
	env := map[string]string{
		"AWS_PROFILE": "missing", "AWS_SHARED_CREDENTIALS_FILE": credentials, "AWS_CONFIG_FILE": configFile,
		"AWS_ENDPOINT_URL_STS": server.URL, "AWS_EC2_METADATA_DISABLED": "true",
	}

	_, err := accountID(t, AWS(t.Context(), Environment(env, io.Discard, io.Discard)))

	require.Error(t, err)
	require.ErrorIs(t, err, ErrAWSCredentials)
	assert.Contains(t, err.Error(), "missing")
	signers, _ := server.seen()
	assert.Empty(t, signers, "no request leaves with substitute credentials")
}

// An invocation that supplies no credentials stays authoritative: the plugin process's keys, container
// credentials and instance role are never used in its place.
func TestAWSDoesNotFallBackToProcessCredentialsWithoutInvocationCredentials(t *testing.T) {
	var ambientCalls atomic.Int32
	ambient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ambientCalls.Add(1)
		_, _ = io.WriteString(w, `{"AccessKeyId":"AKIAAMBIENT","SecretAccessKey":"secret","Token":"ambient-token","Expiration":"2099-01-01T00:00:00Z"}`)
	}))
	defer ambient.Close()
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", ambient.URL)
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAAMBIENT")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient")
	server := newSTSServer(t)
	env := map[string]string{"PATH": "/bin", "AWS_ENDPOINT_URL_STS": server.URL, "AWS_EC2_METADATA_DISABLED": "true"}

	scoped := AWS(t.Context(), Environment(env, io.Discard, io.Discard))
	_, err := accountID(t, scoped)

	require.ErrorIs(t, err, ErrAWSCredentials)
	require.ErrorContains(t, err, "supplies no AWS credentials")
	signers, _ := server.seen()
	assert.Empty(t, signers, "no request leaves with process credentials")
	assert.Zero(t, ambientCalls.Load(), "the process credential endpoint is never called")
	assert.Equal(t, "/bin", scoped.Env["PATH"])
}

// A profile that holds only a region has no credential source in the invocation's files, so the SDK would
// reach the plugin process's container credentials. It is rejected; file-backed sources still resolve.
func TestAWSRejectsRegionOnlyProfileInsteadOfProcessCredentials(t *testing.T) {
	var calls atomic.Int32
	ambient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"AccessKeyId":"AKIAAMBIENT","SecretAccessKey":"secret","Token":"ambient-token","Expiration":"2099-01-01T00:00:00Z"}`)
	}))
	defer ambient.Close()
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", ambient.URL)
	t.Setenv("AWS_CONTAINER_AUTHORIZATION_TOKEN", "ambient-authorization")
	server := newSTSServer(t)
	credentials, configFile := writeProfile(t, "source", "AKIAFILES")
	require.NoError(t, os.WriteFile(configFile, []byte("[profile regional]\nregion = eu-west-1\n[profile chained]\nrole_arn = arn:aws:iam::1:role/test\nsource_profile = regional\n[profile viasource]\nrole_arn = arn:aws:iam::1:role/test\nsource_profile = source\n"), 0o600))
	base := map[string]string{"AWS_SHARED_CREDENTIALS_FILE": credentials, "AWS_CONFIG_FILE": configFile, "AWS_ENDPOINT_URL_STS": server.URL, "AWS_EC2_METADATA_DISABLED": "true"}

	// A role whose source profile has no credentials is refused by the SDK's own profile loading.
	for _, profile := range []string{"regional", "chained"} {
		t.Run(profile, func(t *testing.T) {
			env := maps.Clone(base)
			env["AWS_PROFILE"] = profile
			scoped := AWS(t.Context(), Environment(env, io.Discard, io.Discard))
			require.Equal(t, awsUnresolvedKey, scoped.Env[envAccessKeyID])
			_, err := accountID(t, scoped)
			require.ErrorIs(t, err, ErrAWSCredentials)
		})
	}
	assert.Zero(t, calls.Load(), "the process credential endpoint must never receive a request")

	t.Run("file-backed source profile still resolves", func(t *testing.T) {
		env := maps.Clone(base)
		env["AWS_PROFILE"] = "viasource"
		scoped := AWS(t.Context(), Environment(env, io.Discard, io.Discard))
		assert.Equal(t, "ASIAASSUMED", scoped.Env[envAccessKeyID])
	})
}

// The web identity role is assumed from the invocation's values alone: a process profile or shared file
// that cannot be loaded neither fails nor changes the resolution.
func TestAWSWebIdentityDoesNotLoadTheProcessCredentialChain(t *testing.T) {
	broken := filepath.Join(t.TempDir(), "broken-config")
	require.NoError(t, os.WriteFile(broken, []byte("[profile broken\n"), 0o600))
	t.Setenv("AWS_CONFIG_FILE", broken)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", broken)
	t.Setenv("AWS_PROFILE", "absent-process-profile")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprintf(w, `<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleWithWebIdentityResult><Credentials><AccessKeyId>ASIAWEB</AccessKeyId><SecretAccessKey>web</SecretAccessKey><SessionToken>web-token</SessionToken><Expiration>%s</Expiration></Credentials></AssumeRoleWithWebIdentityResult></AssumeRoleWithWebIdentityResponse>`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	}))
	t.Cleanup(server.Close)
	token := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(token, []byte("jwt"), 0o600))
	env := map[string]string{
		"AWS_WEB_IDENTITY_TOKEN_FILE": token, "AWS_ROLE_ARN": "arn:aws:iam::123456789012:role/web",
		"AWS_REGION": "eu-central-1", "AWS_ENDPOINT_URL_STS": server.URL,
	}

	scoped := AWS(t.Context(), Environment(env, io.Discard, io.Discard))

	assert.Equal(t, "ASIAWEB", scoped.Env["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, "eu-central-1", awsRegion(env))
	assert.Equal(t, "us-east-1", awsRegion(map[string]string{}))
}

func TestAWSTransportRoutesByService(t *testing.T) {
	var mu sync.Mutex
	hits := map[string][]string{}
	handler := func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			hits[name] = append(hits[name], r.Host+r.URL.Path)
			mu.Unlock()
		})
	}
	global, sts, s3 := httptest.NewServer(handler("global")), httptest.NewServer(handler("sts")), httptest.NewServer(handler("s3"))
	t.Cleanup(global.Close)
	t.Cleanup(sts.Close)
	t.Cleanup(s3.Close)
	other := httptest.NewServer(handler("other"))
	t.Cleanup(other.Close)
	transport := &awsTransport{endpoints: awsEndpoints(map[string]string{
		"AWS_ENDPOINT_URL": global.URL, "AWS_ENDPOINT_URL_STS": sts.URL, "AWS_ENDPOINT_URL_S3": s3.URL, "AWS_ENDPOINT_URL_BAD": "not a url",
	})}
	client := &http.Client{Transport: transport}

	for _, target := range []string{
		"https://sts.us-east-1.amazonaws.com/", "https://bucket.s3.us-west-2.amazonaws.com/key", "https://dynamodb.us-east-1.amazonaws.com/", other.URL + "/untouched",
	} {
		response, err := client.Get(target)
		require.NoError(t, err)
		_ = response.Body.Close()
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"sts.us-east-1.amazonaws.com/"}, hits["sts"], "the original host name and path are kept for signature validation")
	assert.Equal(t, []string{"bucket.s3.us-west-2.amazonaws.com/key"}, hits["s3"])
	assert.Equal(t, []string{"dynamodb.us-east-1.amazonaws.com/"}, hits["global"])
	assert.Len(t, hits["other"], 1, "hosts outside AWS are never rewritten")
	assert.Contains(t, hits["other"][0], "/untouched")
}

func TestAWSTransportSignsEndpointPathPrefix(t *testing.T) {
	credentials := aws.Credentials{AccessKeyID: "AKIATEST", SecretAccessKey: "secret"}
	req, err := http.NewRequest(http.MethodPost, "https://sts.us-east-1.amazonaws.com/action?version=1", strings.NewReader("Action=GetCallerIdentity"))
	require.NoError(t, err)
	digest := sha256.Sum256([]byte("Action=GetCallerIdentity"))
	hash := hex.EncodeToString(digest[:])
	stamp := time.Date(2026, time.November, 12, 12, 0, 0, 0, time.UTC)
	require.NoError(t, v4.NewSigner().SignHTTP(t.Context(), credentials, req, hash, "sts", "us-east-1", stamp))
	oldSignature := req.Header.Get("Authorization")
	var sent *http.Request
	next := NewMockRoundTripper(gomock.NewController(t))
	next.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(r *http.Request) (*http.Response, error) {
		sent = r
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	}).Times(1)
	transport := &awsTransport{
		credentials: credentials,
		endpoints:   map[string]*url.URL{"sts": {Scheme: "https", Host: "proxy.example", Path: "/aws/sts"}},
		next:        next,
	}
	response, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, "/aws/sts/action", sent.URL.Path)
	require.Equal(t, "version=1", sent.URL.RawQuery)
	require.Equal(t, "sts.us-east-1.amazonaws.com", sent.Host)
	require.NotEqual(t, oldSignature, sent.Header.Get("Authorization"))
	actualSignature := sent.Header.Get("Authorization")
	sent.Header.Del("Authorization")
	require.NoError(t, v4.NewSigner().SignHTTP(t.Context(), credentials, sent, hash, "sts", "us-east-1", stamp))
	require.Equal(t, actualSignature, sent.Header.Get("Authorization"), "the signature covers the final path")
}

func TestAWSTransportRejectsInvalidPrefixedSigning(t *testing.T) {
	const authorization = "AWS4-HMAC-SHA256 Credential=AKIATEST/20261112/us-east-1/sts/aws4_request, SignedHeaders=host;x-amz-date, Signature=old"
	bodyFailure := errors.New("request body unavailable")
	for _, test := range []struct {
		name, authorization, stamp string
		bodyFailure                bool
	}{
		{name: "malformed authorization", authorization: "malformed", stamp: "20261112T120000Z"},
		{name: "different signing credentials", authorization: strings.ReplaceAll(authorization, "AKIATEST", "AKIACHANGED"), stamp: "20261112T120000Z"},
		{name: "missing signing time", authorization: authorization},
		{name: "unreadable body", authorization: authorization, stamp: "20261112T120000Z", bodyFailure: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "https://sts.us-east-1.amazonaws.com/action", strings.NewReader("Action=GetCallerIdentity"))
			require.NoError(t, err)
			req.Header.Set("Authorization", test.authorization)
			req.Header.Set("X-Amz-Date", test.stamp)
			if test.bodyFailure {
				req.Body = io.NopCloser(iotest.ErrReader(bodyFailure))
			}
			transport := &awsTransport{
				credentials: aws.Credentials{AccessKeyID: "AKIATEST", SecretAccessKey: "secret"},
				endpoints:   map[string]*url.URL{"sts": {Scheme: "https", Host: "proxy.example", Path: "/aws/sts"}},
				next:        NewMockRoundTripper(gomock.NewController(t)), // No request may reach the next transport.
			}
			response, err := transport.RoundTrip(req)
			if response != nil && response.Body != nil {
				require.NoError(t, response.Body.Close())
			}
			require.Nil(t, response)
			if test.bodyFailure {
				require.ErrorIs(t, err, bodyFailure)
			} else {
				require.ErrorIs(t, err, ErrAWSCredentials)
			}
			require.Equal(t, test.authorization, req.Header.Get("Authorization"))
			require.Equal(t, "https://sts.us-east-1.amazonaws.com/action", req.URL.String())
		})
	}
}

// A profile that assumes a role from a source profile signs its STS call with the source credentials, which
// the transport never holds. Behind an endpoint path prefix, the call still succeeds and its signature covers
// the final path.
func TestAWSResolvesSourceProfileRoleChainBehindEndpointPathPrefix(t *testing.T) {
	server := newSTSServer(t)
	dir := t.TempDir()
	credentials, configFile := filepath.Join(dir, "credentials"), filepath.Join(dir, "config")
	require.NoError(t, os.WriteFile(credentials, []byte("[base]\naws_access_key_id = AKIABASE\naws_secret_access_key = base-secret\n"), 0o600))
	require.NoError(t, os.WriteFile(configFile, []byte("[profile chained]\nrole_arn = arn:aws:iam::123456789012:role/chained\nsource_profile = base\n"), 0o600))
	env := map[string]string{
		"AWS_PROFILE": "chained", "AWS_SHARED_CREDENTIALS_FILE": credentials, "AWS_CONFIG_FILE": configFile,
		"AWS_ENDPOINT_URL_STS": server.URL + "/prefix", "AWS_EC2_METADATA_DISABLED": "true",
	}

	_, err := accountID(t, AWS(t.Context(), Environment(env, io.Discard, io.Discard)))

	require.NoError(t, err)
	signers, actions := server.seen()
	assert.Equal(t, []string{"AssumeRole", "GetCallerIdentity"}, actions)
	assert.Equal(t, []string{"AKIABASE", "ASIAASSUMED"}, signers)
	server.mu.Lock()
	assumed, body := server.requests[0], server.bodies[0]
	server.mu.Unlock()
	assert.Equal(t, "/prefix/", assumed.URL.Path, "the prefix is applied once")
	signed := assumed.Header.Get("Authorization")
	replay, err := http.NewRequest(http.MethodPost, "https://"+assumed.Host+assumed.URL.RequestURI(), strings.NewReader(string(body)))
	require.NoError(t, err)
	replay.Header = assumed.Header.Clone()
	// The Go client adds Accept-Encoding after signing; the signature never covers it.
	replay.Header.Del("Accept-Encoding")
	replay.Header.Del("Authorization")
	stamp, err := time.Parse("20060102T150405Z", assumed.Header.Get("X-Amz-Date"))
	require.NoError(t, err)
	digest := sha256.Sum256(body)
	source := aws.Credentials{AccessKeyID: "AKIABASE", SecretAccessKey: "base-secret"}
	require.NoError(t, v4.NewSigner().SignHTTP(t.Context(), source, replay, hex.EncodeToString(digest[:]), "sts", "us-east-1", stamp))
	assert.Equal(t, signed, replay.Header.Get("Authorization"), "the source credentials signed the final path")
}

func TestAWSTransportFailureOnlyAffectsAWSHosts(t *testing.T) {
	cause := errors.New("boom")
	transport := &awsTransport{failure: cause}
	client := &http.Client{Transport: transport}
	local := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(local.Close)

	failed, err := client.Get("https://sts.us-east-1.amazonaws.com/")
	if failed != nil {
		_ = failed.Body.Close()
	}
	require.ErrorIs(t, err, ErrAWSCredentials)
	require.ErrorIs(t, err, cause)
	response, err := client.Get(local.URL)
	require.NoError(t, err)
	_ = response.Body.Close()
}

func TestAWSEndpointsNormalizeServiceNames(t *testing.T) {
	endpoints := awsEndpoints(map[string]string{
		"AWS_ENDPOINT_URL_SECRETS_MANAGER": "http://127.0.0.1:1", "AWS_ENDPOINT_URL_": "http://127.0.0.1:2", "AWS_ENDPOINT_URL_STS": "",
		"AWS_ENDPOINT_URL_KMS": "ftp://127.0.0.1:3", "OTHER": "http://127.0.0.1:4",
	})

	assert.Len(t, endpoints, 1)
	assert.Equal(t, "127.0.0.1:1", endpoints["secretsmanager"].Host)
}

func TestAWSCredentialCacheReusesUnexpiredEntriesOnly(t *testing.T) {
	// Each invocation gets its own namespace, so repeated runs (-count) never share cached entries.
	prefix := t.TempDir()
	loads := 0
	loader := func(expires time.Time) func() (aws.Credentials, error) {
		return func() (aws.Credentials, error) {
			loads++
			return aws.Credentials{AccessKeyID: fmt.Sprintf("AKIA%d", loads), CanExpire: !expires.IsZero(), Expires: expires}, nil
		}
	}

	first, err := cachedCredentials(prefix+"/static", loader(time.Time{}))
	require.NoError(t, err)
	second, err := cachedCredentials(prefix+"/static", loader(time.Time{}))
	require.NoError(t, err)
	assert.Equal(t, first, second, "credentials without an expiry are loaded once")
	assert.Equal(t, 1, loads)

	_, err = cachedCredentials(prefix+"/expiring", loader(time.Now().Add(time.Second)))
	require.NoError(t, err)
	refreshed, err := cachedCredentials(prefix+"/expiring", loader(time.Now().Add(time.Hour)))
	require.NoError(t, err)
	assert.Equal(t, "AKIA3", refreshed.AccessKeyID, "credentials inside the expiry margin are reloaded")

	_, err = cachedCredentials(prefix+"/failing", func() (aws.Credentials, error) { return aws.Credentials{}, errors.New("denied") })
	require.Error(t, err)
	_, found := awsCache.entries[prefix+"/failing"]
	assert.False(t, found, "a failure is never cached")
}

func TestAWSCacheKeyTracksSelectionAndFileChanges(t *testing.T) {
	credentials, configFile := writeProfile(t, "cached", "AKIACACHED")
	env := map[string]string{"AWS_PROFILE": "cached", "AWS_SHARED_CREDENTIALS_FILE": credentials, "AWS_CONFIG_FILE": configFile}
	key := awsCacheKey(env)

	assert.Equal(t, key, awsCacheKey(maps.Clone(env)))
	other := maps.Clone(env)
	other["AWS_PROFILE"] = "another"
	assert.NotEqual(t, key, awsCacheKey(other))
	require.NoError(t, os.WriteFile(credentials, []byte("[cached]\naws_access_key_id = AKIAROTATED\naws_secret_access_key = rotated-secret\n"), 0o600))
	assert.NotEqual(t, key, awsCacheKey(env), "rewriting the credentials file invalidates the cache entry")
}

func TestAWSWebIdentityRoleFromInvocationTokenFile(t *testing.T) {
	var ambientCalls atomic.Int32
	ambient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ambientCalls.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ambient.Close()
	t.Setenv("AWS_ENDPOINT_URL_STS", ambient.URL)
	var mu sync.Mutex
	var gotAction string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotAction = string(body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprintf(w, `<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleWithWebIdentityResult><Credentials><AccessKeyId>ASIAWEB</AccessKeyId><SecretAccessKey>web</SecretAccessKey><SessionToken>web-token</SessionToken><Expiration>%s</Expiration></Credentials></AssumeRoleWithWebIdentityResult></AssumeRoleWithWebIdentityResponse>`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	}))
	t.Cleanup(server.Close)
	token := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(token, []byte("jwt"), 0o600))
	env := map[string]string{
		"AWS_WEB_IDENTITY_TOKEN_FILE": token, "AWS_ROLE_ARN": "arn:aws:iam::123456789012:role/web", "AWS_ROLE_SESSION_NAME": "atmos-web",
		"AWS_ENDPOINT_URL_STS": server.URL, "AWS_EC2_METADATA_DISABLED": "true",
	}

	scoped := AWS(t.Context(), Environment(env, io.Discard, io.Discard))

	assert.Equal(t, "ASIAWEB", scoped.Env["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, "web-token", scoped.Env["AWS_SESSION_TOKEN"])
	mu.Lock()
	action := gotAction
	mu.Unlock()
	assert.Contains(t, action, "AssumeRoleWithWebIdentity")
	assert.Contains(t, action, "RoleSessionName=atmos-web")
	assert.Zero(t, ambientCalls.Load(), "the process STS endpoint must never receive the invocation token")
}

// The web identity session duration is bounded before it narrows to the 32 bits of the STS request: an unset
// value takes the default, and anything STS would refuse is rejected instead of being truncated.
func TestWebIdentityDurationIsBoundedBeforeNarrowing(t *testing.T) {
	tests := []struct {
		name    string
		seconds int64
		want    int32
		wantErr bool
	}{
		{name: "unset takes the default", seconds: 0, want: 3600},
		{name: "negative takes the default", seconds: -5, want: 3600},
		{name: "lowest accepted", seconds: 900, want: 900},
		{name: "highest accepted", seconds: 43200, want: 43200},
		{name: "below the lowest", seconds: 899, wantErr: true},
		{name: "above the highest", seconds: 43201, wantErr: true},
		{name: "would wrap to a small int32", seconds: 1<<32 + 3600, wantErr: true},
		{name: "the largest int64", seconds: math.MaxInt64, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := webIdentityDuration(tt.seconds)

			if tt.wantErr {
				require.ErrorIs(t, err, ErrAWSCredentials)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// An assumed role session is reused only for the same assumption: a different or rotated web identity token,
// duration, or base secret assumes the role again, and no raw secret enters the key.
func TestAWSRoleCacheKeyTracksEveryAssumptionInput(t *testing.T) {
	env := map[string]string{"AWS_PROFILE": "base", envAccessKeyID: "AKIABASE", envSecretKey: "base-secret", envSessionToken: "base-session"}
	role := testRole("arn:aws:iam::123456789012:role/deploy", "token-one")
	key := roleCacheKey(env, role)

	assert.Equal(t, key, roleCacheKey(maps.Clone(env), role))
	assert.NotContains(t, key, "token-one")
	assert.NotEqual(t, key, roleCacheKey(env, testRole(role.RoleARN, "token-two")), "another token is validated again")
	longer := role
	longer.AssumeRoleDuration = 7200
	assert.NotEqual(t, key, roleCacheKey(env, longer))
	rotated := maps.Clone(env)
	rotated[envSecretKey] = "rotated-secret"
	assert.NotEqual(t, key, roleCacheKey(rotated, role))

	tokenFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("jwt-one"), 0o600))
	fromFile := testRole(role.RoleARN, tokenFile)
	before := roleCacheKey(env, fromFile)
	require.NoError(t, os.WriteFile(tokenFile, []byte("jwt-rotated"), 0o600))
	assert.NotEqual(t, before, roleCacheKey(env, fromFile), "rewriting the token file invalidates the session")
}

func testRole(arn, token string) iam.RoleOptions {
	return iam.RoleOptions{RoleARN: arn, AssumeRoleSessionName: "atmos", WebIdentityToken: token}
}

func TestAWSS3PrefixPreservesSigningForEscapedKeys(t *testing.T) {
	keys := aws.Credentials{AccessKeyID: "AKIAS3", SecretAccessKey: "secret"}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/gateway/state with space.tfstate", r.URL.Path)
		assert.Contains(t, r.URL.EscapedPath(), "%20")
		signature := r.Header.Get("Authorization")
		replay := r.Clone(r.Context())
		replay.Header.Del("Authorization")
		stamp, err := time.Parse("20060102T150405Z", r.Header.Get("X-Amz-Date"))
		if !assert.NoError(t, err) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		err = v4.NewSigner().SignHTTP(r.Context(), keys, replay, r.Header.Get("X-Amz-Content-Sha256"), "s3", "us-east-1", stamp, func(o *v4.SignerOptions) { o.DisableURIPathEscaping = true })
		if !assert.NoError(t, err) || !assert.Equal(t, signature, replay.Header.Get("Authorization"), "gateway verifies S3 canonical path escaping") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = io.WriteString(w, "reviewed state")
	}))
	defer gateway.Close()
	endpoint, err := url.Parse(gateway.URL + "/gateway")
	require.NoError(t, err)
	transport := &awsTransport{credentials: keys, endpoints: map[string]*url.URL{"s3": endpoint}}
	client := s3.NewFromConfig(aws.Config{Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider(keys.AccessKeyID, keys.SecretAccessKey, ""), HTTPClient: &http.Client{Transport: transport}}, func(o *s3.Options) { o.RetryMaxAttempts = 1 })
	result, err := client.GetObject(t.Context(), &s3.GetObjectInput{Bucket: aws.String("bucket"), Key: aws.String("state with space.tfstate")})
	require.NoError(t, err)
	defer result.Body.Close()
	body, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	require.Equal(t, "reviewed state", string(body))
}

func TestAWSDefaultSharedFilesInvalidateCachedCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := config.DefaultSharedCredentialsFilename()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(config.DefaultSharedConfigFilename(), nil, 0o600))
	write := func(key string) {
		require.NoError(t, os.WriteFile(path, []byte("[rotating]\naws_access_key_id = "+key+"\naws_secret_access_key = secret\n"), 0o600))
	}
	env := map[string]string{"AWS_PROFILE": "rotating", "AWS_EC2_METADATA_DISABLED": "true"}
	write("AKIAORIGINAL")
	before := awsCacheKey(env)
	first := AWS(t.Context(), Environment(env, io.Discard, io.Discard))
	require.Equal(t, "AKIAORIGINAL", first.Env[envAccessKeyID])
	require.Equal(t, before, awsCacheKey(env), "unchanged files preserve the cache identity")
	again := AWS(t.Context(), Environment(env, io.Discard, io.Discard))
	require.Equal(t, first.Env[envAccessKeyID], again.Env[envAccessKeyID])
	write("AKIAREPLACEMENTLONGER")
	require.NotEqual(t, before, awsCacheKey(env))
	rotated := AWS(t.Context(), Environment(env, io.Discard, io.Discard))
	require.Equal(t, "AKIAREPLACEMENTLONGER", rotated.Env[envAccessKeyID])
}

func TestAWSRejectsAmbientCredentialSourceInRoleProfiles(t *testing.T) {
	var calls atomic.Int32
	ambient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"AccessKeyId":"AKIAAMBIENT","SecretAccessKey":"secret","Token":"ambient-token","Expiration":"2099-01-01T00:00:00Z"}`)
	}))
	defer ambient.Close()
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", ambient.URL)
	t.Setenv("AWS_CONTAINER_AUTHORIZATION_TOKEN", "ambient-authorization")
	server := newSTSServer(t)
	credentials, configFile := writeProfile(t, "source", "AKIAFILES")
	require.NoError(t, os.WriteFile(configFile, []byte("[profile role]\nrole_arn = arn:aws:iam::1:role/test\ncredential_source = EcsContainer\n"), 0o600))
	env := map[string]string{"AWS_PROFILE": "role", "AWS_SHARED_CREDENTIALS_FILE": credentials, "AWS_CONFIG_FILE": configFile, "AWS_ENDPOINT_URL_STS": server.URL, "AWS_EC2_METADATA_DISABLED": "true"}
	scoped := AWS(t.Context(), Environment(env, io.Discard, io.Discard))
	require.Equal(t, awsUnresolvedKey, scoped.Env[envAccessKeyID])
	_, err := accountID(t, scoped)
	require.ErrorIs(t, err, ErrAWSCredentials)
	require.ErrorContains(t, err, "credential_source")
	require.Zero(t, calls.Load(), "the process credential endpoint must never receive a request")
	signers, _ := server.seen()
	require.Empty(t, signers, "no role may be assumed with process credentials")
}

// envCredentialProcessMarker gates the test binary into acting as an AWS credential_process helper. It is
// only ever read by TestMain, so a helper started with the plugin process environment is observable.
const envCredentialProcessMarker = "_ATMOS_TEST_CREDENTIAL_PROCESS_MARKER"

func TestMain(m *testing.M) {
	if marker := os.Getenv(envCredentialProcessMarker); marker != "" {
		// Act as a credential_process helper: record that it ran, then print static credentials.
		_ = os.WriteFile(marker, []byte("ran"), 0o600)
		fmt.Fprint(os.Stdout, `{"Version":1,"AccessKeyId":"AKIAHELPER","SecretAccessKey":"helper-secret"}`)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// webIdentitySTS answers AssumeRoleWithWebIdentity and records the bodies it received.
func webIdentitySTS(t *testing.T, bodies *atomic.Value) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies.Store(string(body))
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprintf(w, `<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleWithWebIdentityResult><Credentials><AccessKeyId>ASIACOMMANDWEB</AccessKeyId><SecretAccessKey>web</SecretAccessKey><SessionToken>web-token</SessionToken><Expiration>%s</Expiration></Credentials></AssumeRoleWithWebIdentityResult></AssumeRoleWithWebIdentityResponse>`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	}))
	t.Cleanup(server.Close)
	return server
}

// A command-level web identity role is assumed with an invocation-scoped, unsigned STS call, so the
// invocation does not need base credentials. The plugin process STS endpoint never sees the token.
func TestAWSCommandLevelWebIdentityRoleNeedsNoBaseCredentials(t *testing.T) {
	var ambientCalls atomic.Int32
	ambient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		ambientCalls.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ambient.Close()
	t.Setenv("AWS_ENDPOINT_URL_STS", ambient.URL)
	var body atomic.Value
	server := webIdentitySTS(t, &body)
	token := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(token, []byte("command-jwt"), 0o600))
	env := map[string]string{
		"TG_IAM_ASSUME_ROLE": "arn:aws:iam::123456789012:role/deploy", "TG_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN": token,
		"TG_IAM_ASSUME_ROLE_SESSION_NAME": "atmos-command", "AWS_ENDPOINT_URL_STS": server.URL, "AWS_EC2_METADATA_DISABLED": "true",
	}

	scoped := AWS(t.Context(), Environment(env, io.Discard, io.Discard))

	assert.Equal(t, "ASIACOMMANDWEB", scoped.Env[envAccessKeyID])
	assert.Equal(t, "web-token", scoped.Env[envSessionToken])
	assert.Contains(t, body.Load(), "Action=AssumeRoleWithWebIdentity")
	assert.Contains(t, body.Load(), "WebIdentityToken=command-jwt")
	assert.Contains(t, body.Load(), "RoleSessionName=atmos-command")
	assert.Zero(t, ambientCalls.Load(), "the process STS endpoint must never receive the invocation token")
}

// Ordinary AssumeRole still needs base credentials: without them the role is not assumed and the call
// fails with the missing-credentials cause.
func TestAWSOrdinaryRoleStillRequiresBaseCredentials(t *testing.T) {
	server := newSTSServer(t)
	env := map[string]string{
		"TG_IAM_ASSUME_ROLE": "arn:aws:iam::123456789012:role/deploy", "AWS_ENDPOINT_URL_STS": server.URL, "AWS_EC2_METADATA_DISABLED": "true",
	}

	scoped := AWS(t.Context(), Environment(env, io.Discard, io.Discard))
	_, err := accountID(t, scoped)

	require.ErrorIs(t, err, ErrAWSCredentials)
	require.ErrorContains(t, err, "supplies no AWS credentials")
	assert.Equal(t, awsUnresolvedKey, scoped.Env[envAccessKeyID])
	signers, _ := server.seen()
	assert.Empty(t, signers)
}

// credential_process helpers run with the plugin process environment in the pinned SDK, which has no seam for
// an invocation environment. Such a profile is rejected, at the end of a source_profile chain too, and the
// helper never starts.
func TestAWSRejectsCredentialProcessProfiles(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	marker := filepath.Join(t.TempDir(), "ran")
	t.Setenv(envCredentialProcessMarker, marker)
	server := newSTSServer(t)
	credentials, configFile := writeProfile(t, "static", "AKIASTATIC")
	require.NoError(t, os.WriteFile(configFile, []byte(fmt.Sprintf("[profile helper]\ncredential_process = %q\n[profile viahelper]\nrole_arn = arn:aws:iam::1:role/test\nsource_profile = helper\n", executable)), 0o600))
	base := map[string]string{"AWS_SHARED_CREDENTIALS_FILE": credentials, "AWS_CONFIG_FILE": configFile, "AWS_ENDPOINT_URL_STS": server.URL, "AWS_EC2_METADATA_DISABLED": "true"}

	for _, profile := range []string{"helper", "viahelper"} {
		t.Run(profile, func(t *testing.T) {
			env := map[string]string{"AWS_PROFILE": profile}
			for name, value := range base {
				env[name] = value
			}
			scoped := AWS(t.Context(), Environment(env, io.Discard, io.Discard))
			require.Equal(t, awsUnresolvedKey, scoped.Env[envAccessKeyID])
			_, err := accountID(t, scoped)
			require.ErrorIs(t, err, ErrAWSCredentials)
			require.ErrorContains(t, err, "credential_process")
			require.NoFileExists(t, marker, "the helper must never run with the plugin process environment")
		})
	}

	t.Run("static profiles still resolve", func(t *testing.T) {
		env := map[string]string{"AWS_PROFILE": "static"}
		for name, value := range base {
			env[name] = value
		}
		scoped := AWS(t.Context(), Environment(env, io.Discard, io.Discard))
		assert.Equal(t, "AKIASTATIC", scoped.Env[envAccessKeyID])
	})
}

// The profile that passed validation is the one that resolves: rewriting the shared files in between must not
// let the SDK select an ambient provider of the plugin process.
func TestAWSResolvesTheValidatedProfileSnapshot(t *testing.T) {
	var ambientCalls atomic.Int32
	ambient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		ambientCalls.Add(1)
		_, _ = io.WriteString(w, `{"AccessKeyId":"AKIAAMBIENT","SecretAccessKey":"secret","Token":"ambient-token","Expiration":"2099-01-01T00:00:00Z"}`)
	}))
	defer ambient.Close()
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", ambient.URL)
	t.Setenv("AWS_CONTAINER_AUTHORIZATION_TOKEN", "ambient-authorization")
	credentials, configFile := writeProfile(t, "racy", "AKIARACY")
	env := map[string]string{"AWS_PROFILE": "racy", "AWS_SHARED_CREDENTIALS_FILE": credentials, "AWS_CONFIG_FILE": configFile, "AWS_EC2_METADATA_DISABLED": "true"}
	previous := afterProfileValidated
	t.Cleanup(func() { afterProfileValidated = previous })
	afterProfileValidated = func() {
		require.NoError(t, os.WriteFile(credentials, []byte("[unrelated]\naws_access_key_id = AKIAOTHER\naws_secret_access_key = other\n"), 0o600))
		require.NoError(t, os.WriteFile(configFile, []byte("[profile racy]\nregion = eu-west-1\n"), 0o600))
	}

	scoped := AWS(t.Context(), Environment(env, io.Discard, io.Discard))

	assert.Equal(t, "AKIARACY", scoped.Env[envAccessKeyID], "the validated snapshot resolves, not the rewritten files")
	assert.Zero(t, ambientCalls.Load(), "the process credential endpoint must never receive a request")
}

// Without a rewrite the snapshot changes nothing, and missing shared files are still reported by the profile lookup.
func TestAWSSnapshotKeepsMissingFilesMissing(t *testing.T) {
	// An isolated temporary root keeps other tests and processes out of the cleanup assertion. os.TempDir reads
	// TMPDIR on Unix and TMP/TEMP on Windows, which is where os.MkdirTemp("") creates the snapshot directory.
	tempRoot := t.TempDir()
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, tempRoot)
	}
	pattern := filepath.Join(tempRoot, "atmos-aws-snapshot-*")
	dir := t.TempDir()
	env := map[string]string{"AWS_PROFILE": "absent", "AWS_SHARED_CREDENTIALS_FILE": filepath.Join(dir, "none"), "AWS_CONFIG_FILE": filepath.Join(dir, "none-config"), "AWS_EC2_METADATA_DISABLED": "true"}

	scoped := AWS(t.Context(), Environment(env, io.Discard, io.Discard))

	assert.Equal(t, awsUnresolvedKey, scoped.Env[envAccessKeyID])
	entries, err := filepath.Glob(pattern)
	require.NoError(t, err)
	assert.Empty(t, entries, "the snapshot directory is removed after resolution")
}

// Positive control for the cleanup assertion above: a resolving profile creates its snapshot directory inside the
// isolated temporary root, so an empty root afterwards proves the cleanup ran rather than that the root went unused.
func TestAWSSnapshotDirectoryLivesInTheIsolatedRootUntilResolved(t *testing.T) {
	tempRoot := t.TempDir()
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, tempRoot)
	}
	pattern := filepath.Join(tempRoot, "atmos-aws-snapshot-*")
	credentials, configFile := writeProfile(t, "resolves", "AKIARESOLVES")
	env := map[string]string{"AWS_PROFILE": "resolves", "AWS_SHARED_CREDENTIALS_FILE": credentials, "AWS_CONFIG_FILE": configFile, "AWS_EC2_METADATA_DISABLED": "true"}
	var during []string
	previous := afterProfileValidated
	t.Cleanup(func() { afterProfileValidated = previous })
	afterProfileValidated = func() {
		during, _ = filepath.Glob(pattern)
	}

	scoped := AWS(t.Context(), Environment(env, io.Discard, io.Discard))

	assert.Equal(t, "AKIARESOLVES", scoped.Env[envAccessKeyID])
	require.Len(t, during, 1, "the snapshot directory exists in the isolated root while the profile resolves")
	after, err := filepath.Glob(pattern)
	require.NoError(t, err)
	assert.Empty(t, after, "the snapshot directory is removed after resolution")
}

// ssoProfile writes a profile that resolves its credentials through SSO and the access token cache that
// profile reads. A session profile gets an expired token so refreshing it reaches the SSO OIDC endpoint, and
// a legacy profile gets a valid one so the call goes straight to the SSO portal.
func ssoProfile(t *testing.T, session bool) (credentialsFile, configFile string) {
	t.Helper()
	home, dir := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	credentialsFile, configFile = filepath.Join(dir, "credentials"), filepath.Join(dir, "config")
	require.NoError(t, os.WriteFile(credentialsFile, nil, 0o600))
	const startURL = "https://example.awsapps.com/start"
	cacheKey, expires, config := startURL, time.Now().Add(time.Hour), "[profile sso]\nsso_account_id = 123456789012\nsso_role_name = Admin\nsso_region = eu-west-1\nsso_start_url = "+startURL+"\n"
	token := map[string]string{"accessToken": "cached-token", "startUrl": startURL, "region": "eu-west-1"}
	if session {
		cacheKey, expires = "team", time.Now().Add(-time.Hour)
		config = "[profile sso]\nsso_account_id = 123456789012\nsso_role_name = Admin\nsso_session = team\n[sso-session team]\nsso_region = eu-west-1\nsso_start_url = " + startURL + "\n"
		token["refreshToken"], token["clientId"], token["clientSecret"] = "refresh", "client", "client-secret"
		token["registrationExpiresAt"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	}
	token["expiresAt"] = expires.UTC().Format(time.RFC3339)
	digest := sha1.Sum([]byte(cacheKey))
	cache := filepath.Join(home, ".aws", "sso", "cache")
	require.NoError(t, os.MkdirAll(cache, 0o700))
	payload, err := json.Marshal(token)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cache, hex.EncodeToString(digest[:])+".json"), payload, 0o600))
	require.NoError(t, os.WriteFile(configFile, []byte(config), 0o600))
	return credentialsFile, configFile
}

// recordingServer answers every request with a forbidden status and counts the requests.
func recordingServer(t *testing.T) (server *httptest.Server, hosts func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Host+r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)
	return server, func() []string { mu.Lock(); defer mu.Unlock(); return slices.Clone(seen) }
}

func TestAWSSSORequestsIgnoreProcessEndpointsAndUseInvocationOverrides(t *testing.T) {
	for _, session := range []bool{false, true} {
		name, service := "legacy sso", "SSO"
		if session {
			name, service = "sso session", "SSO_OIDC"
		}
		t.Run(name, func(t *testing.T) {
			rogue, rogueHits := recordingServer(t)
			approved, approvedHits := recordingServer(t)
			for _, variable := range []string{"AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_SSO", "AWS_ENDPOINT_URL_SSO_OIDC"} {
				t.Setenv(variable, rogue.URL)
			}
			credentials, configFile := ssoProfile(t, session)
			env := map[string]string{
				"AWS_PROFILE": "sso", "AWS_SHARED_CREDENTIALS_FILE": credentials, "AWS_CONFIG_FILE": configFile,
				"AWS_ENDPOINT_URL_" + service: approved.URL, "AWS_EC2_METADATA_DISABLED": "true",
			}

			_, err := resolveAWSCredentials(t.Context(), env, &awsTransport{endpoints: awsEndpoints(env)})

			require.Error(t, err, "the approved endpoint refuses the request")
			assert.Empty(t, rogueHits(), "a process endpoint never receives the bearer token request")
			assert.NotEmpty(t, approvedHits(), "the invocation's own endpoint is used")
		})
	}
}

func TestAWSSSORequestsDefaultToAWSHostsWithoutOverrides(t *testing.T) {
	for _, session := range []bool{false, true} {
		name, host := "legacy sso", "portal.sso.eu-west-1.amazonaws.com"
		if session {
			name, host = "sso session", "oidc.eu-west-1.amazonaws.com"
		}
		t.Run(name, func(t *testing.T) {
			rogue, rogueHits := recordingServer(t)
			for _, variable := range []string{"AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_SSO", "AWS_ENDPOINT_URL_SSO_OIDC"} {
				t.Setenv(variable, rogue.URL)
			}
			credentials, configFile := ssoProfile(t, session)
			env := map[string]string{"AWS_PROFILE": "sso", "AWS_SHARED_CREDENTIALS_FILE": credentials, "AWS_CONFIG_FILE": configFile, "AWS_EC2_METADATA_DISABLED": "true"}
			var sent []string
			var mu sync.Mutex
			next := NewMockRoundTripper(gomock.NewController(t))
			next.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
				mu.Lock()
				sent = append(sent, req.URL.Hostname())
				mu.Unlock()
				return &http.Response{StatusCode: http.StatusForbidden, Body: http.NoBody, Header: http.Header{}, Request: req}, nil
			}).MinTimes(1)

			_, err := resolveAWSCredentials(t.Context(), env, &awsTransport{next: next})

			require.Error(t, err)
			assert.Empty(t, rogueHits(), "a process endpoint never receives the request")
			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, host, sent[0], "the request reaches the default AWS host")
		})
	}
}

func TestPinSSOEndpointAllowsOnlyTheCanonicalRegionalHost(t *testing.T) {
	const getRole, createToken = "GetRoleCredentials", "CreateToken"
	tests := []struct {
		name, service, operation, region, url string
		wantURL                               string
		wantErr                               bool
	}{
		{name: "canonical portal host is unchanged", service: "SSO", operation: getRole, region: "eu-west-1", url: "https://portal.sso.eu-west-1.amazonaws.com/federation/credentials", wantURL: "https://portal.sso.eu-west-1.amazonaws.com/federation/credentials"},
		{name: "canonical oidc host is unchanged", service: "SSO OIDC", operation: createToken, region: "eu-west-1", url: "https://oidc.eu-west-1.amazonaws.com/token", wantURL: "https://oidc.eu-west-1.amazonaws.com/token"},
		{name: "attacker host with an AWS suffix", service: "SSO", operation: getRole, region: "eu-west-1", url: "https://evil.example.amazonaws.com/x", wantURL: "https://portal.sso.eu-west-1.amazonaws.com/federation/credentials"},
		{name: "another AWS service host", service: "SSO OIDC", operation: createToken, region: "eu-west-1", url: "https://s3.eu-west-1.amazonaws.com/token", wantURL: "https://oidc.eu-west-1.amazonaws.com/token"},
		{name: "another region's host", service: "SSO", operation: getRole, region: "eu-west-1", url: "https://portal.sso.us-east-1.amazonaws.com/federation/credentials", wantURL: "https://portal.sso.eu-west-1.amazonaws.com/federation/credentials"},
		{name: "plain http on the canonical host", service: "SSO", operation: getRole, region: "eu-west-1", url: "http://portal.sso.eu-west-1.amazonaws.com/federation/credentials", wantURL: "https://portal.sso.eu-west-1.amazonaws.com/federation/credentials"},
		{name: "china region domain", service: "SSO", operation: getRole, region: "cn-north-1", url: "https://evil.amazonaws.com/x", wantURL: "https://portal.sso.cn-north-1.amazonaws.com.cn/federation/credentials"},
		{name: "unknown operation fails closed", service: "SSO", operation: "ListAccounts", region: "eu-west-1", url: "https://portal.sso.eu-west-1.amazonaws.com/assignment/accounts", wantErr: true},
		{name: "missing region fails closed", service: "SSO OIDC", operation: createToken, region: "", url: "https://oidc.eu-west-1.amazonaws.com/token", wantErr: true},
		{name: "non-SSO request is untouched", service: "STS", operation: "AssumeRole", region: "eu-west-1", url: "https://evil.example.amazonaws.com/x", wantURL: "https://evil.example.amazonaws.com/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := smithyhttp.NewStackRequest().(*smithyhttp.Request)
			parsed, err := url.Parse(tt.url)
			require.NoError(t, err)
			req.URL = parsed
			ctx := awsmiddleware.SetServiceID(t.Context(), tt.service)
			ctx = awsmiddleware.SetOperationName(ctx, tt.operation)
			ctx = awsmiddleware.SetRegion(ctx, tt.region)

			err = pinSSOEndpoint(ctx, req)

			if tt.wantErr {
				require.ErrorIs(t, err, ErrAWSCredentials)
				assert.Equal(t, tt.url, req.URL.String(), "a failed request is left as it was")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantURL, req.URL.String())
		})
	}
}
