package aws

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestCredentials_IsExpired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		expiration time.Time
		expected   bool
	}{
		{
			name:       "zero expiration is not expired",
			expiration: time.Time{},
			expected:   false,
		},
		{
			name:       "past expiration is expired",
			expiration: time.Now().Add(-time.Minute),
			expected:   true,
		},
		{
			name:       "future expiration is not expired",
			expiration: time.Now().Add(time.Minute),
			expected:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			creds := &Credentials{Expiration: tt.expiration}
			assert.Equal(t, tt.expected, creds.IsExpired())
		})
	}
}

func TestCredentials_GetExpiration(t *testing.T) {
	t.Parallel()

	credsWithoutExpiration := &Credentials{}
	expiration, err := credsWithoutExpiration.GetExpiration()
	require.NoError(t, err)
	assert.Nil(t, expiration)

	expected := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	credsWithExpiration := &Credentials{Expiration: expected}
	expiration, err = credsWithExpiration.GetExpiration()
	require.NoError(t, err)
	require.NotNil(t, expiration)
	assert.True(t, expected.Equal(*expiration))
}

func TestCredentials_BuildWhoamiInfo_NilInfo(t *testing.T) {
	t.Parallel()

	creds := &Credentials{Region: MockRegion}
	assert.NotPanics(t, func() {
		creds.BuildWhoamiInfo(nil)
	})
}

func TestCredentials_BuildWhoamiInfo_PopulatesExpectedFields(t *testing.T) {
	t.Parallel()

	expiration := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	creds := &Credentials{
		AccessKeyID:     "AKIA_TEST",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Region:          "eu-west-1",
		Expiration:      expiration,
	}
	info := &types.WhoamiInfo{}

	creds.BuildWhoamiInfo(info)

	assert.Same(t, creds, info.Credentials)
	assert.Equal(t, "eu-west-1", info.Region)
	require.NotNil(t, info.Expiration)
	assert.True(t, expiration.Equal(*info.Expiration))
	assert.Equal(t, "eu-west-1", info.Environment["AWS_REGION"])
	assert.Equal(t, "eu-west-1", info.Environment["AWS_DEFAULT_REGION"])
	assert.NotContains(t, info.Environment, "AWS_ACCESS_KEY_ID")
	assert.NotContains(t, info.Environment, "AWS_SECRET_ACCESS_KEY")
	assert.NotContains(t, info.Environment, "AWS_SESSION_TOKEN")
}

func TestCredentials_Validate(t *testing.T) {
	t.Parallel()

	expiration := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	creds := &Credentials{
		Region:     MockRegion,
		Expiration: expiration,
	}

	validation, err := creds.Validate(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:iam::123456789012:user/mock-user", validation.Principal)
	assert.Equal(t, "123456789012", validation.Account)
	require.NotNil(t, validation.Expiration)
	assert.True(t, expiration.Equal(*validation.Expiration))
}

