package secrets

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ImportSource identifies an existing store coordinate to adopt a value from. Stack and
// Component are raw source path segments (typically copied verbatim from a legacy
// `!store <store> <stack> <component> <key>` expression) — they need not name a real Atmos
// stack or component, and either may be empty to omit that segment from the source path.
type ImportSource struct {
	// Store is the source store name. Empty selects the declaration's own `store:`.
	Store string
	// Stack is the source stack path segment (raw, not validated against real stacks).
	Stack string
	// Component is the source component path segment (raw; empty for stack-scoped legacy paths).
	Component string
	// Key is the source key. Empty selects the declaration name.
	Key string
}

// ImportFromStore copies an existing secret value from a source store coordinate into the
// declared secret's computed coordinate — terraform-import-style adoption that brings a value
// created outside the secrets subsystem (e.g. legacy `!store` usage) under management. The
// source value is read, registered with the masker, and written through the declaration's
// normal Set path (sensitivity flag, scope-derived coordinate); the source is never modified
// or deleted. With dryRun the source is read — proving it exists and is accessible — but
// nothing is written.
func (s *Service) ImportFromStore(name string, src ImportSource, dryRun bool) error {
	defer perf.Track(s.atmosConfig, "secrets.Service.ImportFromStore")()

	decl, err := s.declarationFor(name)
	if err != nil {
		return err
	}

	storeName := src.Store
	if storeName == "" {
		if decl.BackendType != BackendStore || decl.BackendName == "" {
			return fmt.Errorf("%w: secret %q (pass an explicit source store)", ErrImportSourceStore, name)
		}
		storeName = decl.BackendName
	}
	srcStore, ok := s.atmosConfig.Stores[storeName]
	if !ok || srcStore == nil {
		return fmt.Errorf("%w: %q", ErrStoreNotFound, storeName)
	}

	key := src.Key
	if key == "" {
		key = decl.Name
	}

	value, err := srcStore.Get(src.Stack, src.Component, key)
	if err != nil {
		return fmt.Errorf("%w: store %q (stack=%q component=%q key=%q): %w",
			ErrImportSourceRead, storeName, src.Stack, src.Component, key, err)
	}

	// Register before any further handling so the value can never leak into output unmasked.
	io.RegisterSecretValue(value)

	if dryRun {
		return nil
	}
	return s.Set(name, value)
}
