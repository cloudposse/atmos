package s3upload

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestClientComponentIdentityWins(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "ambient-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient-secret")
	credentialsFile := filepath.Join(t.TempDir(), "credentials")
	require.NoError(t, os.WriteFile(credentialsFile, []byte("[component]\naws_access_key_id = component-key\naws_secret_access_key = component-secret\n"), 0o600))
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		assert.True(t, strings.Contains(r.Header.Get("Authorization"), "Credential=component-key/"))
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	authContext := &schema.AWSAuthContext{CredentialsFile: credentialsFile, ConfigFile: filepath.Join(t.TempDir(), "config"), Profile: "component", Region: "us-west-2", EndpointURL: server.URL}
	env := map[string]string{"AWS_ENDPOINT_URL_S3": "http://127.0.0.1:1"}
	client, err := NewClient(t.Context(), "", authContext, env)
	require.NoError(t, err)
	_, err = client.HeadObject(t.Context(), &s3.HeadObjectInput{Bucket: aws.String("bucket"), Key: aws.String("key")})
	require.Error(t, err)
	assert.Equal(t, 1, requests)
}

func TestClientEnvironmentAndRegionPrecedence(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "ambient-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient-secret")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	directory := t.TempDir()
	credentialsFile := filepath.Join(directory, "credentials")
	configFile := filepath.Join(directory, "config")
	require.NoError(t, os.WriteFile(credentialsFile, []byte("[selected]\naws_access_key_id = profile-key\naws_secret_access_key = profile-secret\n"), 0o600))
	require.NoError(t, os.WriteFile(configFile, []byte("[profile selected]\nregion = eu-west-1\n"), 0o600))
	for _, tc := range []struct {
		name    string
		region  string
		env     map[string]string
		wantKey string
		wantReg string
	}{
		{name: "ambient credentials", env: map[string]string{}, wantKey: "ambient-key", wantReg: "us-east-1"},
		{name: "static overlay", env: map[string]string{"AWS_ACCESS_KEY_ID": "step-key", "AWS_SECRET_ACCESS_KEY": "step-secret", "AWS_SESSION_TOKEN": "step-token", "AWS_REGION": "us-west-2"}, wantKey: "step-key", wantReg: "us-west-2"},
		{name: "profile overlay overrides ambient keys", env: map[string]string{"AWS_PROFILE": "selected"}, wantKey: "profile-key", wantReg: "us-east-1"},
		{name: "explicit region overrides overlay", region: "eu-central-1", env: map[string]string{"AWS_REGION": "us-west-2"}, wantKey: "ambient-key", wantReg: "eu-central-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				assert.Contains(t, r.Header.Get("Authorization"), "Credential="+tc.wantKey+"/")
				assert.Contains(t, r.Header.Get("Authorization"), "/"+tc.wantReg+"/s3/aws4_request")
				if token := tc.env["AWS_SESSION_TOKEN"]; token != "" {
					assert.Equal(t, token, r.Header.Get("X-Amz-Security-Token"))
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			tc.env["AWS_ENDPOINT_URL"] = server.URL
			tc.env["AWS_SHARED_CREDENTIALS_FILE"] = credentialsFile
			tc.env["AWS_CONFIG_FILE"] = configFile
			client, err := NewClient(t.Context(), tc.region, nil, tc.env)
			require.NoError(t, err)
			_, err = client.HeadObject(t.Context(), &s3.HeadObjectInput{Bucket: aws.String("bucket"), Key: aws.String("key")})
			require.NoError(t, err)
			assert.Equal(t, 1, requests)
		})
	}
	assert.Equal(t, "ambient-key", os.Getenv("AWS_ACCESS_KEY_ID"))
}

func TestClientRejectsIncompleteCredentialsAndInvalidConfig(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	for _, env := range []map[string]string{{"AWS_ACCESS_KEY_ID": "key"}, {"AWS_SECRET_ACCESS_KEY": "secret"}} {
		_, err := NewClient(t.Context(), "us-east-1", nil, env)
		require.ErrorIs(t, err, errUtils.ErrS3Upload)
	}
	configFile := filepath.Join(t.TempDir(), "invalid-config")
	require.NoError(t, os.WriteFile(configFile, []byte("[profile broken]\nretry_mode = invalid\n"), 0o600))
	_, err := NewClient(t.Context(), "", nil, map[string]string{"AWS_CONFIG_FILE": configFile, "AWS_PROFILE": "broken"})
	require.ErrorIs(t, err, errUtils.ErrLoadAWSConfig)
}
