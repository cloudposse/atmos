package aws

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// poisonProcessEnvironment fills the process environment with a credential source of every kind. None may
// reach an invocation that does not name it, and none may be changed by resolving one.
func poisonProcessEnvironment(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "process-token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("process-web-identity-token"), 0o600))
	credentialsFile := filepath.Join(dir, "credentials")
	require.NoError(t, os.WriteFile(credentialsFile,
		[]byte("[default]\naws_access_key_id = AKIAPROCESSDEFAULT\naws_secret_access_key = s\n[process]\naws_access_key_id = AKIAPROCESSPROFILE\naws_secret_access_key = s\n"), 0o600))
	poisoned := map[string]string{
		"AWS_ACCESS_KEY_ID":                  "AKIAPROCESSKEY",
		"AWS_SECRET_ACCESS_KEY":              "process-secret",
		"AWS_PROFILE":                        "process",
		"AWS_SHARED_CREDENTIALS_FILE":        credentialsFile,
		"AWS_CONFIG_FILE":                    filepath.Join(dir, "config"),
		"AWS_WEB_IDENTITY_TOKEN_FILE":        tokenFile,
		"AWS_ROLE_ARN":                       "arn:aws:iam::9:role/process",
		"AWS_CONTAINER_CREDENTIALS_FULL_URI": "http://127.0.0.1:1/process",
		"AWS_EC2_METADATA_DISABLED":          "true",
		"AWS_REGION":                         "eu-north-1",
	}
	for key, value := range poisoned {
		t.Setenv(key, value)
	}
	return poisoned
}

func assertProcessEnvironmentUnchanged(t *testing.T, poisoned map[string]string) {
	t.Helper()
	for key, value := range poisoned {
		assert.Equal(t, value, os.Getenv(key), "the process environment is not changed: %s", key)
	}
}

// homeWithProfiles makes a home directory whose default shared files hold the given contents.
func homeWithProfiles(t *testing.T, credentials, config string) string {
	t.Helper()
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".aws"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".aws", "credentials"), []byte(credentials), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".aws", "config"), []byte(config), 0o600))
	return home
}

func invocationHome(home string) []string {
	return []string{"HOME=" + home, "USERPROFILE=" + home}
}

// credentialServer serves container credentials, recording the authorization header of each request.
func credentialServer(t *testing.T, key string) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var authorizations []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authorizations = append(authorizations, r.Header.Get("Authorization"))
		mu.Unlock()
		require.NoError(t, json.NewEncoder(w).Encode(map[string]string{
			"AccessKeyId": key, "SecretAccessKey": "container-secret", "Token": "container-token", "Expiration": "2099-01-01T00:00:00Z",
		}))
	}))
	t.Cleanup(server.Close)
	return server, &authorizations
}

// metadataServer emulates the instance metadata role credentials API.
func metadataServer(t *testing.T, key string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/latest/api/token":
			w.Header().Set("X-Aws-Ec2-Metadata-Token-Ttl-Seconds", "21600")
			fmt.Fprint(w, "imds-session-token")
		case r.URL.Path == "/latest/meta-data/iam/security-credentials/":
			fmt.Fprint(w, "instance-role")
		case r.URL.Path == "/latest/meta-data/iam/security-credentials/instance-role":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]string{
				"Code": "Success", "Type": "AWS-HMAC", "AccessKeyId": key, "SecretAccessKey": "instance-secret",
				"Token": "instance-token", "Expiration": "2099-01-01T00:00:00Z", "LastUpdated": "2026-01-01T00:00:00Z",
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func resolveBase(t *testing.T, environ ...string) (string, string, error) {
	t.Helper()
	credentials, err := baseCredentials(context.Background(), environMap(environ))
	if err != nil {
		return "", "", err
	}
	return credentials.AccessKeyID, credentials.Region, nil
}

func TestBaseCredentialsComeFromTheInvocationProfileNotTheProcess(t *testing.T) {
	poisoned := poisonProcessEnvironment(t)
	home := homeWithProfiles(t, "[default]\naws_access_key_id = AKIAINVOCATIONDEFAULT\naws_secret_access_key = s\n", "[default]\nregion = us-west-2\n")

	key, region, err := resolveBase(t, invocationHome(home)...)
	require.NoError(t, err)
	assert.Equal(t, "AKIAINVOCATIONDEFAULT", key, "the invocation's default profile is used, not the process keys, profile, or files")
	assert.Equal(t, "us-west-2", region, "the profile region is used, not the process region")
	assertProcessEnvironmentUnchanged(t, poisoned)
}

func TestBaseCredentialsFromTheInvocationWebIdentity(t *testing.T) {
	poisoned := poisonProcessEnvironment(t)
	sts := newSTSRecorder(t)
	tokenFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("invocation-web-identity-token"), 0o600))

	key, _, err := resolveBase(t, "AWS_WEB_IDENTITY_TOKEN_FILE="+tokenFile, "AWS_ROLE_ARN=arn:aws:iam::1:role/irsa",
		"AWS_ROLE_SESSION_NAME=pod", "AWS_REGION=us-east-1", "AWS_ENDPOINT_URL_STS="+sts.server.URL, "HOME="+t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, assumedKey, key)
	form, _ := sts.last(t)
	assert.Equal(t, "AssumeRoleWithWebIdentity", form.Get("Action"))
	assert.Equal(t, "invocation-web-identity-token", form.Get("WebIdentityToken"))
	assert.Equal(t, "arn:aws:iam::1:role/irsa", form.Get("RoleArn"))
	assert.Equal(t, "pod", form.Get("RoleSessionName"))
	assertProcessEnvironmentUnchanged(t, poisoned)

	_, _, err = resolveBase(t, "AWS_WEB_IDENTITY_TOKEN_FILE="+tokenFile)
	require.ErrorIs(t, err, errNoInvocationCredentials, "a token file without a role is rejected")
}

func TestBaseCredentialsFromTheInvocationContainer(t *testing.T) {
	poisoned := poisonProcessEnvironment(t)
	server, authorizations := credentialServer(t, "AKIACONTAINER")
	empty := t.TempDir()

	key, _, err := resolveBase(t, "HOME="+empty, "AWS_CONTAINER_CREDENTIALS_FULL_URI="+server.URL+"/creds",
		"AWS_CONTAINER_AUTHORIZATION_TOKEN=inline-token")
	require.NoError(t, err)
	assert.Equal(t, "AKIACONTAINER", key)

	tokenFile := filepath.Join(empty, "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("file-token"), 0o600))
	_, _, err = resolveBase(t, "HOME="+empty, "AWS_CONTAINER_CREDENTIALS_FULL_URI="+server.URL+"/creds",
		"AWS_CONTAINER_AUTHORIZATION_TOKEN=inline-token", "AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE="+tokenFile)
	require.NoError(t, err)
	assert.Equal(t, []string{"inline-token", "file-token"}, *authorizations, "the token file named by the invocation overrides the inline token")
	assertProcessEnvironmentUnchanged(t, poisoned)

	_, _, err = resolveBase(t, "HOME="+empty, "AWS_CONTAINER_CREDENTIALS_FULL_URI=http://192.0.2.10/creds")
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig, "plain HTTP is accepted only on loopback and container hosts")
}

func TestBaseCredentialsFromTheInvocationInstanceMetadata(t *testing.T) {
	poisoned := poisonProcessEnvironment(t)
	server, requests := metadataServer(t, "AKIAINSTANCE")
	empty := t.TempDir()

	key, _, err := resolveBase(t, "HOME="+empty, "AWS_EC2_METADATA_SERVICE_ENDPOINT="+server.URL)
	require.NoError(t, err, "the process AWS_EC2_METADATA_DISABLED does not disable the invocation's metadata client")
	assert.Equal(t, "AKIAINSTANCE", key)

	before := requests.Load()
	_, _, err = resolveBase(t, "HOME="+empty, "AWS_EC2_METADATA_SERVICE_ENDPOINT="+server.URL, "AWS_EC2_METADATA_DISABLED=true")
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed, "the invocation disables the metadata client, and the process keys are not a fallback")
	assert.Equal(t, before, requests.Load(), "a disabled metadata client sends no request")
	assertProcessEnvironmentUnchanged(t, poisoned)
}

func TestBaseCredentialsFromAProfileCredentialSource(t *testing.T) {
	poisonProcessEnvironment(t)
	container, _ := credentialServer(t, "AKIACONTAINERSOURCE")
	sts := newSTSRecorder(t)
	home := homeWithProfiles(t, "", "[profile ecs]\nrole_arn = arn:aws:iam::1:role/from-ecs\ncredential_source = EcsContainer\nrole_session_name = ecs-hop\nexternal_id = ext\nduration_seconds = 3600\n"+
		"[profile env]\nrole_arn = arn:aws:iam::1:role/from-env\ncredential_source = Environment\n"+
		"[profile bogus]\nrole_arn = arn:aws:iam::1:role/r\ncredential_source = Nowhere\n")

	key, _, err := resolveBase(t, "HOME="+home, "AWS_PROFILE=ecs", "AWS_REGION=us-east-1", "AWS_ENDPOINT_URL_STS="+sts.server.URL,
		"AWS_CONTAINER_CREDENTIALS_FULL_URI="+container.URL)
	require.NoError(t, err)
	assert.Equal(t, assumedKey, key)
	form, authorization := sts.last(t)
	assert.Equal(t, "AssumeRole", form.Get("Action"))
	assert.Equal(t, "arn:aws:iam::1:role/from-ecs", form.Get("RoleArn"))
	assert.Equal(t, "ecs-hop", form.Get("RoleSessionName"))
	assert.Equal(t, "ext", form.Get("ExternalId"))
	assert.Equal(t, "3600", form.Get("DurationSeconds"))
	assert.Contains(t, authorization, "Credential=AKIACONTAINERSOURCE/", "the role is assumed with the invocation's container credentials")

	_, _, err = resolveBase(t, "HOME="+home, "AWS_PROFILE=ecs")
	require.ErrorIs(t, err, errNoInvocationCredentials, "the process container URI is not used")
	_, _, err = resolveBase(t, "HOME="+home, "AWS_PROFILE=env")
	require.ErrorIs(t, err, errNoInvocationCredentials, "the process keys are not the Environment source")
	_, _, err = resolveBase(t, "HOME="+home, "AWS_PROFILE=bogus")
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
}

func TestBaseCredentialsStayWithTheirInvocationUnderConcurrency(t *testing.T) {
	poisoned := poisonProcessEnvironment(t)
	const invocations = 16
	homes := make([]string, invocations)
	for i := range homes {
		homes[i] = homeWithProfiles(t, fmt.Sprintf("[default]\naws_access_key_id = AKIAINVOCATION%02d\naws_secret_access_key = s\n", i), "")
	}
	server, _ := metadataServer(t, "AKIAINSTANCE")

	var wg sync.WaitGroup
	keys := make([]string, invocations)
	errs := make([]error, invocations)
	for i := range invocations {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Odd invocations name a metadata endpoint too; the profile still comes first.
			environ := invocationHome(homes[i])
			if i%2 == 1 {
				environ = append(environ, "AWS_EC2_METADATA_SERVICE_ENDPOINT="+server.URL)
			}
			credentials, err := baseCredentials(context.Background(), environMap(environ))
			if err == nil {
				keys[i] = credentials.AccessKeyID
			}
			errs[i] = err
		}()
	}
	wg.Wait()
	for i := range invocations {
		require.NoError(t, errs[i])
		assert.Equal(t, fmt.Sprintf("AKIAINVOCATION%02d", i), keys[i])
	}
	assertProcessEnvironmentUnchanged(t, poisoned)
}

func TestInvocationSharedFiles(t *testing.T) {
	configFile, credentialsFile := invocationSharedFiles(map[string]string{"AWS_CONFIG_FILE": "c", "AWS_SHARED_CREDENTIALS_FILE": "k"})
	assert.Equal(t, "c", configFile)
	assert.Equal(t, "k", credentialsFile)
	home := t.TempDir()
	configFile, credentialsFile = invocationSharedFiles(map[string]string{"HOME": home, "USERPROFILE": home})
	assert.Equal(t, filepath.Join(home, ".aws", "config"), configFile)
	assert.Equal(t, filepath.Join(home, ".aws", "credentials"), credentialsFile)
}

func TestBaseCredentialsFromAProfileWebIdentityUseTheInvocationSTSEndpoint(t *testing.T) {
	poisonProcessEnvironment(t)
	processSTS := newSTSRecorder(t)
	t.Setenv("AWS_ENDPOINT_URL_STS", processSTS.server.URL)
	sts := newSTSRecorder(t)
	home := t.TempDir()
	tokenFile := filepath.Join(home, "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("profile-web-identity-token"), 0o600))
	home = homeWithProfiles(t, "", fmt.Sprintf("[profile oidc]\nrole_arn = arn:aws:iam::1:role/oidc\nweb_identity_token_file = %s\nregion = us-east-2\n", tokenFile))

	key, region, err := resolveBase(t, "HOME="+home, "USERPROFILE="+home, "AWS_PROFILE=oidc", "AWS_ENDPOINT_URL_STS="+sts.server.URL)
	require.NoError(t, err)
	assert.Equal(t, assumedKey, key)
	assert.Equal(t, "us-east-2", region)
	form, _ := sts.last(t)
	assert.Equal(t, "profile-web-identity-token", form.Get("WebIdentityToken"))
	processSTS.mu.Lock()
	defer processSTS.mu.Unlock()
	assert.Empty(t, processSTS.forms, "the process STS endpoint receives nothing")
}

func TestBaseCredentialsFromAStaticSourceProfileChainUseTheInvocationSTSEndpoint(t *testing.T) {
	poisonProcessEnvironment(t)
	processSTS := newSTSRecorder(t)
	t.Setenv("AWS_ENDPOINT_URL_STS", processSTS.server.URL)
	sts := newSTSRecorder(t)
	home := homeWithProfiles(t,
		"[base]\naws_access_key_id = AKIACHAINBASE\naws_secret_access_key = s\n",
		"[profile chain]\nrole_arn = arn:aws:iam::1:role/chain\nsource_profile = base\nregion = us-east-2\n")

	key, region, err := resolveBase(t, "HOME="+home, "USERPROFILE="+home, "AWS_PROFILE=chain", "AWS_ENDPOINT_URL_STS="+sts.server.URL)
	require.NoError(t, err)
	assert.Equal(t, assumedKey, key)
	assert.Equal(t, "us-east-2", region)
	form, _ := sts.last(t)
	assert.Equal(t, "arn:aws:iam::1:role/chain", form.Get("RoleArn"), "the profile role assumption reaches the invocation STS endpoint")
	processSTS.mu.Lock()
	defer processSTS.mu.Unlock()
	assert.Empty(t, processSTS.forms, "the process STS endpoint receives nothing")
}

func TestBaseCredentialsHonorAWSDefaultProfile(t *testing.T) {
	poisonProcessEnvironment(t)
	home := homeWithProfiles(t,
		"[default]\naws_access_key_id = AKIAPLAINDEFAULT\naws_secret_access_key = s\n[production]\naws_access_key_id = AKIAPRODUCTION\naws_secret_access_key = s\n", "")

	key, _, err := resolveBase(t, append(invocationHome(home), "AWS_DEFAULT_PROFILE=production")...)
	require.NoError(t, err)
	assert.Equal(t, "AKIAPRODUCTION", key, "AWS_DEFAULT_PROFILE selects the profile when AWS_PROFILE is unset")

	key, _, err = resolveBase(t, append(invocationHome(home), "AWS_DEFAULT_PROFILE=production", "AWS_PROFILE=default")...)
	require.NoError(t, err)
	assert.Equal(t, "AKIAPLAINDEFAULT", key, "AWS_PROFILE takes precedence over AWS_DEFAULT_PROFILE")

	_, _, err = resolveBase(t, append(invocationHome(home), "AWS_DEFAULT_PROFILE=missing")...)
	require.Error(t, err, "a missing profile named by AWS_DEFAULT_PROFILE is an error, not a fallback")
}

// brokerCommand returns a credential_process value that runs the test binary as the fake broker.
func brokerCommand(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	if runtime.GOOS == "windows" {
		if strings.ContainsAny(exe, " \t") {
			return `"` + exe + `"`
		}
		return exe
	}
	return `'` + strings.ReplaceAll(exe, `'`, `'\''`) + `'`
}

// A credential_process broker runs with the invocation environment, so a selector such as BROKER_PROFILE
// comes from the invocation and never from the process.
func TestBaseCredentialsFromAProfileCredentialProcessRunWithTheInvocationEnvironment(t *testing.T) {
	poisonProcessEnvironment(t)
	t.Setenv("BROKER_PROFILE", "development")
	// The process also acts as a broker, so a broker that wrongly inherits the process environment answers
	// with the process value instead of re-running the whole test binary.
	t.Setenv(testCredentialBrokerEnv, "1")
	home := homeWithProfiles(t, "", "[profile brokered]\ncredential_process = "+brokerCommand(t)+"\n")
	invocation := append(invocationHome(home), "AWS_PROFILE=brokered", testCredentialBrokerEnv+"=1", "BROKER_PROFILE=production")
	if root := os.Getenv("SystemRoot"); root != "" {
		invocation = append(invocation, "SystemRoot="+root)
	}

	key, _, err := resolveBase(t, invocation...)
	require.NoError(t, err)
	assert.Equal(t, "AKIABROKERproduction", key, "the broker sees the invocation's BROKER_PROFILE, not the process value")

	sts := newSTSRecorder(t)
	chained := homeWithProfiles(t, "", "[profile brokered]\ncredential_process = "+brokerCommand(t)+"\n[profile deploy]\nrole_arn = arn:aws:iam::1:role/deploy\nsource_profile = brokered\n")
	_, _, err = resolveBase(t, append(invocationHome(chained), "AWS_PROFILE=deploy", "AWS_REGION=us-east-1", "AWS_ENDPOINT_URL_STS="+sts.server.URL,
		testCredentialBrokerEnv+"=1", "BROKER_PROFILE=staging")...)
	require.NoError(t, err)
	_, authorization := sts.last(t)
	assert.Contains(t, authorization, "Credential=AKIABROKERstaging/", "a role assumed from a broker profile is signed with the invocation's broker credentials")
}

// ssoTokenCache writes a cached SSO token for the key under the home directory.
func ssoTokenCache(t *testing.T, home, key, token string) {
	t.Helper()
	sum := sha1.Sum([]byte(key))
	dir := filepath.Join(home, ".aws", "sso", "cache")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	contents, err := json.Marshal(map[string]string{"accessToken": token, "expiresAt": "2099-01-01T00:00:00Z"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, hex.EncodeToString(sum[:])+".json"), contents, 0o600))
}

// ssoPortal serves the SSO role credentials API and records the bearer token of each request.
func ssoPortal(t *testing.T) *[]string {
	t.Helper()
	var mu sync.Mutex
	var tokens []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		tokens = append(tokens, r.Header.Get("X-Amz-Sso_bearer_token"))
		mu.Unlock()
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"roleCredentials": map[string]any{
			"accessKeyId": "ASIASSOROLE", "secretAccessKey": "sso-secret", "sessionToken": "sso-token", "expiration": int64(4070908800000),
		}}))
	}))
	t.Cleanup(server.Close)
	t.Setenv("AWS_ENDPOINT_URL_SSO", server.URL)
	return &tokens
}

// An SSO profile reads its cached login from the invocation home, in both profile forms, and never from the
// home of the process.
func TestBaseCredentialsFromAnSSOProfileReadTheInvocationTokenCache(t *testing.T) {
	const startURL = "https://corp.awsapps.com/start"
	forms := map[string]struct{ config, cacheKey string }{
		"legacy": {
			config:   "[profile sso]\nsso_start_url = " + startURL + "\nsso_region = us-east-1\nsso_account_id = 111122223333\nsso_role_name = Admin\n",
			cacheKey: startURL,
		},
		"session": {
			config: "[profile sso]\nsso_session = corp\nsso_account_id = 111122223333\nsso_role_name = Admin\n" +
				"[sso-session corp]\nsso_start_url = " + startURL + "\nsso_region = us-east-1\n",
			cacheKey: "corp",
		},
	}
	for name, form := range forms {
		t.Run(name, func(t *testing.T) {
			tokens := ssoPortal(t)
			processHome := t.TempDir()
			t.Setenv("HOME", processHome)
			t.Setenv("USERPROFILE", processHome)
			ssoTokenCache(t, processHome, form.cacheKey, "process-login")
			home := homeWithProfiles(t, "", form.config)
			invocation := append(invocationHome(home), "AWS_PROFILE=sso")

			_, _, err := resolveBase(t, invocation...)
			require.Error(t, err, "an invocation without a cached login fails instead of using the process login")
			assert.Empty(t, *tokens, "the process login is never sent")

			ssoTokenCache(t, home, form.cacheKey, "invocation-login")
			key, _, err := resolveBase(t, invocation...)
			require.NoError(t, err)
			assert.Equal(t, "ASIASSOROLE", key)
			assert.Equal(t, []string{"invocation-login"}, *tokens, "the invocation's cached login is used")
		})
	}
}

// metadataServerWithoutTokens emulates an instance whose token endpoint refuses requests, which is when
// the IMDSv1 fallback applies, and counts the credential reads that arrive without a session token.
func metadataServerWithoutTokens(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var v1Reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/latest/api/token":
			http.Error(w, "forbidden", http.StatusForbidden)
		case r.URL.Path == "/latest/meta-data/iam/security-credentials/":
			v1Reads.Add(1)
			fmt.Fprint(w, "instance-role")
		case r.URL.Path == "/latest/meta-data/iam/security-credentials/instance-role":
			v1Reads.Add(1)
			require.NoError(t, json.NewEncoder(w).Encode(map[string]string{
				"Code": "Success", "Type": "AWS-HMAC", "AccessKeyId": "AKIAIMDSV1", "SecretAccessKey": "instance-secret",
				"Token": "instance-token", "Expiration": "2099-01-01T00:00:00Z", "LastUpdated": "2026-01-01T00:00:00Z",
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, &v1Reads
}

// The restriction of a profile reaches the metadata client, and an invocation environment variable decides
// over it, as it does in the SDK.
func TestBaseCredentialsFromAProfileCredentialSourceHonorTheIMDSv1Restriction(t *testing.T) {
	poisonProcessEnvironment(t)
	sts := newSTSRecorder(t)
	metadata, v1Reads := metadataServerWithoutTokens(t)
	profile := func(restriction string) string {
		return "[profile imds]\nrole_arn = arn:aws:iam::1:role/from-imds\ncredential_source = Ec2InstanceMetadata\n" + restriction
	}
	invocation := func(home string, extra ...string) []string {
		return append(append(invocationHome(home), "AWS_PROFILE=imds", "AWS_REGION=us-east-1",
			"AWS_ENDPOINT_URL_STS="+sts.server.URL, "AWS_EC2_METADATA_SERVICE_ENDPOINT="+metadata.URL), extra...)
	}

	restricted := homeWithProfiles(t, "", profile("ec2_metadata_v1_disabled = true\n"))
	_, _, err := resolveBase(t, invocation(restricted)...)
	require.Error(t, err, "a failed token request does not fall back to IMDSv1 when only the profile restricts it")
	assert.Zero(t, v1Reads.Load(), "no credentials are read without a session token")

	_, _, err = resolveBase(t, invocation(restricted, "AWS_EC2_METADATA_V1_DISABLED=false")...)
	require.NoError(t, err, "the environment variable overrides the profile")
	assert.Positive(t, v1Reads.Load())

	unrestricted := homeWithProfiles(t, "", profile(""))
	before := v1Reads.Load()
	_, _, err = resolveBase(t, invocation(unrestricted)...)
	require.NoError(t, err, "a profile without the restriction keeps the fallback")
	assert.Greater(t, v1Reads.Load(), before)

	before = v1Reads.Load()
	_, _, err = resolveBase(t, invocation(unrestricted, "AWS_EC2_METADATA_V1_DISABLED=true")...)
	require.Error(t, err, "the environment variable restricts a profile that does not")
	assert.Equal(t, before, v1Reads.Load())
}
