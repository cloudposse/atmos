package types

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestGCPCredentials_IsExpired(t *testing.T) {
	now := time.Now().UTC()

	cases := []struct {
		name      string
		tokenExp  time.Time
		wantExpired bool
	}{
		{"zero-time", time.Time{}, false},
		{"past", now.Add(-1 * time.Hour), true},
		{"future", now.Add(1 * time.Hour), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &GCPCredentials{TokenExpiry: tc.tokenExp}
			assert.Equal(t, tc.wantExpired, c.IsExpired())
		})
	}
}

func TestGCPCredentials_GetExpiration(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	// Zero TokenExpiry returns nil, nil.
	c := &GCPCredentials{}
	exp, err := c.GetExpiration()
	assert.NoError(t, err)
	assert.Nil(t, exp)

	// Valid TokenExpiry returns local time.
	c.TokenExpiry = now.Add(30 * time.Minute)
	exp, err = c.GetExpiration()
	assert.NoError(t, err)
	if assert.NotNil(t, exp) {
		assert.WithinDuration(t, now.Add(30*time.Minute), *exp, time.Second)
	}
}

func TestGCPCredentials_GetExpirationTime(t *testing.T) {
	now := time.Now().UTC()
	c := &GCPCredentials{TokenExpiry: now}
	assert.Equal(t, now, c.GetExpirationTime())

	c2 := &GCPCredentials{}
	assert.True(t, c2.GetExpirationTime().IsZero())
}

func TestGCPCredentials_BuildWhoamiInfo(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	c := &GCPCredentials{
		ProjectID:            "my-project",
		ServiceAccountEmail:  "sa@my-project.iam.gserviceaccount.com",
		TokenExpiry:          now.Add(15 * time.Minute),
	}

	var w WhoamiInfo
	c.BuildWhoamiInfo(&w)

	assert.Equal(t, "my-project", w.Account)
	assert.Equal(t, "sa@my-project.iam.gserviceaccount.com", w.Principal)
	if assert.NotNil(t, w.Expiration) {
		assert.WithinDuration(t, now.Add(15*time.Minute), *w.Expiration, time.Second)
	}
}

func TestGCPCredentials_BuildWhoamiInfo_NilSafe(t *testing.T) {
	c := &GCPCredentials{ProjectID: "p"}
	c.BuildWhoamiInfo(nil)
	// No panic.
}

func TestGCPCredentials_Validate(t *testing.T) {
	c := &GCPCredentials{
		AccessToken: "token",
		ProjectID:   "proj",
	}

	ctx := context.Background()
	info, err := c.Validate(ctx)

	assert.Nil(t, info)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrNotImplemented))
}
