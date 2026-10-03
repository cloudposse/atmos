package aws

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guards: field renames used by these tests fail the build, and the identity must
// satisfy the standalone interface the manager dispatches through.
var (
	_                          = schema.Identity{Kind: "", Via: nil, Credentials: nil, Spec: nil, Env: nil}
	_                          = types.AWSCredentials{AccessKeyID: "", SecretAccessKey: "", SessionToken: "", Region: "", Expiration: ""}
	_ types.StandaloneIdentity = (*credentialProcessIdentity)(nil)
	_ types.Identity           = (*credentialProcessIdentity)(nil)
)

const cpTestCommand = "okta-aws-cli web --profile prod"

// fakeRetriever is an injectable credential_process runner.
type fakeRetriever struct {
	calls    int
	commands []string
	names    []string
	creds    *types.AWSCredentials
	err      error
}

func (f *fakeRetriever) retrieve(_ context.Context, name, command string) (*types.AWSCredentials, error) {
	f.calls++
	f.names = append(f.names, name)
	f.commands = append(f.commands, command)
	if f.err != nil {
		return nil, f.err
	}
	if f.creds == nil {
		return nil, nil
	}
	// Return a copy so the identity may mutate it (region) without affecting the fixture.
	c := *f.creds
	return &c, nil
}

// isolateXDG points the Atmos-managed AWS files at a temp dir.
func isolateXDG(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("ATMOS_XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	return dir
}

func newCPIdentity(t *testing.T, credentials map[string]any, fake *fakeRetriever) *credentialProcessIdentity {
	t.Helper()
	if credentials == nil {
		credentials = map[string]any{"credential_process": cpTestCommand}
	}
	id, err := NewCredentialProcessIdentity("corp-base", &schema.Identity{
		Kind:        types.IdentityKindAWSCredentialProcess,
		Credentials: credentials,
	})
	require.NoError(t, err)
	cp, ok := id.(*credentialProcessIdentity)
	require.True(t, ok)
	if fake != nil {
		cp.retrieve = fake.retrieve
	}
	return cp
}

func futureExpiration() string { return time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339) }

func TestNewCredentialProcessIdentity(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		id, err := NewCredentialProcessIdentity("x", nil)
		require.Error(t, err)
		assert.Nil(t, id)
		assert.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
	})

	t.Run("wrong kind", func(t *testing.T) {
		id, err := NewCredentialProcessIdentity("x", &schema.Identity{Kind: "aws/user"})
		require.Error(t, err)
		assert.Nil(t, id)
		assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityKind)
	})

	t.Run("valid", func(t *testing.T) {
		id := newCPIdentity(t, nil, nil)
		assert.Equal(t, "aws/credential-process", id.Kind())
		assert.NotNil(t, id.retrieve, "the real credential_process runner is the default")

		provider, err := id.GetProviderName()
		require.NoError(t, err)
		assert.Equal(t, "aws-credential-process", provider)

		assert.True(t, id.IsStandalone())
	})

	t.Run("constructor tolerates invalid settings so Validate can report them", func(t *testing.T) {
		id, err := NewCredentialProcessIdentity("x", &schema.Identity{Kind: types.IdentityKindAWSCredentialProcess})
		require.NoError(t, err)
		require.ErrorIs(t, id.Validate(), errUtils.ErrInvalidIdentityConfig)
	})
}

func TestCredentialProcessIdentity_Validate(t *testing.T) {
	tests := []struct {
		name        string
		identity    schema.Identity
		wantErr     bool
		wantContain string
	}{
		{
			name: "valid minimal",
			identity: schema.Identity{
				Kind:        types.IdentityKindAWSCredentialProcess,
				Credentials: map[string]any{"credential_process": cpTestCommand},
			},
		},
		{
			name: "valid with region and endpoint",
			identity: schema.Identity{
				Kind:        types.IdentityKindAWSCredentialProcess,
				Credentials: map[string]any{"credential_process": cpTestCommand, "region": "eu-west-1"},
				Spec:        map[string]any{"endpoint_url": "http://localhost:4566"},
			},
		},
		{
			name:        "missing credentials section",
			identity:    schema.Identity{Kind: types.IdentityKindAWSCredentialProcess},
			wantErr:     true,
			wantContain: "credentials.credential_process",
		},
		{
			name: "empty credential_process",
			identity: schema.Identity{
				Kind:        types.IdentityKindAWSCredentialProcess,
				Credentials: map[string]any{"credential_process": ""},
			},
			wantErr:     true,
			wantContain: "credentials.credential_process",
		},
		{
			name: "whitespace credential_process",
			identity: schema.Identity{
				Kind:        types.IdentityKindAWSCredentialProcess,
				Credentials: map[string]any{"credential_process": "   "},
			},
			wantErr:     true,
			wantContain: "credentials.credential_process",
		},
		{
			name: "non-string credential_process",
			identity: schema.Identity{
				Kind:        types.IdentityKindAWSCredentialProcess,
				Credentials: map[string]any{"credential_process": 42},
			},
			wantErr:     true,
			wantContain: "credentials.credential_process",
		},
		{
			name: "rejects via provider",
			identity: schema.Identity{
				Kind:        types.IdentityKindAWSCredentialProcess,
				Via:         &schema.IdentityVia{Provider: "sso"},
				Credentials: map[string]any{"credential_process": cpTestCommand},
			},
			wantErr:     true,
			wantContain: "via",
		},
		{
			name: "rejects via identity",
			identity: schema.Identity{
				Kind:        types.IdentityKindAWSCredentialProcess,
				Via:         &schema.IdentityVia{Identity: "other"},
				Credentials: map[string]any{"credential_process": cpTestCommand},
			},
			wantErr:     true,
			wantContain: "via",
		},
		{
			name: "rejects session duration",
			identity: schema.Identity{
				Kind:        types.IdentityKindAWSCredentialProcess,
				Session:     &schema.SessionConfig{Duration: "12h"},
				Credentials: map[string]any{"credential_process": cpTestCommand},
			},
			wantErr:     true,
			wantContain: "session",
		},
		{
			name: "rejects principal",
			identity: schema.Identity{
				Kind:        types.IdentityKindAWSCredentialProcess,
				Principal:   map[string]any{"name": "Admin"},
				Credentials: map[string]any{"credential_process": cpTestCommand},
			},
			wantErr:     true,
			wantContain: "principal",
		},
		{
			name: "rejects access_key_id",
			identity: schema.Identity{
				Kind:        types.IdentityKindAWSCredentialProcess,
				Credentials: map[string]any{"credential_process": cpTestCommand, "access_key_id": "AKIA"},
			},
			wantErr:     true,
			wantContain: "access_key_id",
		},
		{
			name: "rejects secret_access_key",
			identity: schema.Identity{
				Kind:        types.IdentityKindAWSCredentialProcess,
				Credentials: map[string]any{"credential_process": cpTestCommand, "secret_access_key": "SECRET"},
			},
			wantErr:     true,
			wantContain: "secret_access_key",
		},
		{
			name: "rejects mfa_arn",
			identity: schema.Identity{
				Kind:        types.IdentityKindAWSCredentialProcess,
				Credentials: map[string]any{"credential_process": cpTestCommand, "mfa_arn": "arn:aws:iam::111111111111:mfa/me"},
			},
			wantErr:     true,
			wantContain: "mfa_arn",
		},
		{
			name: "empty access_key_id is ignored",
			identity: schema.Identity{
				Kind:        types.IdentityKindAWSCredentialProcess,
				Credentials: map[string]any{"credential_process": cpTestCommand, "access_key_id": ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := NewCredentialProcessIdentity("corp-base", &tt.identity)
			require.NoError(t, err)

			err = id.Validate()
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
			assert.Contains(t, err.Error(), tt.wantContain)
		})
	}

	t.Run("key rejections point to aws/user", func(t *testing.T) {
		id, err := NewCredentialProcessIdentity("corp-base", &schema.Identity{
			Kind:        types.IdentityKindAWSCredentialProcess,
			Credentials: map[string]any{"credential_process": cpTestCommand, "access_key_id": "AKIA"},
		})
		require.NoError(t, err)
		err = id.Validate()
		require.Error(t, err)
		assert.Contains(t, strings.Join(cockroachErrors.GetAllHints(err), "\n"), "aws/user")
	})
}

func TestCredentialProcessIdentity_Authenticate_ReusesUnexpiredFiles(t *testing.T) {
	isolateXDG(t)
	fake := &fakeRetriever{err: errors.New("the helper must not run")}
	id := newCPIdentity(t, nil, fake)

	cached := &types.AWSCredentials{
		AccessKeyID:     "AKIACACHED",
		SecretAccessKey: "cached-secret",
		SessionToken:    "cached-token",
		Region:          "us-east-1",
		Expiration:      futureExpiration(),
	}
	require.NoError(t, id.writeAWSFiles(cached))

	got, err := id.Authenticate(context.Background(), nil)
	require.NoError(t, err)

	creds, ok := got.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, 0, fake.calls, "unexpired cached credentials must skip the helper")
	assert.Equal(t, "AKIACACHED", creds.AccessKeyID)
	assert.Equal(t, "cached-secret", creds.SecretAccessKey)
	assert.Equal(t, "cached-token", creds.SessionToken)
	assert.Equal(t, cached.Expiration, creds.Expiration)
}

func TestCredentialProcessIdentity_Authenticate_HonorsMinValidityFromContext(t *testing.T) {
	// Cached credentials with 20 minutes left satisfy the 15 minute default but not a 30 minute request.
	cachedExpiration := time.Now().UTC().Add(20 * time.Minute).Format(time.RFC3339)
	fresh := &types.AWSCredentials{
		AccessKeyID:     "AKIAFRESH",
		SecretAccessKey: "fresh-secret",
		SessionToken:    "fresh-token",
		Expiration:      cachedExpiration, // The helper only mints 20 minute credentials.
	}

	tests := []struct {
		name      string
		ctx       func() context.Context
		wantCalls int
	}{
		{name: "no requested minimum uses the 15 minute default", ctx: context.Background, wantCalls: 0},
		{
			name:      "smaller minimum reuses the cache",
			ctx:       func() context.Context { return types.WithMinCredentialValidity(context.Background(), 5*time.Minute) },
			wantCalls: 0,
		},
		{
			name:      "larger minimum refreshes through the helper",
			ctx:       func() context.Context { return types.WithMinCredentialValidity(context.Background(), 30*time.Minute) },
			wantCalls: 1,
		},
		{
			name:      "zero minimum reuses any unexpired cache",
			ctx:       func() context.Context { return types.WithMinCredentialValidity(context.Background(), 0) },
			wantCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateXDG(t)
			fake := &fakeRetriever{creds: fresh}
			id := newCPIdentity(t, nil, fake)
			require.NoError(t, id.writeAWSFiles(&types.AWSCredentials{
				AccessKeyID: "AKIACACHED", SecretAccessKey: "cached-secret", SessionToken: "cached-token",
				Expiration: cachedExpiration,
			}))

			got, err := id.Authenticate(tt.ctx(), nil)
			require.NoError(t, err)

			assert.Equal(t, tt.wantCalls, fake.calls)
			creds, ok := got.(*types.AWSCredentials)
			require.True(t, ok)
			if tt.wantCalls == 0 {
				assert.Equal(t, "AKIACACHED", creds.AccessKeyID)
			} else {
				// The helper still only issues short-lived credentials: they are returned as-is, no loop, no error.
				assert.Equal(t, "AKIAFRESH", creds.AccessKeyID)
			}
		})
	}
}

func TestCredentialProcessIdentity_Authenticate_RunsHelperWhenFilesNotReusable(t *testing.T) {
	fresh := &types.AWSCredentials{
		AccessKeyID:     "AKIAFRESH",
		SecretAccessKey: "fresh-secret",
		SessionToken:    "fresh-token",
		Expiration:      futureExpiration(),
	}

	tests := []struct {
		name   string
		cached *types.AWSCredentials // nil means no files exist.
	}{
		{name: "no files"},
		{
			name: "expired files",
			cached: &types.AWSCredentials{
				AccessKeyID: "AKIAOLD", SecretAccessKey: "old-secret", SessionToken: "old-token",
				Expiration: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
			},
		},
		{
			name: "files expiring within the reuse buffer",
			cached: &types.AWSCredentials{
				AccessKeyID: "AKIAOLD", SecretAccessKey: "old-secret", SessionToken: "old-token",
				Expiration: time.Now().UTC().Add(types.DefaultMinCredentialValidity / 2).Format(time.RFC3339),
			},
		},
		{
			name: "files with a session token but no expiration",
			cached: &types.AWSCredentials{
				AccessKeyID: "AKIAOLD", SecretAccessKey: "old-secret", SessionToken: "old-token",
			},
		},
		{
			name: "long-lived files with no expiration",
			cached: &types.AWSCredentials{
				AccessKeyID: "AKIAOLD", SecretAccessKey: "old-secret",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateXDG(t)
			fake := &fakeRetriever{creds: fresh}
			id := newCPIdentity(t, nil, fake)

			if tt.cached != nil {
				require.NoError(t, id.writeAWSFiles(tt.cached))
			}

			got, err := id.Authenticate(context.Background(), nil)
			require.NoError(t, err)

			creds, ok := got.(*types.AWSCredentials)
			require.True(t, ok)
			assert.Equal(t, 1, fake.calls)
			assert.Equal(t, []string{"corp-base"}, fake.names)
			assert.Equal(t, []string{cpTestCommand}, fake.commands)
			assert.Equal(t, "AKIAFRESH", creds.AccessKeyID)
			assert.Equal(t, "fresh-secret", creds.SecretAccessKey)
			assert.Equal(t, "fresh-token", creds.SessionToken)
			assert.Equal(t, fresh.Expiration, creds.Expiration)

			// The new credentials replace whatever was cached.
			loaded, err := id.LoadCredentials(context.Background())
			require.NoError(t, err)
			loadedCreds, ok := loaded.(*types.AWSCredentials)
			require.True(t, ok)
			assert.Equal(t, "AKIAFRESH", loadedCreds.AccessKeyID)
		})
	}
}

func TestCredentialProcessIdentity_Authenticate_WritesFilesUnderProviderDirectory(t *testing.T) {
	fake := &fakeRetriever{creds: &types.AWSCredentials{
		AccessKeyID: "AKIAFRESH", SecretAccessKey: "fresh-secret", SessionToken: "fresh-token", Expiration: futureExpiration(),
	}}

	t.Run("configured region", func(t *testing.T) {
		dir := isolateXDG(t)
		id := newCPIdentity(t, map[string]any{"credential_process": cpTestCommand, "region": "eu-west-1"}, fake)

		got, err := id.Authenticate(context.Background(), nil)
		require.NoError(t, err)
		creds, ok := got.(*types.AWSCredentials)
		require.True(t, ok)
		assert.Equal(t, "eu-west-1", creds.Region)

		env, err := id.Environment()
		require.NoError(t, err)
		wantDir := filepath.Join(dir, "atmos", "aws", "aws-credential-process")
		assert.Equal(t, filepath.Join(wantDir, "credentials"), env["AWS_SHARED_CREDENTIALS_FILE"])
		assert.Equal(t, filepath.Join(wantDir, "config"), env["AWS_CONFIG_FILE"])
		assert.Equal(t, "corp-base", env["AWS_PROFILE"])
		assert.Equal(t, "eu-west-1", env["AWS_REGION"])
		assert.Equal(t, "eu-west-1", env["AWS_DEFAULT_REGION"])

		credFile, err := os.ReadFile(env["AWS_SHARED_CREDENTIALS_FILE"])
		require.NoError(t, err)
		assert.Contains(t, string(credFile), "[corp-base]")
		assert.Regexp(t, `aws_access_key_id\s*=\s*AKIAFRESH`, string(credFile))
		assert.Contains(t, string(credFile), "atmos: expiration="+fake.creds.Expiration)

		configFile, err := os.ReadFile(env["AWS_CONFIG_FILE"])
		require.NoError(t, err)
		assert.Contains(t, string(configFile), "eu-west-1")

		exists, err := id.CredentialsExist()
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("default region", func(t *testing.T) {
		isolateXDG(t)
		id := newCPIdentity(t, nil, fake)
		got, err := id.Authenticate(context.Background(), nil)
		require.NoError(t, err)
		creds, ok := got.(*types.AWSCredentials)
		require.True(t, ok)
		assert.Equal(t, defaultRegion, creds.Region)

		env, err := id.Environment()
		require.NoError(t, err)
		_, hasRegion := env["AWS_REGION"]
		assert.False(t, hasRegion, "the default region is not exported unless configured")
	})
}

func TestCredentialProcessIdentity_Authenticate_PreservesHelperRegion(t *testing.T) {
	isolateXDG(t)
	fake := &fakeRetriever{creds: &types.AWSCredentials{
		AccessKeyID: "AKIA", SecretAccessKey: "secret", SessionToken: "token", Region: "ap-south-1", Expiration: futureExpiration(),
	}}
	id := newCPIdentity(t, map[string]any{"credential_process": cpTestCommand, "region": "eu-west-1"}, fake)

	got, err := id.Authenticate(context.Background(), nil)
	require.NoError(t, err)
	creds, ok := got.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, "ap-south-1", creds.Region)
}

func TestCredentialProcessIdentity_Authenticate_Errors(t *testing.T) {
	t.Run("helper error propagates and writes nothing", func(t *testing.T) {
		isolateXDG(t)
		fake := &fakeRetriever{err: errUtils.ErrCredentialProcessFailed}
		id := newCPIdentity(t, nil, fake)

		got, err := id.Authenticate(context.Background(), nil)
		require.ErrorIs(t, err, errUtils.ErrCredentialProcessFailed)
		assert.Nil(t, got)

		exists, existsErr := id.CredentialsExist()
		require.NoError(t, existsErr)
		assert.False(t, exists)
	})

	t.Run("recursion error propagates", func(t *testing.T) {
		isolateXDG(t)
		fake := &fakeRetriever{err: errUtils.ErrCredentialProcessRecursion}
		_, err := newCPIdentity(t, nil, fake).Authenticate(context.Background(), nil)
		assert.ErrorIs(t, err, errUtils.ErrCredentialProcessRecursion)
	})

	t.Run("nil credentials are invalid output", func(t *testing.T) {
		isolateXDG(t)
		fake := &fakeRetriever{}
		_, err := newCPIdentity(t, nil, fake).Authenticate(context.Background(), nil)
		assert.ErrorIs(t, err, errUtils.ErrCredentialProcessInvalidOutput)
	})

	t.Run("invalid config never runs the helper", func(t *testing.T) {
		isolateXDG(t)
		fake := &fakeRetriever{creds: &types.AWSCredentials{AccessKeyID: "A", SecretAccessKey: "S"}}
		id := newCPIdentity(t, map[string]any{"credential_process": ""}, fake)

		_, err := id.Authenticate(context.Background(), nil)
		require.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
		assert.Equal(t, 0, fake.calls)
	})

	t.Run("unwritable credential directory is reported", func(t *testing.T) {
		dir := isolateXDG(t)
		// A file where the atmos config directory must be created blocks every write.
		require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos"), []byte("not a directory"), 0o600))

		fake := &fakeRetriever{creds: &types.AWSCredentials{AccessKeyID: "A", SecretAccessKey: "S"}}
		_, err := newCPIdentity(t, nil, fake).Authenticate(context.Background(), nil)
		require.ErrorIs(t, err, errUtils.ErrAwsAuth)
		assert.Contains(t, err.Error(), "failed to write AWS files")
	})
}

func TestCredentialProcessIdentity_AuthenticateStandalone(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		isolateXDG(t)
		fake := &fakeRetriever{creds: &types.AWSCredentials{
			AccessKeyID: "AKIA", SecretAccessKey: "secret", SessionToken: "token", Expiration: futureExpiration(),
		}}
		id := newCPIdentity(t, nil, fake)

		got, err := id.AuthenticateStandalone(context.Background())
		require.NoError(t, err)
		creds, ok := got.(*types.AWSCredentials)
		require.True(t, ok)
		assert.Equal(t, "AKIA", creds.AccessKeyID)
		assert.Equal(t, 1, fake.calls)
	})

	t.Run("failure wraps both the auth error and the cause", func(t *testing.T) {
		isolateXDG(t)
		fake := &fakeRetriever{err: errUtils.ErrCredentialProcessFailed}
		_, err := newCPIdentity(t, nil, fake).AuthenticateStandalone(context.Background())
		require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
		assert.ErrorIs(t, err, errUtils.ErrCredentialProcessFailed)
		assert.Contains(t, err.Error(), "corp-base")
	})
}

func TestCredentialProcessIdentity_Authenticate_WithRealHelperProcess(t *testing.T) {
	isolateXDG(t)

	exe, err := os.Executable()
	require.NoError(t, err)
	command := `'` + strings.ReplaceAll(exe, `'`, `'\''`) + `'`
	if runtime.GOOS == "windows" {
		command = exe
		if strings.ContainsAny(exe, " \t") {
			command = `"` + exe + `"`
		}
	}

	exp := futureExpiration()
	// The child (this test binary) inherits the environment and acts as the helper.
	t.Setenv(testCredentialProcessJSONEnv,
		`{"Version":1,"AccessKeyId":"AKIAREAL","SecretAccessKey":"real-secret","SessionToken":"real-token","Expiration":"`+exp+`"}`)

	// Chain from the real constructor so the default runner is exercised.
	id, err := NewCredentialProcessIdentity("corp-base", &schema.Identity{
		Kind:        types.IdentityKindAWSCredentialProcess,
		Credentials: map[string]any{"credential_process": command, "region": "us-west-2"},
	})
	require.NoError(t, err)

	got, err := id.Authenticate(context.Background(), nil)
	require.NoError(t, err)
	creds, ok := got.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, "AKIAREAL", creds.AccessKeyID)
	assert.Equal(t, "real-secret", creds.SecretAccessKey)
	assert.Equal(t, "real-token", creds.SessionToken)
	assert.Equal(t, exp, creds.Expiration)
	assert.Equal(t, "us-west-2", creds.Region)

	// The second call reuses the cached files instead of spawning the helper again. Make the
	// helper unusable to prove it is not run.
	t.Setenv(testCredentialProcessJSONEnv, `{"Version":1`)
	again, err := id.Authenticate(context.Background(), nil)
	require.NoError(t, err)
	againCreds, ok := again.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, "AKIAREAL", againCreds.AccessKeyID)
}
