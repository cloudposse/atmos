package types

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestAWSCredentials_IsExpired(t *testing.T) {
	now := time.Now().UTC()

	cases := []struct {
		name string
		exp  string
		want bool
	}{
		{"no-exp", "", false},
		{"invalid-format", "not-a-time", true},
		{"past", now.Add(-1 * time.Hour).Format(time.RFC3339), true},
		{"future", now.Add(1 * time.Hour).Format(time.RFC3339), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &AWSCredentials{Expiration: tc.exp}
			assert.Equal(t, tc.want, c.IsExpired())
		})
	}
}

func TestAWSCredentials_GetExpiration(t *testing.T) {
	testGetExpiration(t, &AWSCredentials{}, func(c interface{}, exp string) {
		c.(*AWSCredentials).Expiration = exp
	})
}

func TestAWSCredentials_BuildWhoamiInfo(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	c := &AWSCredentials{
		Region:     "us-east-1",
		Expiration: now.Add(15 * time.Minute).Format(time.RFC3339),
	}

	var w WhoamiInfo
	c.BuildWhoamiInfo(&w)

	assert.Equal(t, "us-east-1", w.Region)
	if assert.NotNil(t, w.Expiration) {
		assert.WithinDuration(t, now.Add(15*time.Minute), *w.Expiration, time.Second)
	}
}

// The public entrypoint must preserve cancellation without contacting live STS.
func TestAWSCredentials_ValidateCanceled(t *testing.T) {
	for _, tt := range []struct{ name, expiration string }{
		{name: "without expiration"},
		{name: "with expiration", expiration: time.Date(2099, 1, 2, 3, 4, 5, 0, time.UTC).Format(time.RFC3339)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			credentials := &AWSCredentials{AccessKeyID: "synthetic-access", SecretAccessKey: "synthetic-secret", Region: "us-east-1", Expiration: tt.expiration}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			info, err := credentials.Validate(ctx)
			require.Nil(t, info)
			require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
			require.ErrorIs(t, err, context.Canceled)
		})
	}
}
