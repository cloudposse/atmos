package blob

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Capabilities describes what a store can enforce. It is computed without network access.
type Capabilities struct {
	// ConditionalPut reports that if-absent and if-match writes are enforced.
	ConditionalPut bool

	// ConditionalDelete reports that if-match deletes are enforced.
	ConditionalDelete bool

	// Multipart reports that bodies larger than one buffer stream without being held in memory.
	Multipart bool

	// Tags reports that object tags are stored and returned.
	Tags bool

	// Metadata reports that user metadata and content type are stored and returned.
	Metadata bool

	// Range reports that partial reads are supported.
	Range bool

	// Listing reports that prefix listing with pagination is supported.
	Listing bool

	// ListDelimiter reports that listing rolls keys up into common prefixes.
	ListDelimiter bool
}

// SupportsPut reports whether the store can enforce the put condition.
func (c Capabilities) SupportsPut(cond Condition) bool {
	defer perf.Track(nil, "blob.Capabilities.SupportsPut")()

	return !cond.IsConditional() || c.ConditionalPut
}

// CheckPut returns a clear ErrBlobConditionalUnsupported error when the store cannot enforce cond.
func (c Capabilities) CheckPut(store, key string, cond Condition) error {
	defer perf.Track(nil, "blob.Capabilities.CheckPut")()

	if c.SupportsPut(cond) {
		return nil
	}
	return NewError(KindUnsupported, OpPut, key, fmt.Errorf("%w: %s store cannot enforce conditional writes", errUtils.ErrBlobConditionalUnsupported, store))
}

// CheckDelete returns a clear ErrBlobConditionalUnsupported error when the store cannot enforce an if-match delete.
func (c Capabilities) CheckDelete(store, key string, opts DeleteOptions) error {
	defer perf.Track(nil, "blob.Capabilities.CheckDelete")()

	if opts.IfMatch == "" || c.ConditionalDelete {
		return nil
	}
	return NewError(KindUnsupported, OpDelete, key, fmt.Errorf("%w: %s store cannot enforce conditional deletes", errUtils.ErrBlobConditionalUnsupported, store))
}
