package store

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSecretsManager is an in-memory SecretsManagerClient for testing the ASM store logic.
type fakeSecretsManager struct {
	data map[string]string

	createErr error // returned by CreateSecret when set.
	putErr    error // returned by PutSecretValue (overrides the not-found-on-missing default).
	getErr    error // returned by GetSecretValue when set.
	delErr    error // returned by DeleteSecret when set.

	// nilStringOnGet makes GetSecretValue return an output with a nil SecretString.
	nilStringOnGet bool
}

func newFakeSecretsManager() *fakeSecretsManager {
	return &fakeSecretsManager{data: make(map[string]string)}
}

func (f *fakeSecretsManager) CreateSecret(_ context.Context, in *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.data[aws.ToString(in.Name)] = aws.ToString(in.SecretString)
	return &secretsmanager.CreateSecretOutput{}, nil
}

func (f *fakeSecretsManager) PutSecretValue(_ context.Context, in *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
	if f.putErr != nil {
		return nil, f.putErr
	}
	id := aws.ToString(in.SecretId)
	if _, ok := f.data[id]; !ok {
		return nil, &smtypes.ResourceNotFoundException{}
	}
	f.data[id] = aws.ToString(in.SecretString)
	return &secretsmanager.PutSecretValueOutput{}, nil
}

func (f *fakeSecretsManager) GetSecretValue(_ context.Context, in *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	id := aws.ToString(in.SecretId)
	v, ok := f.data[id]
	if !ok {
		return nil, &smtypes.ResourceNotFoundException{}
	}
	if f.nilStringOnGet {
		return &secretsmanager.GetSecretValueOutput{}, nil
	}
	return &secretsmanager.GetSecretValueOutput{SecretString: aws.String(v)}, nil
}

func (f *fakeSecretsManager) DeleteSecret(_ context.Context, in *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
	if f.delErr != nil {
		return nil, f.delErr
	}
	delete(f.data, aws.ToString(in.SecretId))
	return &secretsmanager.DeleteSecretOutput{}, nil
}

func newTestASMStore(client SecretsManagerClient) *SecretsManagerStore {
	delim := "-"
	return &SecretsManagerStore{
		client:         client,
		prefix:         "atmos/secrets",
		stackDelimiter: &delim,
		region:         "us-east-1",
	}
}

func TestSecretsManagerStore_SetCreatesThenUpdates(t *testing.T) {
	fake := newFakeSecretsManager()
	s := newTestASMStore(fake)

	// First Set creates (PutSecretValue returns ResourceNotFound, then CreateSecret).
	require.NoError(t, s.Set("prod", "api", "API_KEY", "v1"))
	got, err := s.Get("prod", "api", "API_KEY")
	require.NoError(t, err)
	assert.Equal(t, "v1", got)

	// Second Set updates via PutSecretValue.
	require.NoError(t, s.Set("prod", "api", "API_KEY", "v2"))
	got, err = s.Get("prod", "api", "API_KEY")
	require.NoError(t, err)
	assert.Equal(t, "v2", got)
}

func TestSecretsManagerStore_DeleteAndHas(t *testing.T) {
	fake := newFakeSecretsManager()
	s := newTestASMStore(fake)

	require.NoError(t, s.Set("prod", "api", "API_KEY", "v1"))

	has, err := s.Has("prod", "api", "API_KEY")
	require.NoError(t, err)
	assert.True(t, has)

	require.NoError(t, s.Delete("prod", "api", "API_KEY"))

	has, err = s.Has("prod", "api", "API_KEY")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestSecretsManagerStore_ImplementsInterfaces(t *testing.T) {
	var s Store = newTestASMStore(newFakeSecretsManager())
	_, ok := s.(DeletableStore)
	assert.True(t, ok)
	_, ok = s.(StatusStore)
	assert.True(t, ok)
}

func TestNewSecretsManagerStore_RegionRequired(t *testing.T) {
	_, err := NewSecretsManagerStore(SecretsManagerStoreOptions{}, "")
	assert.ErrorIs(t, err, ErrRegionRequired)
}

func TestNewSecretsManagerStore_DefersClientForIdentity(t *testing.T) {
	// With an identity name set, client init is deferred (lazy) and must not require AWS config.
	prefix := "atmos"
	delim := "_"
	s, err := NewSecretsManagerStore(SecretsManagerStoreOptions{
		Region:         "us-west-2",
		Prefix:         &prefix,
		StackDelimiter: &delim,
	}, "aws/admin")
	require.NoError(t, err)
	asm, ok := s.(*SecretsManagerStore)
	require.True(t, ok)
	assert.Equal(t, "us-west-2", asm.region)
	assert.Equal(t, "atmos", asm.prefix)
	require.NotNil(t, asm.stackDelimiter)
	assert.Equal(t, "_", *asm.stackDelimiter)
	assert.Equal(t, "aws/admin", asm.identityName)
	assert.Nil(t, asm.client, "identity-based store must defer client creation")
}

func TestNewSecretsManagerStore_DefaultStackDelimiter(t *testing.T) {
	// No explicit delimiter and an identity (to avoid touching real AWS) yields the "-" default.
	s, err := NewSecretsManagerStore(SecretsManagerStoreOptions{Region: "us-east-1"}, "aws/admin")
	require.NoError(t, err)
	asm := s.(*SecretsManagerStore)
	require.NotNil(t, asm.stackDelimiter)
	assert.Equal(t, "-", *asm.stackDelimiter)
}

func TestSecretsManagerStore_KeyBuilding(t *testing.T) {
	fake := newFakeSecretsManager()
	s := newTestASMStore(fake) // prefix "atmos/secrets", delimiter "-".

	require.NoError(t, s.Set("plat-prod-ue1", "vpc/flow-logs", "API_KEY", "v"))
	// stack split on "-", component split on "/", joined with "/" final delimiter.
	const wantID = "atmos/secrets/plat/prod/ue1/vpc/flow-logs/API_KEY"
	_, ok := fake.data[wantID]
	assert.True(t, ok, "expected secret id %q, have %v", wantID, fake.data)
}

func TestSecretsManagerStore_Set_Validation(t *testing.T) {
	s := newTestASMStore(newFakeSecretsManager())
	assert.ErrorIs(t, s.Set("", "api", "k", "v"), ErrEmptyStack)
	assert.ErrorIs(t, s.Set("prod", "", "k", "v"), ErrEmptyComponent)
	assert.ErrorIs(t, s.Set("prod", "api", "", "v"), ErrEmptyKey)
	assert.ErrorIs(t, s.Set("prod", "api", "k", nil), ErrNilValue)
}

func TestSecretsManagerStore_Get_Validation(t *testing.T) {
	s := newTestASMStore(newFakeSecretsManager())
	_, err := s.Get("", "api", "k")
	assert.ErrorIs(t, err, ErrEmptyStack)
	_, err = s.Get("prod", "", "k")
	assert.ErrorIs(t, err, ErrEmptyComponent)
	_, err = s.Get("prod", "api", "")
	assert.ErrorIs(t, err, ErrEmptyKey)
}

func TestSecretsManagerStore_Delete_Validation(t *testing.T) {
	s := newTestASMStore(newFakeSecretsManager())
	assert.ErrorIs(t, s.Delete("", "api", "k"), ErrEmptyStack)
	assert.ErrorIs(t, s.Delete("prod", "", "k"), ErrEmptyComponent)
	assert.ErrorIs(t, s.Delete("prod", "api", ""), ErrEmptyKey)
}

func TestSecretsManagerStore_GetKey(t *testing.T) {
	t.Run("with prefix", func(t *testing.T) {
		fake := newFakeSecretsManager()
		fake.data["atmos/secrets/external/token"] = `"tok"`
		s := newTestASMStore(fake)

		got, err := s.GetKey("external/token")
		require.NoError(t, err)
		assert.Equal(t, "tok", got)
	})

	t.Run("empty key rejected", func(t *testing.T) {
		s := newTestASMStore(newFakeSecretsManager())
		_, err := s.GetKey("")
		assert.ErrorIs(t, err, ErrEmptyKey)
	})

	t.Run("not found wrapped", func(t *testing.T) {
		s := newTestASMStore(newFakeSecretsManager())
		_, err := s.GetKey("missing")
		assert.ErrorIs(t, err, ErrGetSecret)
	})
}

func TestSecretsManagerStore_Get_NonJSONPassThrough(t *testing.T) {
	fake := newFakeSecretsManager()
	// A raw, non-JSON secret string is returned verbatim.
	fake.data["atmos/secrets/prod/api/RAW"] = "plain-not-json"
	s := newTestASMStore(fake)

	got, err := s.Get("prod", "api", "RAW")
	require.NoError(t, err)
	assert.Equal(t, "plain-not-json", got)
}

func TestSecretsManagerStore_Get_JSONStructuredRoundTrips(t *testing.T) {
	fake := newFakeSecretsManager()
	s := newTestASMStore(fake)

	require.NoError(t, s.Set("prod", "api", "CFG", map[string]any{"a": 1, "b": "x"}))
	got, err := s.Get("prod", "api", "CFG")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"a": float64(1), "b": "x"}, got)
}

func TestSecretsManagerStore_Get_NilSecretString(t *testing.T) {
	fake := newFakeSecretsManager()
	fake.data["atmos/secrets/prod/api/K"] = "x"
	fake.nilStringOnGet = true
	s := newTestASMStore(fake)

	_, err := s.Get("prod", "api", "K")
	assert.ErrorIs(t, err, ErrGetSecret)
}

func TestSecretsManagerStore_Set_PutErrorNotNotFound(t *testing.T) {
	fake := newFakeSecretsManager()
	fake.putErr = errors.New("access denied")
	s := newTestASMStore(fake)

	err := s.Set("prod", "api", "K", "v")
	assert.ErrorIs(t, err, ErrSetSecret)
}

func TestSecretsManagerStore_Set_CreateErrorWrapped(t *testing.T) {
	// PutSecretValue returns ResourceNotFound (missing), then CreateSecret fails.
	fake := newFakeSecretsManager()
	fake.createErr = errors.New("quota exceeded")
	s := newTestASMStore(fake)

	err := s.Set("prod", "api", "K", "v")
	assert.ErrorIs(t, err, ErrSetSecret)
}

func TestSecretsManagerStore_Delete_ErrorWrapped(t *testing.T) {
	fake := newFakeSecretsManager()
	fake.data["atmos/secrets/prod/api/K"] = `"v"`
	fake.delErr = errors.New("access denied")
	s := newTestASMStore(fake)

	err := s.Delete("prod", "api", "K")
	assert.ErrorIs(t, err, ErrDeleteSecret)
}

func TestSecretsManagerStore_Has_OtherErrorPropagates(t *testing.T) {
	fake := newFakeSecretsManager()
	fake.getErr = errors.New("throttled")
	s := newTestASMStore(fake)

	_, err := s.Has("prod", "api", "K")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrGetSecret)
}

func TestSecretsManagerStore_SetAuthContext(t *testing.T) {
	s := newTestASMStore(newFakeSecretsManager())
	s.SetAuthContext(nil, "aws/admin")
	assert.Equal(t, "aws/admin", s.identityName)
	s.SetAuthContext(nil, "")
	assert.Equal(t, "aws/admin", s.identityName)
}

func TestSecretsManagerStore_GetKey_StackDelimiterNotSet(t *testing.T) {
	s := &SecretsManagerStore{client: newFakeSecretsManager(), region: "us-east-1", stackDelimiter: nil}
	_, err := s.Get("prod", "api", "k")
	assert.ErrorIs(t, err, ErrGetKey)
}
