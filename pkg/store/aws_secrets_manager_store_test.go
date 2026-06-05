package store

import (
	"context"
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
}

func newFakeSecretsManager() *fakeSecretsManager {
	return &fakeSecretsManager{data: make(map[string]string)}
}

func (f *fakeSecretsManager) CreateSecret(_ context.Context, in *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	f.data[aws.ToString(in.Name)] = aws.ToString(in.SecretString)
	return &secretsmanager.CreateSecretOutput{}, nil
}

func (f *fakeSecretsManager) PutSecretValue(_ context.Context, in *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
	id := aws.ToString(in.SecretId)
	if _, ok := f.data[id]; !ok {
		return nil, &smtypes.ResourceNotFoundException{}
	}
	f.data[id] = aws.ToString(in.SecretString)
	return &secretsmanager.PutSecretValueOutput{}, nil
}

func (f *fakeSecretsManager) GetSecretValue(_ context.Context, in *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	id := aws.ToString(in.SecretId)
	v, ok := f.data[id]
	if !ok {
		return nil, &smtypes.ResourceNotFoundException{}
	}
	return &secretsmanager.GetSecretValueOutput{SecretString: aws.String(v)}, nil
}

func (f *fakeSecretsManager) DeleteSecret(_ context.Context, in *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
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
