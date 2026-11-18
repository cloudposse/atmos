package authority_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/authority/authoritytest"
)

func TestPruneRevokesOnlyReceiptsWhoseKeyIsGone(t *testing.T) {
	r := newRig(t)
	gone := authoritytest.NewKey(t)
	r.enrollOther("old", gone, authority.UseSigning)
	setup := r.params(authority.UseSigning)
	setup.Existing = nil
	r.mustRun(setup)
	require.Equal(t, 1, r.fake.Count(gone, authority.UseSigning))

	p := r.params()
	p.Existing = authoritytest.Fingerprints(r.key)
	res, err := authority.Prune(context.Background(), p)
	require.NoError(t, err)

	assert.Len(t, res.Revoked, 1)
	assert.Equal(t, "old", res.Revoked[0].KeyName)
	assert.Zero(t, r.fake.Count(gone, authority.UseSigning), "the missing key was withdrawn")
	assert.Equal(t, 1, r.fake.Count(r.key, authority.UseSigning), "a live key stays enrolled")
	_, err = r.store.Load("fake", "old")
	assert.ErrorIs(t, err, authority.ErrReceiptNotFound)
}

func TestPruneRevokesTheReceiptOfADeletedKeyUnderTheSameName(t *testing.T) {
	r := newRig(t)
	r.mustRun(r.params(authority.UseSigning))

	p := r.params()
	p.Existing = map[string]bool{}
	res, err := authority.Prune(context.Background(), p)
	require.NoError(t, err)

	assert.Len(t, res.Revoked, 1)
	assert.Zero(t, r.fake.Count(r.key, authority.UseSigning))
}

func TestPruneDoesNothingWhenTheKeysAreUnknown(t *testing.T) {
	r := newRig(t)
	r.mustRun(r.params(authority.UseSigning))
	before := len(r.fake.Calls())

	p := r.params()
	p.Existing = nil
	res, err := authority.Prune(context.Background(), p)
	require.NoError(t, err)

	assert.Empty(t, res.Revoked)
	assert.False(t, res.Contacted)
	assert.Len(t, r.fake.Calls(), before)
}

func TestPruneKeepsTheReceiptWhenRevocationFails(t *testing.T) {
	r := newRig(t)
	r.mustRun(r.params(authority.UseSigning))
	r.fake.FailNextRevoke(errBoom)

	p := r.params()
	p.Existing = map[string]bool{}
	res, err := authority.Prune(context.Background(), p)

	require.ErrorIs(t, err, errBoom)
	assert.Equal(t, authority.OutcomeFailed, res.Outcome)
	_, loadErr := r.store.Load("fake", "ops")
	assert.NoError(t, loadErr, "the receipt stays so a later run can retry")
}

func TestPruneRejectsIncompleteParams(t *testing.T) {
	_, err := authority.Prune(context.Background(), authority.Params{})
	assert.ErrorIs(t, err, authority.ErrInvalidRequest)
	_, err = authority.Prune(context.Background(), authority.Params{Authority: authoritytest.NewFake("fake")})
	assert.ErrorIs(t, err, authority.ErrInvalidRequest)
}
