package providers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	onepassword "github.com/1password/onepassword-sdk-go"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	vault "github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/store"
)

// newTestVaultKVv2Client builds a real *vault.KVv2-backed vaultKVv2Client pointed at an httptest
// server, exercising the production adapter (Put/Get/Delete) below the wrapper.
func newTestVaultKVv2Client(t *testing.T, srv *httptest.Server) *vaultKVv2Client {
	t.Helper()
	cfg := vault.DefaultConfig()
	cfg.Address = srv.URL
	client, err := vault.NewClient(cfg)
	require.NoError(t, err)
	client.SetToken("test-token")
	return &vaultKVv2Client{kv: client.KVv2("secret")}
}

func TestVaultKVv2Client_PutGetDelete(t *testing.T) {
	var got struct {
		put    bool
		getHit bool
		delHit bool
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/secret/data/prod/api/KEY", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut, http.MethodPost:
			got.put = true
			writeJSON(t, w, map[string]any{"data": map[string]any{"version": 1}})
		case http.MethodGet:
			got.getHit = true
			writeJSON(t, w, map[string]any{
				"data": map[string]any{
					"data":     map[string]any{vaultValueKey: "secret-value"},
					"metadata": map[string]any{"version": 1},
				},
			})
		case http.MethodDelete:
			got.delHit = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestVaultKVv2Client(t, srv)

	require.NoError(t, c.Put(context.Background(), "prod/api/KEY", map[string]any{vaultValueKey: "secret-value"}))
	assert.True(t, got.put)

	data, err := c.Get(context.Background(), "prod/api/KEY")
	require.NoError(t, err)
	assert.Equal(t, "secret-value", data[vaultValueKey])
	assert.True(t, got.getHit)

	require.NoError(t, c.Delete(context.Background(), "prod/api/KEY"))
	assert.True(t, got.delHit)
}

func TestVaultKVv2Client_Get_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/secret/data/boom", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(t, w, map[string]any{"errors": []string{"server error"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestVaultKVv2Client(t, srv)
	_, err := c.Get(context.Background(), "boom")
	require.Error(t, err)
}

func TestNewVaultStore(t *testing.T) {
	t.Run("mount required", func(t *testing.T) {
		t.Setenv("VAULT_ADDR", "")
		_, err := NewVaultStore(&VaultStoreOptions{Address: "https://vault.example"}, "")
		assert.ErrorIs(t, err, store.ErrVaultMountRequired)
	})

	t.Run("path back-compat as mount, defaults", func(t *testing.T) {
		t.Setenv("VAULT_ADDR", "")
		t.Setenv("VAULT_TOKEN", "")
		s, err := NewVaultStore(&VaultStoreOptions{Path: "kv", URL: "https://vault.example", Token: "t"}, "aws/admin")
		require.NoError(t, err)
		vs, ok := s.(*VaultStore)
		require.True(t, ok)
		assert.Equal(t, "kv", vs.mount)
		assert.Equal(t, "aws/admin", vs.identityName)
		require.NotNil(t, vs.stackDelimiter)
		assert.Equal(t, "/", *vs.stackDelimiter, "default stack delimiter is /")
	})

	t.Run("explicit prefix and delimiter and token from env", func(t *testing.T) {
		t.Setenv("VAULT_ADDR", "")
		t.Setenv("VAULT_TOKEN", "env-token")
		prefix := "atmos"
		delim := "-"
		s, err := NewVaultStore(&VaultStoreOptions{
			Mount:          "secret",
			Address:        "https://vault.example",
			Prefix:         &prefix,
			StackDelimiter: &delim,
		}, "")
		require.NoError(t, err)
		vs := s.(*VaultStore)
		assert.Equal(t, "atmos", vs.prefix)
		assert.Equal(t, "-", *vs.stackDelimiter)
	})
}

func TestIsGitHubActionsRunner(t *testing.T) {
	t.Run("true inside runner", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		assert.True(t, isGitHubActionsRunner())
	})
	t.Run("false otherwise", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "")
		assert.False(t, isGitHubActionsRunner())
	})
}

func TestGitHubSecretStringValue(t *testing.T) {
	v, err := githubSecretStringValue("plain")
	require.NoError(t, err)
	assert.Equal(t, "plain", v)

	v, err = githubSecretStringValue([]byte("bytes"))
	require.NoError(t, err)
	assert.Equal(t, "bytes", v)

	v, err = githubSecretStringValue(map[string]any{"a": 1})
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":1}`, v)
}

func TestOPStringValue(t *testing.T) {
	v, err := opStringValue("plain")
	require.NoError(t, err)
	assert.Equal(t, "plain", v)

	v, err = opStringValue([]byte("bytes"))
	require.NoError(t, err)
	assert.Equal(t, "bytes", v)

	v, err = opStringValue(map[string]any{"a": 1})
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":1}`, v)
}

func TestGitHubActionsStore_GetKey(t *testing.T) {
	t.Setenv("DB_PASSWORD", "from-env")
	s := newTestStore(newFakeGitHubActionsClient(),
		&GitHubActionsStoreOptions{Owner: "acme", Repo: "infra"}, true)

	got, err := s.GetKey("db_password")
	require.NoError(t, err)
	assert.Equal(t, "from-env", got)
}

func TestGitHubActionsStore_Set_EmptyKey(t *testing.T) {
	s := newTestStore(newFakeGitHubActionsClient(),
		&GitHubActionsStoreOptions{Owner: "acme", Repo: "infra"}, false)
	assert.ErrorIs(t, s.Set("dev", "vpc", "", "v"), store.ErrEmptyKey)
}

func TestGitHubActionsStore_VerifyAlignment_RepoMismatchWarns(t *testing.T) {
	// A mismatched GITHUB_REPOSITORY triggers the warning branch (no environment configured).
	t.Setenv("GITHUB_REPOSITORY", "someone/else")
	t.Setenv("DB_PASSWORD", "v")
	s := newTestStore(newFakeGitHubActionsClient(),
		&GitHubActionsStoreOptions{Owner: "acme", Repo: "infra"}, true)

	// First read triggers the one-time alignment check; it must not error.
	got, err := s.Get("dev", "vpc", "db_password")
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}

func TestOnePasswordStore_NewAndGetClient(t *testing.T) {
	t.Run("NewOnePasswordStore carries vault", func(t *testing.T) {
		s, err := NewOnePasswordStore(&OnePasswordStoreOptions{Vault: "Shared"})
		require.NoError(t, err)
		ops, ok := s.(*OnePasswordStore)
		require.True(t, ok)
		assert.Equal(t, "Shared", ops.vault)
	})

	t.Run("getClient lazily builds and errors with no creds", func(t *testing.T) {
		t.Setenv("OP_CONNECT_HOST", "")
		t.Setenv("OP_CONNECT_TOKEN", "")
		t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")
		s, err := NewOnePasswordStore(&OnePasswordStoreOptions{Vault: "Shared"})
		require.NoError(t, err)
		_, err = s.Get("prod", "api", "op://Shared/Datadog/api_key")
		assert.ErrorIs(t, err, store.ErrOnePasswordNoAuth)
	})
}

func TestOnePasswordStore_Set_WriteErrorWrapped(t *testing.T) {
	fake := newFakeOPClient(map[string]string{})
	fake.err = assert.AnError
	s := newTestOPStore(fake, "")
	err := s.Set("prod", "api", "op://Shared/Datadog/api_key", "v")
	assert.ErrorIs(t, err, store.ErrOnePasswordWrite)
}

func TestOnePasswordStore_Delete_ErrorWrapped(t *testing.T) {
	fake := newFakeOPClient(map[string]string{})
	fake.err = assert.AnError
	s := newTestOPStore(fake, "")
	err := s.Delete("prod", "api", "op://Shared/Datadog/api_key")
	assert.ErrorIs(t, err, store.ErrOnePasswordDelete)
}

func TestIndexOfSDKField(t *testing.T) {
	fields := []onepassword.ItemField{
		{ID: "id-1", Title: "username"},
		{ID: "id-2", Title: "API_KEY"},
	}
	assert.Equal(t, 1, indexOfSDKField(fields, "api_key"), "matched by title, case-insensitive")
	assert.Equal(t, 0, indexOfSDKField(fields, "id-1"), "matched by ID")
	assert.Equal(t, -1, indexOfSDKField(fields, "missing"))
}

func TestBuildSecretsManagerStore(t *testing.T) {
	cfg := store.StoreConfig{
		Kind:     "aws-secrets-manager",
		Identity: "aws/admin", // identity defers client creation, so no real AWS call.
		Options:  map[string]any{"region": "us-east-1", "prefix": "atmos"},
	}
	s, err := buildSecretsManagerStore("asm", cfg)
	require.NoError(t, err)
	asm, ok := s.(*SecretsManagerStore)
	require.True(t, ok)
	assert.Equal(t, "us-east-1", asm.region)
	assert.Equal(t, "atmos", asm.prefix)
}

func TestBuildVaultStore(t *testing.T) {
	t.Setenv("VAULT_ADDR", "")
	t.Setenv("VAULT_TOKEN", "")
	cfg := store.StoreConfig{
		Kind:     "vault",
		Identity: "aws/admin",
		Options:  map[string]any{"mount": "secret", "address": "https://vault.example", "token": "t"},
	}
	s, err := buildVaultStore("vault", cfg)
	require.NoError(t, err)
	vs, ok := s.(*VaultStore)
	require.True(t, ok)
	assert.Equal(t, "secret", vs.mount)
}

func TestBuildOnePasswordStore(t *testing.T) {
	cfg := store.StoreConfig{
		Kind:    "1password",
		Options: map[string]any{"vault": "Shared", "token": "sa-token"},
	}
	s, err := buildOnePasswordStore("op", cfg)
	require.NoError(t, err)
	ops, ok := s.(*OnePasswordStore)
	require.True(t, ok)
	assert.Equal(t, "Shared", ops.vault)
}

// newTestSSMStore builds an SSMStore wired to an injected client with no role assumption (nil
// writeRoleArn/awsConfig), so Delete/Has run against the injected mock directly.
func newTestSSMStore(client SSMClient) *SSMStore {
	delim := "-"
	return &SSMStore{
		client:         client,
		prefix:         "/atmos",
		stackDelimiter: &delim,
	}
}

func TestSSMStore_Delete(t *testing.T) {
	t.Run("deletes via the injected client", func(t *testing.T) {
		mockSSM := new(MockSSMClient)
		mockSSM.On("DeleteParameter", mock.Anything, mock.Anything).
			Return(&ssm.DeleteParameterOutput{}, nil)
		s := newTestSSMStore(mockSSM)

		require.NoError(t, s.Delete("prod", "api", "API_KEY"))
		mockSSM.AssertExpectations(t)
	})

	t.Run("validation errors", func(t *testing.T) {
		s := newTestSSMStore(new(MockSSMClient))
		assert.ErrorIs(t, s.Delete("prod", "api", ""), store.ErrEmptyKey)
	})

	t.Run("scoped coordinates omit empty segments", func(t *testing.T) {
		// Stack-scoped (empty component) and global (empty stack and component) secret
		// coordinates are valid; the path simply omits the empty segments.
		mockSSM := new(MockSSMClient)
		mockSSM.On("DeleteParameter", mock.Anything, &ssm.DeleteParameterInput{
			Name: aws.String("/atmos/prod/k"),
		}).Return(&ssm.DeleteParameterOutput{}, nil)
		mockSSM.On("DeleteParameter", mock.Anything, &ssm.DeleteParameterInput{
			Name: aws.String("/atmos/k"),
		}).Return(&ssm.DeleteParameterOutput{}, nil)
		s := newTestSSMStore(mockSSM)

		require.NoError(t, s.Delete("prod", "", "k"))
		require.NoError(t, s.Delete("", "", "k"))
		mockSSM.AssertExpectations(t)
	})

	t.Run("client error wrapped", func(t *testing.T) {
		mockSSM := new(MockSSMClient)
		mockSSM.On("DeleteParameter", mock.Anything, mock.Anything).
			Return(nil, errors.New("access denied"))
		s := newTestSSMStore(mockSSM)

		err := s.Delete("prod", "api", "API_KEY")
		assert.ErrorIs(t, err, store.ErrDeleteParameter)
	})
}

func TestSSMStore_Has(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		mockSSM := new(MockSSMClient)
		mockSSM.On("GetParameter", mock.Anything, mock.Anything).
			Return(&ssm.GetParameterOutput{Parameter: &ssmtypes.Parameter{Value: aws.String(`"v"`)}}, nil)
		s := newTestSSMStore(mockSSM)

		has, err := s.Has("prod", "api", "API_KEY")
		require.NoError(t, err)
		assert.True(t, has)
	})

	t.Run("error propagates", func(t *testing.T) {
		mockSSM := new(MockSSMClient)
		mockSSM.On("GetParameter", mock.Anything, mock.Anything).
			Return(nil, errors.New("throttled"))
		s := newTestSSMStore(mockSSM)

		_, err := s.Has("prod", "api", "API_KEY")
		require.Error(t, err)
	})
}

func TestIsParameterNotFound(t *testing.T) {
	assert.True(t, isParameterNotFound(&ssmtypes.ParameterNotFound{}))
	assert.False(t, isParameterNotFound(errors.New("other")))
}
