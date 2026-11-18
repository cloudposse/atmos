package authoritytest

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
)

// Harness builds a fresh Environment for each conformance check.
type Harness func(t *testing.T) *Environment

// Run checks that an authority honors the contract in authority.Authority.
func Run(t *testing.T, harness Harness) {
	t.Helper()
	checks := []struct {
		name string
		run  func(*testing.T, *Environment)
	}{
		{"EnsureIsIdempotent", checkEnsureIdempotent},
		{"UsesAreEnrolledSeparately", checkUsesSeparate},
		{"RevokeIsIdempotent", checkRevokeIdempotent},
		{"RevokeOneUseLeavesTheOther", checkRevokeOne},
		{"MissingPermissionNamesIt", checkPermission},
		{"InvalidCredentialIsUnauthorized", checkInvalid},
		{"ExistingKeyIsAdopted", checkAdoption},
		{"UnsupportedUseIsRejected", checkUnsupported},
		{"NothingLeaksTheSecret", checkNoLeak},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) { c.run(t, harness(t)) })
	}
}

func request(key ssh.PublicKey, uses ...authority.Use) authority.Request {
	return authority.Request{KeyName: "ops", Key: key, Uses: uses, Title: "ops@host (Atmos)"}
}

func ensure(t *testing.T, env *Environment, req authority.Request) authority.Receipt {
	t.Helper()
	receipt, err := env.Authority.Ensure(context.Background(), env.Credentials, req)
	require.NoError(t, err)
	return receipt
}

func revoke(t *testing.T, env *Environment, receipt authority.Receipt) {
	t.Helper()
	require.NoError(t, env.Authority.Revoke(context.Background(), env.Credentials, receipt))
}

func count(t *testing.T, env *Environment, key ssh.PublicKey, use authority.Use, want int) {
	t.Helper()
	if env.Enrollments != nil {
		assert.Equal(t, want, env.Enrollments(key, use), "enrollments of %s", use)
	}
}

// withoutCreated clears the issue time, which differs between two calls.
func withoutCreated(r authority.Receipt) authority.Receipt {
	r.Created = time.Time{}
	return r
}

func checkEnsureIdempotent(t *testing.T, env *Environment) {
	key := NewKey(t)
	uses := env.Authority.Supports()
	first := ensure(t, env, request(key, uses...))
	second := ensure(t, env, request(key, uses...))

	assert.Equal(t, env.Authority.Name(), first.Authority)
	assert.Equal(t, "ops", first.KeyName)
	assert.Equal(t, authority.Fingerprint(key), first.Fingerprint)
	assert.Equal(t, authority.AuthorizedKey(key), first.PublicKey)
	assert.True(t, first.ExpiresAt.IsZero() || first.ExpiresAt.After(first.Created))
	assert.Equal(t, authority.NormalizeUses(uses), authority.NormalizeUses(first.Uses))
	assert.Equal(t, withoutCreated(first), withoutCreated(second), "second Ensure returns the same receipt")
	for _, use := range uses {
		count(t, env, key, use, 1)
	}
}

func checkUsesSeparate(t *testing.T, env *Environment) {
	uses := authority.NormalizeUses(env.Authority.Supports())
	if len(uses) < 2 {
		t.Skip("authority supports a single use")
	}
	key := NewKey(t)
	first := ensure(t, env, request(key, uses[0]))
	count(t, env, key, uses[0], 1)
	count(t, env, key, uses[1], 0)

	both := ensure(t, env, request(key, uses...))
	assert.Equal(t, first.References[uses[0]], both.References[uses[0]], "the first enrollment is not disturbed")
	for _, use := range uses {
		count(t, env, key, use, 1)
	}
}

func checkRevokeIdempotent(t *testing.T, env *Environment) {
	key := NewKey(t)
	uses := env.Authority.Supports()
	receipt := ensure(t, env, request(key, uses...))
	revoke(t, env, receipt)
	revoke(t, env, receipt)
	for _, use := range uses {
		count(t, env, key, use, 0)
	}
	// A receipt for something never enrolled is also success.
	revoke(t, env, ensureAndForget(t, env))
}

// ensureAndForget enrolls a key, revokes it, and returns the stale receipt.
func ensureAndForget(t *testing.T, env *Environment) authority.Receipt {
	key := NewKey(t)
	receipt := ensure(t, env, request(key, env.Authority.Supports()...))
	revoke(t, env, receipt)
	return receipt
}

func checkRevokeOne(t *testing.T, env *Environment) {
	uses := authority.NormalizeUses(env.Authority.Supports())
	if len(uses) < 2 || env.Enrollments == nil {
		t.Skip("needs two uses and a way to count enrollments")
	}
	key := NewKey(t)
	receipt := ensure(t, env, request(key, uses...))

	partial := receipt
	partial.Uses = uses[:1]
	partial.References = map[authority.Use]string{uses[0]: receipt.References[uses[0]]}
	revoke(t, env, partial)

	count(t, env, key, uses[0], 0)
	count(t, env, key, uses[1], 1)
}

func checkPermission(t *testing.T, env *Environment) {
	if env.Restricted == nil || env.Permission == nil {
		t.Skip("authority has no permission model")
	}
	for _, use := range env.Authority.Supports() {
		t.Run(string(use), func(t *testing.T) {
			key := NewKey(t)
			_, err := env.Authority.Ensure(context.Background(), env.Restricted(use), request(key, use))
			require.Error(t, err)
			assert.ErrorIs(t, err, authority.ErrMissingPermission)
			assert.Contains(t, err.Error(), env.Permission(use))
			var ae *authority.Error
			require.ErrorAs(t, err, &ae)
			assert.Equal(t, env.Permission(use), ae.Permission)
			count(t, env, key, use, 0)
		})
	}
}

func checkInvalid(t *testing.T, env *Environment) {
	if env.Invalid == nil {
		t.Skip("authority has no credential check")
	}
	_, err := env.Authority.Ensure(context.Background(), env.Invalid, request(NewKey(t), env.Authority.Supports()...))
	require.Error(t, err)
	assert.ErrorIs(t, err, authority.ErrUnauthorized)
	_, err = env.Authority.Ensure(context.Background(), nil, request(NewKey(t), env.Authority.Supports()...))
	assert.ErrorIs(t, err, authority.ErrUnauthorized, "no credential at all")
}

func checkAdoption(t *testing.T, env *Environment) {
	if env.Seed == nil || env.Enrollments == nil {
		t.Skip("authority cannot be pre-seeded")
	}
	key := NewKey(t)
	uses := env.Authority.Supports()
	ids := map[authority.Use]string{}
	for _, use := range uses {
		ids[use] = env.Seed(t, key, use)
	}
	receipt := ensure(t, env, request(key, uses...))
	for _, use := range uses {
		assert.Equal(t, ids[use], receipt.References[use], "reference of the adopted %s key", use)
		count(t, env, key, use, 1)
	}
}

func checkUnsupported(t *testing.T, env *Environment) {
	_, err := env.Authority.Ensure(context.Background(), env.Credentials, request(NewKey(t), authority.Use("teleportation")))
	assert.ErrorIs(t, err, authority.ErrUnsupportedUse)
	_, err = env.Authority.Ensure(context.Background(), env.Credentials, request(NewKey(t)))
	assert.ErrorIs(t, err, authority.ErrInvalidRequest, "no uses")
}

func checkNoLeak(t *testing.T, env *Environment) {
	if len(env.Secrets) == 0 {
		t.Skip("authority takes no secret")
	}
	var artifacts []string
	key := NewKey(t)
	uses := env.Authority.Supports()
	receipt := ensure(t, env, request(key, uses...))
	artifacts = append(artifacts, mustJSON(t, receipt), receipt.Subject)

	record := func(err error) {
		if err != nil {
			artifacts = append(artifacts, err.Error(), errorsJoinText(err))
		}
	}
	if env.Invalid != nil {
		_, err := env.Authority.Ensure(context.Background(), env.Invalid, request(NewKey(t), uses...))
		record(err)
		record(env.Authority.Revoke(context.Background(), env.Invalid, receipt))
	}
	if env.Restricted != nil {
		for _, use := range uses {
			_, err := env.Authority.Ensure(context.Background(), env.Restricted(use), request(NewKey(t), use))
			record(err)
		}
	}
	revoke(t, env, receipt)
	if env.Log != nil {
		artifacts = append(artifacts, env.Log())
	}

	for _, secret := range env.Secrets {
		require.NotEmpty(t, secret)
		for _, text := range artifacts {
			assert.NotContains(t, text, secret)
		}
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}

// errorsJoinText flattens a possibly joined error, including every wrapped message.
func errorsJoinText(err error) string {
	var parts []string
	for err != nil {
		parts = append(parts, err.Error())
		var multi interface{ Unwrap() []error }
		if errors.As(err, &multi) {
			for _, e := range multi.Unwrap() {
				parts = append(parts, errorsJoinText(e))
			}
			break
		}
		err = errors.Unwrap(err)
	}
	return strings.Join(parts, "\n")
}
