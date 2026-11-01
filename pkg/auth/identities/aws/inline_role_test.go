package aws

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// stsRecorder is an STS endpoint that accepts AssumeRole and AssumeRoleWithWebIdentity and records them.
type stsRecorder struct {
	mu            sync.Mutex
	server        *httptest.Server
	forms         []url.Values
	authorization []string
}

const (
	assumedKey     = "ASIAASSUMEDKEY"
	assumedSecret  = "assumed-secret-value"
	assumedSession = "assumed-session-token"
)

func newSTSRecorder(t *testing.T) *stsRecorder {
	t.Helper()
	recorder := &stsRecorder{}
	recorder.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		recorder.mu.Lock()
		recorder.forms = append(recorder.forms, r.PostForm)
		recorder.authorization = append(recorder.authorization, r.Header.Get("Authorization"))
		recorder.mu.Unlock()
		action := r.PostForm.Get("Action")
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<%[1]sResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><%[1]sResult><Credentials>
<AccessKeyId>%[2]s</AccessKeyId><SecretAccessKey>%[3]s</SecretAccessKey><SessionToken>%[4]s</SessionToken>
<Expiration>2099-01-01T00:00:00Z</Expiration></Credentials></%[1]sResult><ResponseMetadata><RequestId>r</RequestId></ResponseMetadata></%[1]sResponse>`,
			action, assumedKey, assumedSecret, assumedSession)
	}))
	t.Cleanup(recorder.server.Close)
	return recorder
}

func (r *stsRecorder) last(t *testing.T) (url.Values, string) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.NotEmpty(t, r.forms, "STS received no request")
	return r.forms[len(r.forms)-1], r.authorization[len(r.authorization)-1]
}

func TestAssumeInlineRoleLayersOnStaticCredentials(t *testing.T) {
	sts := newSTSRecorder(t)
	env, err := AssumeInlineRole(context.Background(),
		InlineRole{Role: "arn:aws:iam::123456789012:role/deploy", SessionName: "terragrunt-test", Duration: 2 * time.Hour},
		[]string{"AWS_ACCESS_KEY_ID=AKIABASEKEY", "AWS_SECRET_ACCESS_KEY=base-secret", "AWS_REGION=eu-west-1", "AWS_ENDPOINT_URL_STS=" + sts.server.URL, "AWS_PROFILE=ambient"})
	require.NoError(t, err)

	form, authorization := sts.last(t)
	assert.Equal(t, "AssumeRole", form.Get("Action"))
	assert.Equal(t, "arn:aws:iam::123456789012:role/deploy", form.Get("RoleArn"))
	assert.Equal(t, "terragrunt-test", form.Get("RoleSessionName"))
	assert.Equal(t, "7200", form.Get("DurationSeconds"))
	assert.Contains(t, authorization, "Credential=AKIABASEKEY/", "the request is signed by the invocation credentials")

	assert.Equal(t, map[string]string{
		"AWS_ACCESS_KEY_ID": assumedKey, "AWS_SECRET_ACCESS_KEY": assumedSecret, "AWS_SESSION_TOKEN": assumedSession,
		"AWS_SECURITY_TOKEN": assumedSession, "AWS_PROFILE": "",
	}, env)
}

func TestAssumeInlineRoleUsesProfileFilesFromTheEnvironment(t *testing.T) {
	sts := newSTSRecorder(t)
	dir := t.TempDir()
	credentialsFile := filepath.Join(dir, "credentials")
	require.NoError(t, os.WriteFile(credentialsFile, []byte("[atmos]\naws_access_key_id = AKIAFROMFILE\naws_secret_access_key = file-secret\n"), 0o600))
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAPROCESSKEY")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "process-secret")

	_, err := AssumeInlineRole(context.Background(), InlineRole{Role: "arn:aws:iam::1:role/r"},
		[]string{"AWS_PROFILE=atmos", "AWS_SHARED_CREDENTIALS_FILE=" + credentialsFile, "AWS_CONFIG_FILE=" + filepath.Join(dir, "config"), "AWS_ENDPOINT_URL=" + sts.server.URL, "AWS_DEFAULT_REGION=us-east-2"})
	require.NoError(t, err)
	form, authorization := sts.last(t)
	assert.Contains(t, authorization, "Credential=AKIAFROMFILE/", "the profile named by the environment signs the request, not the process environment")
	assert.Empty(t, form.Get("DurationSeconds"), "no duration is sent unless one is requested")
	assert.Contains(t, form.Get("RoleSessionName"), "atmos-", "a generated session name is used")
}

func TestAssumeInlineRoleWithWebIdentityNeedsNoCredentials(t *testing.T) {
	sts := newSTSRecorder(t)
	env, err := AssumeInlineRole(context.Background(),
		InlineRole{Role: "arn:aws:iam::1:role/oidc", WebIdentityToken: "oidc-token-value", SessionName: "ci"},
		[]string{"AWS_ENDPOINT_URL_STS=" + sts.server.URL})
	require.NoError(t, err)
	form, _ := sts.last(t)
	assert.Equal(t, "AssumeRoleWithWebIdentity", form.Get("Action"))
	assert.Equal(t, "oidc-token-value", form.Get("WebIdentityToken"))
	assert.Equal(t, "ci", form.Get("RoleSessionName"))
	assert.Equal(t, assumedKey, env["AWS_ACCESS_KEY_ID"])
	assert.NotContains(t, env, "oidc-token-value")
}

func TestAssumeInlineRoleFailures(t *testing.T) {
	t.Run("STS rejects the role", func(t *testing.T) {
		server := stsErrorServer(t, http.StatusForbidden, "AccessDenied", "not authorized to assume the role")
		_, err := AssumeInlineRole(context.Background(), InlineRole{Role: "arn:aws:iam::1:role/denied"},
			[]string{"AWS_ACCESS_KEY_ID=AKIAX", "AWS_SECRET_ACCESS_KEY=secret-never-shown", "AWS_ENDPOINT_URL_STS=" + server.URL})
		require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
		// CI renders color and wraps long lines, so compare the text without escapes and whitespace.
		rendered := strings.Join(strings.Fields(ansi.Strip(errUtils.Format(err, errUtils.FormatterConfig{}))), "")
		assert.Contains(t, rendered, "arn:aws:iam::1:role/denied")
		assert.NotContains(t, err.Error(), "secret-never-shown")
	})
	t.Run("no starting credentials", func(t *testing.T) {
		empty := t.TempDir()
		t.Setenv("AWS_ACCESS_KEY_ID", "")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "")
		t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
		_, err := AssumeInlineRole(context.Background(), InlineRole{Role: "arn:aws:iam::1:role/r"},
			[]string{"AWS_SHARED_CREDENTIALS_FILE=" + filepath.Join(empty, "none"), "AWS_CONFIG_FILE=" + filepath.Join(empty, "none"), "AWS_PROFILE=missing"})
		require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	})
}

func TestInlineRoleConfigAndEnvironment(t *testing.T) {
	env := environMap([]string{"A=1", "A=2", "B=x=y", "malformed"})
	assert.Equal(t, map[string]string{"A": "2", "B": "x=y"}, env)
	identity := inlineRoleConfig(InlineRole{Role: "r"}, map[string]string{})
	assert.Nil(t, identity.Spec)
	assert.NotContains(t, identity.Principal, "duration")
}
