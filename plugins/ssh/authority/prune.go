package authority

import (
	"context"
	"fmt"
)

// Prune revokes and deletes this authority's receipts for keys that no longer exist, and does
// nothing else: it enrolls no key and calls the authority only for those receipts. It is what a
// logout needs, where there may be no current key to enroll.
//
// Existing holds the fingerprints of the keys that still exist on this machine. Nil means unknown,
// and nothing is revoked then. Unlike Reconcile, no receipt is exempt because of its key name: a
// receipt whose key was deleted is withdrawn even when the identity that owned it still exists.
func Prune(ctx context.Context, p Params) (Result, error) {
	switch {
	case p.Authority == nil:
		return Result{Outcome: OutcomeFailed}, fmt.Errorf("%w: no authority", ErrInvalidRequest)
	case p.Store == nil:
		return Result{Outcome: OutcomeFailed}, fmt.Errorf("%w: no receipt store", ErrInvalidRequest)
	}
	// No current key: nothing is exempt from the sweep.
	p.KeyName, p.Key = "", nil
	r := &reconciler{Params: p, name: p.Authority.Name()}
	r.res.Outcome = OutcomeUnchanged
	r.cleanup(ctx)
	if len(r.res.Failures) > 0 {
		r.res.Outcome = OutcomeFailed
	}
	return r.res, r.res.Err()
}
