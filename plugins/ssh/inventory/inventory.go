// Package inventory lists the keys that exist on this machine across every key store.
package inventory

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/keystore"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
)

// List returns the keys of every registered store, ordered by store and name. A store that does
// not exist on this platform is skipped. A store that fails for another reason is reported in the
// returned error together with the keys of the stores that worked, so a caller that needs a
// complete picture can tell it does not have one.
func List(ctx context.Context, open agent.StoreOpener, p *paths.Paths) ([]keystore.Info, error) {
	var keys []keystore.Info
	var errs []error
	for _, kind := range keystore.Kinds() {
		store, err := open(kind, p.StoreDir(kind))
		if err == nil {
			var infos []keystore.Info
			infos, err = store.List(ctx)
			keys = append(keys, infos...)
		}
		if err != nil && !errors.Is(err, keystore.ErrUnsupportedPlatform) {
			errs = append(errs, fmt.Errorf("list %s keys: %w", kind, err))
		}
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i].Store != keys[j].Store {
			return keys[i].Store < keys[j].Store
		}
		return keys[i].Name < keys[j].Name
	})
	return keys, errors.Join(errs...)
}

// Fingerprints returns the fingerprints of keys as a set. It returns nil, meaning unknown, when
// err is not nil, because a partial set would make a live key look gone.
func Fingerprints(keys []keystore.Info, err error) map[string]bool {
	if err != nil {
		return nil
	}
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		if k.PublicKey != nil {
			set[authority.Fingerprint(k.PublicKey)] = true
		}
	}
	return set
}
