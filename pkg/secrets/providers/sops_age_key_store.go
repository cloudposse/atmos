package providers

import (
	"fmt"

	"github.com/getsops/sops/v3/keyservice"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/store"
)

// ageKeySource is the resolved age-key source from a provider spec: inline material, the store
// name (empty → file mode), the store key/path, and the file-mode key file path.
type ageKeySource struct {
	value     string
	storeName string
	path      string
	file      string
}

// parseAgeKeySpec resolves the age-key source from the provider spec. It accepts:
//   - `age_key:` as a nested object `{ store: file|<name>, path: <string>, value: <inline> }`;
//   - `age_key:` as a bare string (treated as inline `value`, back-compat);
//   - `age_key_file:` as a shorthand for file mode at an explicit path (back-compat).
//
// The reserved store value "file" selects the local file backend.
func parseAgeKeySpec(spec map[string]any) ageKeySource {
	var ak ageKeySource
	ak.file, _ = spec["age_key_file"].(string)

	switch v := spec["age_key"].(type) {
	case string:
		ak.value = v
	case map[string]any:
		ak.value, _ = v["value"].(string)
		ak.storeName, _ = v["store"].(string)
		ak.path, _ = v["path"].(string)
		if ak.storeName == "" || ak.storeName == ageKeyStoreFile {
			// File mode: the path (when given) is the key file path.
			ak.storeName = ""
			if ak.path != "" {
				ak.file = ak.path
			}
			ak.path = ""
		}
	}
	return ak
}

// ageKeyStoreErr reports a failure to read/write the age key from/to the configured `age_key.store`
// as ErrSopsAgeKey with actionable hints. The errors.Is(result, ErrSopsAgeKey) check holds.
func ageKeyStoreErr(storeName string, cause error) error {
	return errUtils.Build(ErrSopsAgeKey).
		WithCause(fmt.Errorf("store %q: %w", storeName, cause)).
		WithHintf("Ensure a store named %q is configured under `stores:` and holds the age private key.", storeName).
		WithHintf("Generate and store it with `atmos secret keygen` (writes the key into the `%s` store).", storeName).
		Err()
}

// ageKeyStoreTriple returns the (stack, component, key) under which this vault's age private key is
// stored: stack = vault name, component = "age-key", key = ageKeyPath (defaulting to the vault name).
// keygen (write) and the provider (read) use the same triple so the key round-trips.
func (p *sopsProvider) ageKeyStoreTriple() (string, string, string) {
	key := p.ageKeyPath
	if key == "" {
		key = p.name
	}
	return p.name, ageKeyStoreComponent, key
}

// writeKeyToStore writes the age private identity into the configured `age_key.store` at the
// provider-owned triple, returning a human-readable location for keygen output.
func (p *sopsProvider) writeKeyToStore(identity string) (string, error) {
	st, ok := p.stores[p.ageKeyStore]
	if !ok || st == nil {
		return "", ageKeyStoreErr(p.ageKeyStore, fmt.Errorf(errFmtWrapQuoted, ErrStoreNotFound, p.ageKeyStore))
	}
	stack, component, key := p.ageKeyStoreTriple()
	if err := st.Set(stack, component, key, identity); err != nil {
		return "", ageKeyStoreErr(p.ageKeyStore, err)
	}
	return fmt.Sprintf("store %q (%s/%s/%s)", p.ageKeyStore, stack, component, key), nil
}

// storeHasKey reports whether the configured `age_key.store` already holds this vault's key.
// Best-effort: any lookup error (store missing, no status capability) reports false so keygen runs.
func (p *sopsProvider) storeHasKey() bool {
	st, ok := p.stores[p.ageKeyStore]
	if !ok || st == nil {
		return false
	}
	ss, ok := st.(store.StatusStore)
	if !ok {
		return false
	}
	stack, component, key := p.ageKeyStoreTriple()
	has, err := ss.Has(stack, component, key)
	return err == nil && has
}

// ageKeyStoreClient builds a key service from the age private key held in the configured store
// (`age_key.store`). The store entry is provider-owned (written by `atmos secret keygen`).
func (p *sopsProvider) ageKeyStoreClient() (keyservice.KeyServiceClient, error) {
	st, ok := p.stores[p.ageKeyStore]
	if !ok || st == nil {
		return nil, ageKeyStoreErr(p.ageKeyStore, fmt.Errorf(errFmtWrapQuoted, ErrStoreNotFound, p.ageKeyStore))
	}
	stack, component, key := p.ageKeyStoreTriple()
	v, err := st.Get(stack, component, key)
	if err != nil {
		return nil, ageKeyStoreErr(p.ageKeyStore, err)
	}
	material, ok := v.(string)
	if !ok || material == "" {
		return nil, ageKeyStoreErr(p.ageKeyStore, fmt.Errorf("%w: store %q returned no age key at %q", ErrSopsAgeKey, p.ageKeyStore, key))
	}
	return ageClientFromKeyMaterial(material, func(e error) error { return ageKeyStoreErr(p.ageKeyStore, e) })
}
