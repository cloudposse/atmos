package authority_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/authority/authoritytest"
	"github.com/cloudposse/atmos/plugins/ssh/authority/github"
)

var errBoom = errors.New("boom")

var bothUses = []authority.Use{authority.UseSigning, authority.UseAuthentication}

type rig struct {
	t     *testing.T
	fake  *authoritytest.Fake
	store *authority.Store
	key   ssh.PublicKey
}

func newRig(t *testing.T) *rig {
	t.Helper()
	return &rig{t: t, fake: authoritytest.NewFake("fake"), store: newStore(t), key: authoritytest.NewKey(t)}
}

func (r *rig) params(uses ...authority.Use) authority.Params {
	return authority.Params{
		Authority:   r.fake,
		Credentials: map[string]string{authority.CredentialToken: authoritytest.FakeToken},
		Store:       r.store,
		KeyName:     "ops",
		Key:         r.key,
		Uses:        uses,
		Title:       "ops@host (Atmos)",
		Existing:    authoritytest.Fingerprints(r.key),
	}
}

func (r *rig) run(p authority.Params) authority.Result {
	r.t.Helper()
	res, err := authority.Reconcile(context.Background(), p)
	assert.Equal(r.t, res.Err(), err)
	return res
}

func (r *rig) mustRun(p authority.Params) authority.Result {
	r.t.Helper()
	res, err := authority.Reconcile(context.Background(), p)
	require.NoError(r.t, err)
	return res
}

// enrollOther enrolls a second key under name without cleaning anything up.
func (r *rig) enrollOther(name string, key ssh.PublicKey, uses ...authority.Use) {
	r.t.Helper()
	p := r.params(uses...)
	p.KeyName, p.Key, p.Existing = name, key, nil
	r.mustRun(p)
}

func (r *rig) saved(name string) authority.Receipt {
	r.t.Helper()
	rec, err := r.store.Load("fake", name)
	require.NoError(r.t, err)
	return rec
}

func TestReconcileEnrollsThenIsUnchangedWithoutCallingTheAuthority(t *testing.T) {
	r := newRig(t)

	first := r.mustRun(r.params(bothUses...))
	assert.Equal(t, authority.OutcomeEnrolled, first.Outcome)
	assert.True(t, first.Contacted)
	assert.Equal(t, []authority.Use{authority.UseAuthentication, authority.UseSigning}, first.Added)
	assert.Equal(t, 1, r.fake.CallCount("ensure"))
	assert.Equal(t, first.Receipt, r.saved("ops"))
	assert.Equal(t, authority.Fingerprint(r.key), first.Receipt.Fingerprint)
	assert.Equal(t, "fake-user", first.Receipt.Subject)

	second := r.mustRun(r.params(bothUses...))
	assert.Equal(t, authority.OutcomeUnchanged, second.Outcome)
	assert.False(t, second.Contacted)
	assert.Empty(t, second.Added)
	assert.Equal(t, first.Receipt, second.Receipt)
	assert.Len(t, r.fake.Calls(), 1, "the fast path made no call")
}

func TestReconcileFastPathNeedsNoNetworkAtAll(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a, err := github.New(authority.Options{BaseURL: srv.URL, HTTPClient: srv.Client()})
	require.NoError(t, err)
	env := srv.Environment(a)
	store := newStore(t)
	key := authoritytest.NewKey(t)
	p := authority.Params{
		Authority: a, Credentials: env.Credentials, Store: store, KeyName: "ops", Key: key,
		Uses: bothUses, Title: "ops@host (Atmos)", Existing: authoritytest.Fingerprints(key),
	}

	res, err := authority.Reconcile(context.Background(), p)
	require.NoError(t, err)
	assert.Equal(t, authority.OutcomeEnrolled, res.Outcome)
	assert.Equal(t, authoritytest.Login, res.Receipt.Subject)
	assert.NotEmpty(t, srv.Requests())

	srv.ResetRequests()
	p.Credentials = map[string]string{} // the token is not even needed on the fast path
	res, err = authority.Reconcile(context.Background(), p)
	require.NoError(t, err)
	assert.Equal(t, authority.OutcomeUnchanged, res.Outcome)
	assert.Empty(t, srv.Requests(), "no request reached GitHub")
}

func TestReconcileRotatedKeyRevokesTheOldKeyOnGitHub(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a, err := github.New(authority.Options{BaseURL: srv.URL, HTTPClient: srv.Client()})
	require.NoError(t, err)
	env := srv.Environment(a)
	store := newStore(t)
	oldKey, newKey := authoritytest.NewKey(t), authoritytest.NewKey(t)
	p := authority.Params{
		Authority: a, Credentials: env.Credentials, Store: store, KeyName: "ops", Key: oldKey,
		Uses: bothUses, Existing: authoritytest.Fingerprints(oldKey),
	}
	_, err = authority.Reconcile(context.Background(), p)
	require.NoError(t, err)

	p.Key, p.Existing = newKey, authoritytest.Fingerprints(newKey)
	res, err := authority.Reconcile(context.Background(), p)
	require.NoError(t, err)

	assert.Equal(t, authority.OutcomeEnrolled, res.Outcome)
	require.Len(t, res.Revoked, 1)
	assert.Equal(t, authority.Fingerprint(oldKey), res.Revoked[0].Fingerprint)
	assert.Zero(t, srv.KeyCount(oldKey, authority.UseSigning)+srv.KeyCount(oldKey, authority.UseAuthentication))
	assert.Equal(t, 1, srv.KeyCount(newKey, authority.UseSigning))
	assert.Equal(t, 1, srv.KeyCount(newKey, authority.UseAuthentication))
	stored, err := store.Load("github", "ops")
	require.NoError(t, err)
	assert.Equal(t, authority.Fingerprint(newKey), stored.Fingerprint)
}

func TestReconcileChangedUses(t *testing.T) {
	tests := []struct {
		name        string
		first       []authority.Use
		second      []authority.Use
		wantOutcome authority.Outcome
		added       []authority.Use
		removed     []authority.Use
	}{
		{"use added", []authority.Use{authority.UseSigning}, bothUses, authority.OutcomeUpdated, []authority.Use{authority.UseAuthentication}, nil},
		{"use dropped", bothUses, []authority.Use{authority.UseSigning}, authority.OutcomeUpdated, nil, []authority.Use{authority.UseAuthentication}},
		{"use swapped", []authority.Use{authority.UseSigning}, []authority.Use{authority.UseAuthentication}, authority.OutcomeUpdated, []authority.Use{authority.UseAuthentication}, []authority.Use{authority.UseSigning}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			first := r.mustRun(r.params(tt.first...))

			res := r.mustRun(r.params(tt.second...))

			assert.Equal(t, tt.wantOutcome, res.Outcome)
			assert.Equal(t, tt.added, res.Added)
			assert.Equal(t, tt.removed, res.Removed)
			assert.Equal(t, authority.NormalizeUses(tt.second), res.Receipt.EnrolledUses(), "references and uses agree")
			assert.Equal(t, res.Receipt, r.saved("ops"))
			assert.Equal(t, first.Receipt.Created, res.Receipt.Created, "the receipt keeps its issue time")
			for _, use := range bothUses {
				want := 0
				if contains(tt.second, use) {
					want = 1
				}
				assert.Equal(t, want, r.fake.Count(r.key, use), use)
			}
		})
	}
}

func contains(uses []authority.Use, use authority.Use) bool {
	for _, u := range uses {
		if u == use {
			return true
		}
	}
	return false
}

func TestReconcileForceAndExpiryAskTheAuthorityAgain(t *testing.T) {
	r := newRig(t)
	r.mustRun(r.params(bothUses...))

	forced := r.params(bothUses...)
	forced.Force = true
	res := r.mustRun(forced)
	assert.Equal(t, authority.OutcomeRefreshed, res.Outcome)
	assert.True(t, res.Contacted)
	assert.Empty(t, res.Added)
	assert.Empty(t, res.Removed)
	assert.Equal(t, 2, r.fake.CallCount("ensure"))
	assert.Equal(t, 1, r.fake.Count(r.key, authority.UseSigning), "forcing creates no duplicate")

	rec := r.saved("ops")
	rec.ExpiresAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, r.store.Save(rec))

	before := r.params(bothUses...)
	before.Now = func() time.Time { return time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC) }
	assert.Equal(t, authority.OutcomeUnchanged, r.mustRun(before).Outcome, "not yet expired")
	assert.Equal(t, 2, r.fake.CallCount("ensure"))

	after := r.params(bothUses...)
	after.Now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	assert.Equal(t, authority.OutcomeRefreshed, r.mustRun(after).Outcome, "expired")
	assert.Equal(t, 3, r.fake.CallCount("ensure"))
}

func TestReconcileRotatedKeyUnderTheSameName(t *testing.T) {
	r := newRig(t)
	oldKey := r.key
	r.mustRun(r.params(bothUses...))

	r.key = authoritytest.NewKey(t)
	res := r.mustRun(r.params(bothUses...))

	assert.Equal(t, authority.OutcomeEnrolled, res.Outcome)
	require.Len(t, res.Revoked, 1)
	assert.Equal(t, authority.Fingerprint(oldKey), res.Revoked[0].Fingerprint)
	assert.Zero(t, r.fake.Count(oldKey, authority.UseSigning))
	assert.Equal(t, 1, r.fake.Count(r.key, authority.UseSigning))
	assert.Equal(t, authority.Fingerprint(r.key), r.saved("ops").Fingerprint)
}

func TestReconcileRotationKeepsTheOldReceiptWhenRevokeFails(t *testing.T) {
	r := newRig(t)
	oldKey := r.key
	r.mustRun(r.params(bothUses...))
	r.key = authoritytest.NewKey(t)
	r.fake.FailNextRevoke(errBoom)

	res := r.run(r.params(bothUses...))

	assert.Equal(t, authority.OutcomeFailed, res.Outcome)
	require.Len(t, res.Failures, 1)
	assert.Equal(t, authority.StepRevokeRotated, res.Failures[0].Step)
	assert.ErrorIs(t, res.Err(), errBoom)
	assert.Equal(t, 1, r.fake.CallCount("ensure"), "the new key was not enrolled over the old receipt")
	assert.Equal(t, authority.Fingerprint(oldKey), r.saved("ops").Fingerprint, "the old receipt is still there to retry")

	res = r.mustRun(r.params(bothUses...))
	assert.Equal(t, authority.OutcomeEnrolled, res.Outcome)
	assert.Zero(t, r.fake.Count(oldKey, authority.UseSigning))
}

func TestReconcileRevokesReceiptsOfKeysThatNoLongerExist(t *testing.T) {
	r := newRig(t)
	gone, kept := authoritytest.NewKey(t), authoritytest.NewKey(t)
	r.enrollOther("gone", gone, authority.UseSigning)
	r.enrollOther("kept", kept, authority.UseSigning)
	foreign := sampleReceipt("foreign", "SHA256:foreign")
	foreign.Authority = "someone-else"
	require.NoError(t, r.store.Save(foreign))

	p := r.params(authority.UseSigning)
	p.Existing = authoritytest.Fingerprints(r.key, kept)
	res := r.mustRun(p)

	require.Len(t, res.Revoked, 1)
	assert.Equal(t, "gone", res.Revoked[0].KeyName)
	_, err := r.store.Load("fake", "gone")
	assert.ErrorIs(t, err, authority.ErrReceiptNotFound)
	assert.Zero(t, r.fake.Count(gone, authority.UseSigning))
	assert.Equal(t, 1, r.fake.Count(kept, authority.UseSigning), "a key that still exists stays enrolled")
	_, err = r.store.Load("someone-else", "foreign")
	assert.NoError(t, err, "another authority's receipts are not ours to revoke")
}

func TestReconcileCleansUpEvenOnTheFastPath(t *testing.T) {
	r := newRig(t)
	r.mustRun(r.params(authority.UseSigning))
	r.enrollOther("gone", authoritytest.NewKey(t), authority.UseSigning)
	ensures := r.fake.CallCount("ensure")

	res := r.mustRun(r.params(authority.UseSigning))

	assert.Equal(t, authority.OutcomeUnchanged, res.Outcome)
	assert.Equal(t, ensures, r.fake.CallCount("ensure"))
	require.Len(t, res.Revoked, 1)
	assert.True(t, res.Contacted)
}

func TestReconcileWithUnknownExistingSetRevokesNothing(t *testing.T) {
	r := newRig(t)
	r.enrollOther("other", authoritytest.NewKey(t), authority.UseSigning)

	p := r.params(authority.UseSigning)
	p.Existing = nil
	res := r.mustRun(p)

	assert.Empty(t, res.Revoked)
	assert.Zero(t, r.fake.CallCount("revoke"))
	_, err := r.store.Load("fake", "other")
	assert.NoError(t, err)
}

func TestReconcileTreatsTheCurrentKeyAsExistingEvenIfOmitted(t *testing.T) {
	r := newRig(t)
	r.mustRun(r.params(authority.UseSigning))
	p := r.params(authority.UseSigning)
	p.Existing = map[string]bool{}
	res := r.mustRun(p)
	assert.Empty(t, res.Revoked)
	assert.Equal(t, 1, r.fake.Count(r.key, authority.UseSigning))
}

func TestReconcileKeepsAnOrphanReceiptWhenItsRevokeFails(t *testing.T) {
	r := newRig(t)
	gone := authoritytest.NewKey(t)
	r.enrollOther("gone", gone, authority.UseSigning)
	r.fake.FailNextRevoke(errBoom)

	res := r.run(r.params(authority.UseSigning))

	assert.Equal(t, authority.OutcomeEnrolled, res.Outcome, "the current key is fine")
	assert.Empty(t, res.Revoked)
	require.Len(t, res.Failures, 1)
	assert.Equal(t, authority.StepRevokeOrphan, res.Failures[0].Step)
	assert.Equal(t, "gone", res.Failures[0].KeyName)
	assert.Equal(t, []authority.Use{authority.UseSigning}, res.Failures[0].Uses)
	assert.ErrorIs(t, res.Err(), errBoom)
	assert.Equal(t, authority.Fingerprint(gone), r.saved("gone").Fingerprint, "the receipt is kept for the next try")

	res = r.mustRun(r.params(authority.UseSigning))
	require.Len(t, res.Revoked, 1)
	_, err := r.store.Load("fake", "gone")
	assert.ErrorIs(t, err, authority.ErrReceiptNotFound)
}

func TestReconcileWhenEnsureFailsSavesNothingAndSkipsCleanup(t *testing.T) {
	r := newRig(t)
	gone := authoritytest.NewKey(t)
	r.enrollOther("gone", gone, authority.UseSigning)
	r.fake.FailNextEnsure(&authority.Error{Authority: "fake", Err: authority.ErrUnauthorized})

	res := r.run(r.params(authority.UseSigning))

	assert.Equal(t, authority.OutcomeFailed, res.Outcome)
	require.Len(t, res.Failures, 1)
	assert.Equal(t, authority.StepEnsure, res.Failures[0].Step)
	assert.ErrorIs(t, res.Err(), authority.ErrUnauthorized)
	_, err := r.store.Load("fake", "ops")
	assert.ErrorIs(t, err, authority.ErrReceiptNotFound)
	assert.Zero(t, r.fake.CallCount("revoke"), "no cleanup against an authority that just refused us")
	_, err = r.store.Load("fake", "gone")
	assert.NoError(t, err)
}

func TestReconcileFailureInTheMiddleKeepsWhatWasEnrolled(t *testing.T) {
	r := newRig(t)
	r.fake.FailEnsureOnUse(authority.UseSigning, errBoom) // authentication sorts first and succeeds

	res := r.run(r.params(bothUses...))

	assert.Equal(t, authority.OutcomeFailed, res.Outcome)
	require.Len(t, res.Failures, 1)
	assert.ErrorIs(t, res.Err(), errBoom)
	partial := r.saved("ops")
	assert.Equal(t, []authority.Use{authority.UseAuthentication}, partial.EnrolledUses())
	assert.Equal(t, partial, res.Receipt)
	assert.Equal(t, 1, r.fake.Count(r.key, authority.UseAuthentication))

	// The partial receipt is what lets a later run clean up.
	later := r.params(bothUses...)
	r.fake.FailEnsureOnUse(authority.UseSigning, nil)
	res = r.mustRun(later)
	assert.Equal(t, authority.OutcomeUpdated, res.Outcome)
	assert.Equal(t, []authority.Use{authority.UseSigning}, res.Added)

	r.key = authoritytest.NewKey(t)
	res = r.mustRun(r.params(bothUses...))
	require.Len(t, res.Revoked, 1)
}

func TestReconcilePartialFailureOnAnUpdateKeepsOldReferences(t *testing.T) {
	r := newRig(t)
	first := r.mustRun(r.params(authority.UseSigning))
	r.fake.FailEnsureOnUse(authority.UseAuthentication, errBoom)

	res := r.run(r.params(bothUses...))

	assert.Equal(t, authority.OutcomeFailed, res.Outcome)
	assert.Equal(t, first.Receipt.References[authority.UseSigning], r.saved("ops").References[authority.UseSigning])
	assert.Equal(t, first.Receipt.Created, r.saved("ops").Created)
}

func TestReconcileDroppedUseThatCannotBeRevokedIsRetriedLater(t *testing.T) {
	r := newRig(t)
	first := r.mustRun(r.params(bothUses...))
	r.fake.FailNextRevoke(errBoom)

	res := r.run(r.params(authority.UseSigning))

	assert.Equal(t, authority.OutcomeRefreshed, res.Outcome, "the current key is usable and nothing was withdrawn")
	require.Len(t, res.Failures, 1)
	assert.Equal(t, authority.StepRevokeDropped, res.Failures[0].Step)
	assert.Equal(t, []authority.Use{authority.UseAuthentication}, res.Failures[0].Uses)
	assert.Empty(t, res.Removed)
	kept := r.saved("ops")
	assert.Equal(t, first.Receipt.References[authority.UseAuthentication], kept.References[authority.UseAuthentication], "the reference needed to revoke is kept")
	assert.Equal(t, 1, r.fake.Count(r.key, authority.UseAuthentication))

	res = r.mustRun(r.params(authority.UseSigning))
	assert.Equal(t, authority.OutcomeUpdated, res.Outcome)
	assert.Equal(t, []authority.Use{authority.UseAuthentication}, res.Removed)
	assert.Zero(t, r.fake.Count(r.key, authority.UseAuthentication))
	assert.Equal(t, []authority.Use{authority.UseSigning}, r.saved("ops").EnrolledUses())

	assert.Equal(t, authority.OutcomeUnchanged, r.mustRun(r.params(authority.UseSigning)).Outcome)
}

func TestReconcileStopsOnACorruptReceiptForTheKey(t *testing.T) {
	r := newRig(t)
	r.mustRun(r.params(authority.UseSigning))
	files, err := filepath.Glob(filepath.Join(r.store.Dir(), "*.json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(files[0], []byte("garbage"), 0o600))
	calls := len(r.fake.Calls())

	res := r.run(r.params(authority.UseSigning))

	assert.Equal(t, authority.OutcomeFailed, res.Outcome)
	require.Len(t, res.Failures, 1)
	assert.Equal(t, authority.StepLoad, res.Failures[0].Step)
	assert.ErrorIs(t, res.Err(), authority.ErrReceiptCorrupt)
	assert.Len(t, r.fake.Calls(), calls)
	data, err := os.ReadFile(files[0])
	require.NoError(t, err)
	assert.Equal(t, "garbage", string(data), "the file was not overwritten")
}

func TestReconcileReportsAnUnreadableReceiptDirectoryDuringCleanup(t *testing.T) {
	r := newRig(t)
	r.mustRun(r.params(authority.UseSigning))
	require.NoError(t, os.WriteFile(filepath.Join(r.store.Dir(), "junk.json"), []byte("nope"), 0o600))

	res := r.run(r.params(authority.UseSigning))

	assert.Equal(t, authority.OutcomeUnchanged, res.Outcome)
	require.Len(t, res.Failures, 1)
	assert.Equal(t, authority.StepList, res.Failures[0].Step)
	assert.Contains(t, res.Failures[0].Error(), "list-receipts")
}

func TestReconcileRejectsBadParamsBeforeAnyCall(t *testing.T) {
	r := newRig(t)
	tests := []struct {
		name string
		edit func(*authority.Params)
		want error
	}{
		{"no authority", func(p *authority.Params) { p.Authority = nil }, authority.ErrInvalidRequest},
		{"no store", func(p *authority.Params) { p.Store = nil }, authority.ErrInvalidRequest},
		{"no key name", func(p *authority.Params) { p.KeyName = "" }, authority.ErrInvalidRequest},
		{"no key", func(p *authority.Params) { p.Key = nil }, authority.ErrInvalidRequest},
		{"no uses", func(p *authority.Params) { p.Uses = nil }, authority.ErrInvalidRequest},
		{"unsupported use", func(p *authority.Params) { p.Uses = []authority.Use{"teleportation"} }, authority.ErrUnsupportedUse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := r.params(authority.UseSigning)
			tt.edit(&p)
			res, err := authority.Reconcile(context.Background(), p)
			assert.ErrorIs(t, err, tt.want)
			assert.Equal(t, authority.OutcomeFailed, res.Outcome)
			assert.Empty(t, r.fake.Calls())
		})
	}
}

func TestReconcileReportsASaveFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a directory the process cannot write")
	}
	r := newRig(t)
	require.NoError(t, os.MkdirAll(r.store.Dir(), 0o700))
	require.NoError(t, os.Chmod(r.store.Dir(), 0o500))
	t.Cleanup(func() { _ = os.Chmod(r.store.Dir(), 0o700) })

	res, err := authority.Reconcile(context.Background(), r.params(authority.UseSigning))

	require.Error(t, err)
	assert.Equal(t, authority.OutcomeFailed, res.Outcome)
	require.Len(t, res.Failures, 1)
	assert.Equal(t, authority.StepSave, res.Failures[0].Step)
	assert.Equal(t, 1, r.fake.Count(r.key, authority.UseSigning), "the enrollment exists but its receipt could not be written")
}
