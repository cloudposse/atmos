package blob

import (
	"context"
	"io"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

//go:generate go run go.uber.org/mock/mockgen -package blob -destination mock_store.go github.com/cloudposse/atmos/pkg/artifact/blob Store

// UnknownSize is the Size value of a [PutOptions] whose body length is not known in advance.
const UnknownSize int64 = -1

// Store is transport-only object I/O. Keys are slash-separated and already fully formed by the
// caller; a Store never interprets them beyond validation. Every method honors context
// cancellation. Implementations are safe for concurrent use.
type Store interface {
	// Name returns the implementation name, for example "local" or "s3".
	Name() string

	// Capabilities describes what the store can enforce without network access.
	Capabilities() Capabilities

	// Put streams body to key. An unmet or unenforceable [PutOptions.Condition] fails the call
	// without changing the object. Returned info describes the stored object, including its ETag.
	Put(ctx context.Context, key string, body io.Reader, opts PutOptions) (*ObjectInfo, error)

	// Get opens key for streaming. Callers must close the returned body. A missing object fails
	// with ErrBlobNotFound.
	Get(ctx context.Context, key string, opts GetOptions) (*Object, error)

	// Stat returns object attributes without reading the body. A missing object fails with
	// ErrBlobNotFound.
	Stat(ctx context.Context, key string, opts StatOptions) (*ObjectInfo, error)

	// Delete removes key. Deleting a missing object without a condition succeeds. With
	// [DeleteOptions.IfMatch] set, a missing object fails with ErrBlobNotFound and a different ETag
	// fails with ErrBlobPreconditionFailed.
	Delete(ctx context.Context, key string, opts DeleteOptions) error

	// List returns one lexicographically ordered page of objects under a prefix. Pass the returned
	// NextCursor back to continue; an empty NextCursor means the listing is complete.
	List(ctx context.Context, opts ListOptions) (*ListPage, error)
}

// ConditionKind selects how a write is guarded.
type ConditionKind int

// Condition kinds.
const (
	// ConditionNone writes unconditionally.
	ConditionNone ConditionKind = iota

	// ConditionIfAbsent writes only when the key does not exist (S3 If-None-Match: *).
	ConditionIfAbsent

	// ConditionIfMatch writes only when the current ETag equals Condition.ETag (S3 If-Match).
	ConditionIfMatch
)

// Condition guards a write. The zero value is unconditional.
type Condition struct {
	// Kind selects the guard.
	Kind ConditionKind

	// ETag is the expected current ETag for [ConditionIfMatch]. It is an opaque token.
	ETag string
}

// IfAbsent returns a condition that succeeds only when the key does not exist.
func IfAbsent() Condition {
	defer perf.Track(nil, "blob.IfAbsent")()

	return Condition{Kind: ConditionIfAbsent}
}

// IfMatch returns a condition that succeeds only when the current ETag equals etag.
func IfMatch(etag string) Condition {
	defer perf.Track(nil, "blob.IfMatch")()

	return Condition{Kind: ConditionIfMatch, ETag: etag}
}

// IsConditional reports whether the condition constrains the write.
func (c Condition) IsConditional() bool {
	defer perf.Track(nil, "blob.Condition.IsConditional")()

	return c.Kind != ConditionNone
}

// PutOptions describe one write.
type PutOptions struct {
	// Size is the body length in bytes, or [UnknownSize]. A known size is verified: a body that
	// is shorter or longer fails the call and leaves no object behind.
	Size int64

	// ContentType is stored with the object when non-empty.
	ContentType string

	// Metadata is stored as user metadata.
	Metadata map[string]string

	// Tags are explicit object tags. Stores that merge default tags apply them first; these win.
	Tags map[string]string

	// Condition guards the write. The zero value is unconditional.
	Condition Condition
}

// GetOptions describe one read.
type GetOptions struct {
	// Offset is the first byte to read.
	Offset int64

	// Length is the number of bytes to read from Offset. Zero or negative reads to the end.
	Length int64
}

// IsRange reports whether the read is a partial read.
func (o GetOptions) IsRange() bool {
	defer perf.Track(nil, "blob.GetOptions.IsRange")()

	return o.Offset > 0 || o.Length > 0
}

// StatOptions describe one head request.
type StatOptions struct {
	// IncludeTags also loads the object tags, which may cost an extra request.
	IncludeTags bool
}

// DeleteOptions describe one delete.
type DeleteOptions struct {
	// IfMatch deletes only when the current ETag equals this opaque token.
	IfMatch string
}

// ListOptions describe one listing page.
type ListOptions struct {
	// Prefix restricts the listing to keys beginning with it.
	Prefix string

	// Delimiter groups keys sharing a prefix up to the delimiter into CommonPrefixes.
	Delimiter string

	// Cursor continues a previous listing.
	Cursor string

	// Limit caps the entries (objects plus common prefixes) in one page; zero uses the store default.
	Limit int
}

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	// Key is the object key.
	Key string

	// Size is the full object size in bytes, even for a ranged read.
	Size int64

	// ETag is an opaque concurrency token, never a payload checksum.
	ETag string

	// ContentType is the stored content type, when known.
	ContentType string

	// LastModified is the last modification time.
	LastModified time.Time

	// Metadata is user metadata, when loaded.
	Metadata map[string]string

	// Tags are object tags, loaded only when requested.
	Tags map[string]string
}

// Object is an open object. Callers must close Body.
type Object struct {
	ObjectInfo

	// Body streams the (possibly ranged) content.
	Body io.ReadCloser
}

// ListPage is one page of a listing.
type ListPage struct {
	// Objects are the keys found, lexicographically ordered.
	Objects []ObjectInfo

	// CommonPrefixes are rolled-up prefixes when a delimiter was requested.
	CommonPrefixes []string

	// NextCursor continues the listing; empty means complete.
	NextCursor string
}
