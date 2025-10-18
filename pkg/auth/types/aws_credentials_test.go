package types

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

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
	now := time.Now().UTC().Truncate(time.Second)

	// Blank.
	c := &AWSCredentials{}
	exp, err := c.GetExpiration()
	assert.NoError(t, err)
	assert.Nil(t, exp)

	// Valid.
	c.Expiration = now.Add(30 * time.Minute).Format(time.RFC3339)
	exp, err = c.GetExpiration()
	assert.NoError(t, err)
	if assert.NotNil(t, exp) {
		assert.WithinDuration(t, now.Add(30*time.Minute), *exp, time.Second)
	}

	// Invalid.
	c.Expiration = "bogus"
	exp, err = c.GetExpiration()
	assert.Nil(t, exp)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig))
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
