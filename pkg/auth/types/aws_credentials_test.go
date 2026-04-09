package types

import (
	"context"
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

func TestAWSCredentials_Validate(t *testing.T) {
	// Note: Validate requires valid AWS credentials in environment.
	// In test environment without AWS creds, this will fail.
	// This test verifies the method exists and returns appropriate error.
	c := &AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Region:          "us-east-1",
	}

	ctx := context.Background()
	_, err := c.Validate(ctx)

	// Should return authentication error (invalid credentials).
	assert.Error(t, err, "Validate should fail with invalid credentials")
	assert.True(t, errors.Is(err, errUtils.ErrAuthenticationFailed), "Should return ErrAuthenticationFailed")
}

func TestAWSCredentials_Validate_WithExpiration(t *testing.T) {
	// Test that expiration is returned when available.
	now := time.Now().UTC()
	c := &AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Region:          "us-east-1",
		Expiration:      now.Add(1 * time.Hour).Format(time.RFC3339),
	}

	// This will fail validation (invalid creds), but we're testing the method structure.
	ctx := context.Background()
	_, err := c.Validate(ctx)
	assert.Error(t, err, "Should fail with invalid credentials")
}
