package atmosbridge

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAccount = "123456789012"

// stsServer answers the two STS actions Terragrunt issues and records who signed each request.
type stsServer struct {
	*httptest.Server
	mu      sync.Mutex
	signers []string
	actions []string
	hosts   []string
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

func TestAWSLeavesEnvironmentWithoutSelectorsAlone(t *testing.T) {
	base := Environment(map[string]string{"PATH": "/bin"}, io.Discard, io.Discard)

	scoped := AWS(t.Context(), base)

	assert.Equal(t, base.Env, scoped.Env)
	assert.Same(t, base.HTTP, scoped.HTTP, "the HTTP client is replaced only when an override or failure needs it")
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

type captureTransport func(*http.Request) (*http.Response, error)

func (f captureTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

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
	transport := &awsTransport{
		credentials: credentials,
		endpoints:   map[string]*url.URL{"sts": {Scheme: "https", Host: "proxy.example", Path: "/aws/sts"}},
		next: captureTransport(func(r *http.Request) (*http.Response, error) {
			sent = r
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		}),
	}
	_, err = transport.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, "/aws/sts/action", sent.URL.Path)
	require.Equal(t, "version=1", sent.URL.RawQuery)
	require.Equal(t, "sts.us-east-1.amazonaws.com", sent.Host)
	require.NotEqual(t, oldSignature, sent.Header.Get("Authorization"))
	actualSignature := sent.Header.Get("Authorization")
	sent.Header.Del("Authorization")
	require.NoError(t, v4.NewSigner().SignHTTP(t.Context(), credentials, sent, hash, "sts", "us-east-1", stamp))
	require.Equal(t, actualSignature, sent.Header.Get("Authorization"), "the signature covers the final path")
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
	loads := 0
	loader := func(expires time.Time) func() (aws.Credentials, error) {
		return func() (aws.Credentials, error) {
			loads++
			return aws.Credentials{AccessKeyID: fmt.Sprintf("AKIA%d", loads), CanExpire: !expires.IsZero(), Expires: expires}, nil
		}
	}

	first, err := cachedCredentials(t.Name()+"/static", loader(time.Time{}))
	require.NoError(t, err)
	second, err := cachedCredentials(t.Name()+"/static", loader(time.Time{}))
	require.NoError(t, err)
	assert.Equal(t, first, second, "credentials without an expiry are loaded once")
	assert.Equal(t, 1, loads)

	_, err = cachedCredentials(t.Name()+"/expiring", loader(time.Now().Add(time.Second)))
	require.NoError(t, err)
	refreshed, err := cachedCredentials(t.Name()+"/expiring", loader(time.Now().Add(time.Hour)))
	require.NoError(t, err)
	assert.Equal(t, "AKIA3", refreshed.AccessKeyID, "credentials inside the expiry margin are reloaded")

	_, err = cachedCredentials(t.Name()+"/failing", func() (aws.Credentials, error) { return aws.Credentials{}, errors.New("denied") })
	require.Error(t, err)
	_, found := awsCache.entries[t.Name()+"/failing"]
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
}
