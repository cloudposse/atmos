package authority

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

// Outcome says what Reconcile did for the current key.
type Outcome string

const (
	// OutcomeUnchanged means a valid receipt already matched and the authority was not asked.
	OutcomeUnchanged Outcome = "unchanged"
	// OutcomeEnrolled means the key had no receipt and is now enrolled.
	OutcomeEnrolled Outcome = "enrolled"
	// OutcomeUpdated means uses were added or removed on an enrolled key.
	OutcomeUpdated Outcome = "updated"
	// OutcomeRefreshed means the authority was asked again and nothing about the uses changed.
	OutcomeRefreshed Outcome = "refreshed"
	// OutcomeFailed means the key is not enrolled as requested. See Result.Failures.
	OutcomeFailed Outcome = "failed"
)

// Step names the part of Reconcile that failed.
type Step string

const (
	// StepLoad is reading this key's receipt.
	StepLoad Step = "load-receipt"
	// StepEnsure is asking the authority to enroll the key.
	StepEnsure Step = "ensure"
	// StepRevokeDropped is withdrawing a use that is no longer requested.
	StepRevokeDropped Step = "revoke-dropped-use"
	// StepRevokeRotated is withdrawing the enrollment of a key that was replaced under the same name.
	StepRevokeRotated Step = "revoke-rotated-key"
	// StepRevokeOrphan is withdrawing the enrollment of a key that no longer exists.
	StepRevokeOrphan Step = "revoke-missing-key"
	// StepSave is writing a receipt.
	StepSave Step = "save-receipt"
	// StepDelete is removing a receipt after its enrollment was withdrawn.
	StepDelete Step = "delete-receipt"
	// StepList is listing the stored receipts.
	StepList Step = "list-receipts"
)

// StepError is one failure inside Reconcile. Every step that fails is recorded, so a caller can
// render exactly what is half done.
type StepError struct {
	Step    Step
	KeyName string
	Uses    []Use
	Err     error
}

// Error implements the error interface.
func (e *StepError) Error() string {
	if e.KeyName == "" {
		return fmt.Sprintf("%s: %v", e.Step, e.Err)
	}
	return fmt.Sprintf("%s (key %q): %v", e.Step, e.KeyName, e.Err)
}

// Unwrap returns the underlying error.
func (e *StepError) Unwrap() error { return e.Err }

// Params describe one reconciliation.
type Params struct {
	// Authority vouches for the key.
	Authority Authority
	// Credentials authenticate to the authority.
	Credentials map[string]string
	// Store holds the receipts.
	Store *Store
	// KeyName and Key identify the current key.
	KeyName string
	Key     ssh.PublicKey
	// Uses is what the key is to be trusted for.
	Uses []Use
	// Title labels the key on the authority.
	Title string
	// Principals and Target pass through to authorities that use them.
	Principals []string
	Target     string
	// Existing holds the fingerprints of the keys that still exist on this machine. Receipts of
	// this authority for other fingerprints are revoked. Nil means unknown, and no receipt is
	// revoked on this ground.
	Existing map[string]bool
	// Force asks the authority even when a valid receipt already matches.
	Force bool
	// Now is the clock used for expiry. Nil means time.Now.
	Now func() time.Time
}

// Result describes what Reconcile did.
type Result struct {
	// Outcome is the state of the current key.
	Outcome Outcome
	// Receipt is the current key's receipt. It is valid unless Outcome is OutcomeFailed, and then
	// it holds only what was saved of a partial enrollment, if anything.
	Receipt Receipt
	// Contacted reports whether the authority was called at all.
	Contacted bool
	// Added and Removed are the uses enrolled and withdrawn on the current key.
	Added   []Use
	Removed []Use
	// Revoked lists the receipts withdrawn because their key is gone or was replaced.
	Revoked []Receipt
	// Failures lists every step that failed. A non-empty list can accompany a usable Receipt, for
	// example when only the cleanup of a stale enrollment failed.
	Failures []*StepError
}

// Err joins the failures into one error, or returns nil when none occurred.
func (r Result) Err() error {
	errs := make([]error, len(r.Failures))
	for i, f := range r.Failures {
		errs[i] = f
	}
	return errors.Join(errs...)
}

// Reconcile brings the authority and the receipts into agreement with the key.
//
//  1. A valid, unexpired receipt for this key that covers exactly the requested uses is returned
//     without calling the authority.
//  2. Otherwise the authority is asked to Ensure the key, the receipt is saved, and uses that were
//     dropped from the request are revoked.
//  3. Receipts of this authority whose fingerprint is not in Existing are revoked and deleted.
//
// A receipt is deleted only after its enrollment was withdrawn. A receipt that is still needed to
// revoke something stays on disk.
func Reconcile(ctx context.Context, p Params) (Result, error) {
	if err := p.validate(); err != nil {
		return Result{Outcome: OutcomeFailed}, err
	}
	r := &reconciler{Params: p, name: p.Authority.Name(), fp: Fingerprint(p.Key), uses: NormalizeUses(p.Uses)}
	r.run(ctx)
	return r.res, r.res.Err()
}

func (p Params) validate() error {
	switch {
	case p.Authority == nil:
		return fmt.Errorf("%w: no authority", ErrInvalidRequest)
	case p.Store == nil:
		return fmt.Errorf("%w: no receipt store", ErrInvalidRequest)
	case p.KeyName == "":
		return fmt.Errorf("%w: no key name", ErrInvalidRequest)
	}
	return ValidateRequest(p.Authority, Request{KeyName: p.KeyName, Key: p.Key, Uses: p.Uses})
}

type reconciler struct {
	Params
	name string
	fp   string
	uses []Use
	res  Result
}

func (r *reconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *reconciler) fail(step Step, keyName string, uses []Use, err error) {
	r.res.Failures = append(r.res.Failures, &StepError{Step: step, KeyName: keyName, Uses: uses, Err: err})
}

func (r *reconciler) run(ctx context.Context) {
	old, found, ok := r.loadCurrent()
	if !ok {
		r.res.Outcome = OutcomeFailed
		return
	}
	if found && r.upToDate(old) {
		r.res.Outcome = OutcomeUnchanged
		r.res.Receipt = old
		r.cleanup(ctx)
		return
	}
	if found && old.Fingerprint != r.fp {
		if !r.revokeRotated(ctx, old) {
			r.res.Outcome = OutcomeFailed
			return
		}
		old, found = Receipt{}, false
	}
	r.enroll(ctx, old, found)
	if r.res.Outcome != OutcomeFailed {
		r.cleanup(ctx)
	}
}

// loadCurrent reads this key's receipt. It reports ok=false on a failure that must stop the run,
// because overwriting an unreadable receipt could lose a reference needed to revoke.
func (r *reconciler) loadCurrent() (old Receipt, found, ok bool) {
	old, err := r.Store.Load(r.name, r.KeyName)
	switch {
	case err == nil:
		return old, true, true
	case errors.Is(err, ErrReceiptNotFound):
		return Receipt{}, false, true
	default:
		r.fail(StepLoad, r.KeyName, nil, err)
		return Receipt{}, false, false
	}
}

func (r *reconciler) upToDate(old Receipt) bool {
	if r.Force || old.Fingerprint != r.fp || old.Expired(r.now()) {
		return false
	}
	return equalUses(old.Uses, r.uses) && equalUses(old.EnrolledUses(), r.uses)
}

// revokeRotated withdraws the receipt of a different key that used this key's name. The new
// receipt would overwrite its file, so the old one must be gone first.
func (r *reconciler) revokeRotated(ctx context.Context, old Receipt) bool {
	r.res.Contacted = true
	if err := r.Authority.Revoke(ctx, r.Credentials, old); err != nil {
		r.fail(StepRevokeRotated, old.KeyName, old.EnrolledUses(), err)
		return false
	}
	if err := r.Store.Delete(old.Authority, old.KeyName); err != nil {
		r.fail(StepDelete, old.KeyName, nil, err)
		return false
	}
	r.res.Revoked = append(r.res.Revoked, old)
	return true
}

func (r *reconciler) enroll(ctx context.Context, old Receipt, hadOld bool) {
	r.res.Contacted = true
	rec, err := r.Authority.Ensure(ctx, r.Credentials, Request{
		KeyName: r.KeyName, Key: r.Key, Uses: r.uses, Title: r.Title,
		Principals: r.Principals, Target: r.Target,
	})
	rec = r.complete(rec, old, hadOld, err == nil)
	if err != nil {
		r.res.Outcome = OutcomeFailed
		r.fail(StepEnsure, r.KeyName, r.uses, err)
		r.savePartial(rec, old)
		return
	}
	enrolled := old.EnrolledUses()
	if !hadOld {
		enrolled = nil
	}
	added := differenceUses(r.uses, enrolled)
	dropped := differenceUses(enrolled, r.uses)
	kept := r.revokeDropped(ctx, old, dropped)
	for use, ref := range kept {
		rec.References[use] = ref
	}
	if err := r.Store.Save(rec); err != nil {
		r.res.Outcome = OutcomeFailed
		r.fail(StepSave, r.KeyName, r.uses, err)
		return
	}
	r.res.Receipt = rec
	r.res.Added = added
	r.res.Removed = differenceUses(dropped, keysOf(kept))
	r.res.Outcome = outcomeFor(hadOld, len(r.res.Added)+len(r.res.Removed) > 0)
}

func outcomeFor(hadOld, changed bool) Outcome {
	switch {
	case !hadOld:
		return OutcomeEnrolled
	case changed:
		return OutcomeUpdated
	default:
		return OutcomeRefreshed
	}
}

// complete fills the identity fields of a receipt the authority returned and carries over what
// the old receipt knew.
func (r *reconciler) complete(rec, old Receipt, hadOld, succeeded bool) Receipt {
	rec.Authority, rec.KeyName, rec.Fingerprint = r.name, r.KeyName, r.fp
	if rec.PublicKey == "" {
		rec.PublicKey = AuthorizedKey(r.Key)
	}
	if rec.References == nil {
		rec.References = map[Use]string{}
	}
	if succeeded && len(rec.Uses) == 0 {
		rec.Uses = r.uses
	}
	rec.Uses = NormalizeUses(rec.Uses)
	switch {
	case hadOld && !old.Created.IsZero():
		rec.Created = old.Created
	case rec.Created.IsZero():
		rec.Created = r.now().UTC()
	}
	return rec
}

// savePartial persists what a failed Ensure did enroll, merged with the old references, so a later
// run can still revoke it.
func (r *reconciler) savePartial(rec, old Receipt) {
	if len(rec.References) == 0 {
		return
	}
	for use, ref := range old.References {
		if _, ok := rec.References[use]; !ok {
			rec.References[use] = ref
		}
	}
	rec.Uses = NormalizeUses(append(rec.Uses, old.Uses...))
	if err := r.Store.Save(rec); err != nil {
		r.fail(StepSave, r.KeyName, nil, err)
		return
	}
	r.res.Receipt = rec
}

// revokeDropped withdraws the dropped uses. It returns the references that could not be withdrawn,
// which the new receipt must keep.
func (r *reconciler) revokeDropped(ctx context.Context, old Receipt, dropped []Use) map[Use]string {
	if len(dropped) == 0 {
		return nil
	}
	refs := map[Use]string{}
	for _, use := range dropped {
		if ref, ok := old.References[use]; ok {
			refs[use] = ref
		}
	}
	partial := old
	partial.Uses, partial.References = dropped, refs
	err := r.Authority.Revoke(ctx, r.Credentials, partial)
	if err == nil {
		return nil
	}
	r.fail(StepRevokeDropped, r.KeyName, dropped, err)
	return refs
}

func keysOf(m map[Use]string) []Use {
	out := make([]Use, 0, len(m))
	for use := range m {
		out = append(out, use)
	}
	return out
}

// cleanup revokes this authority's receipts for keys that no longer exist.
func (r *reconciler) cleanup(ctx context.Context) {
	if r.Existing == nil {
		return
	}
	receipts, err := r.Store.List()
	if err != nil {
		r.fail(StepList, "", nil, err)
	}
	for _, rec := range receipts {
		if rec.Authority != r.name || rec.KeyName == r.KeyName || rec.Fingerprint == r.fp || r.Existing[rec.Fingerprint] {
			continue
		}
		r.revokeOrphan(ctx, rec)
	}
}

func (r *reconciler) revokeOrphan(ctx context.Context, rec Receipt) {
	r.res.Contacted = true
	if err := r.Authority.Revoke(ctx, r.Credentials, rec); err != nil {
		r.fail(StepRevokeOrphan, rec.KeyName, rec.EnrolledUses(), err)
		return
	}
	if err := r.Store.Delete(rec.Authority, rec.KeyName); err != nil {
		r.fail(StepDelete, rec.KeyName, nil, err)
		return
	}
	r.res.Revoked = append(r.res.Revoked, rec)
}
