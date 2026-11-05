package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"

	"golang.org/x/sync/errgroup"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

// Content types of the objects the repository writes.
const (
	contentTypeJSON   = "application/json"
	contentTypeBinary = "application/octet-stream"
	contentTypeMD     = "text/markdown"
)

// putAttempts bounds how often an unconditional write is repeated after an ambiguous outcome.
// Unconditional writes of the same bytes are idempotent, so repeating them is safe.
const putAttempts = 3

// uri returns the display locator of a store key.
func (r *Repository) uri(key string) string {
	if r.prefix == "" {
		return "s3://" + r.bucket + "/" + key
	}
	return "s3://" + r.bucket + "/" + r.prefix + "/" + key
}

// putOptions builds the write options of one object. Every object the repository writes carries
// the configured tags; a fresh copy is passed so the store can never mutate the shared map.
func (r *Repository) putOptions(size int64, contentType string, cond blob.Condition) blob.PutOptions {
	return blob.PutOptions{Size: size, ContentType: contentType, Tags: maps.Clone(r.tags), Condition: cond}
}

// readObject reads one managed object, bounded by limit. A missing object returns (nil, "", nil).
func (r *Repository) readObject(ctx context.Context, key string, limit int64) (data []byte, etag string, err error) {
	object, err := r.store.Get(ctx, key, blob.GetOptions{})
	if blob.IsNotFound(err) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", r.mapErr(opRead, key, err)
	}
	defer object.Body.Close()
	data, err = io.ReadAll(io.LimitReader(object.Body, limit+1))
	if err != nil {
		return nil, "", r.mapErr(opRead, key, err)
	}
	if int64(len(data)) > limit {
		return nil, "", fmt.Errorf("%w: %s exceeds %d bytes", errUtils.ErrArtifactTooLarge, r.uri(key), limit)
	}
	return data, object.ETag, nil
}

// putConditional writes bytes under a condition and returns the new ETag. It does not retry: the
// caller decides how to reconcile a lost response because a repeated conditional write could
// conflict with itself.
func (r *Repository) putConditional(ctx context.Context, key string, data []byte, contentType string, cond blob.Condition) (string, error) {
	info, err := r.store.Put(ctx, key, bytes.NewReader(data), r.putOptions(int64(len(data)), contentType, cond))
	if err != nil {
		return "", r.mapErr(opWrite, key, err)
	}
	return info.ETag, nil
}

// putBytes writes a JSON document unconditionally, repeating the idempotent write after an ambiguous outcome.
func (r *Repository) putBytes(ctx context.Context, key string, data []byte) error {
	var err error
	for range putAttempts {
		_, err = r.store.Put(ctx, key, bytes.NewReader(data), r.putOptions(int64(len(data)), contentTypeJSON, blob.Condition{}))
		if !blob.IsAmbiguous(err) {
			break
		}
	}
	return r.mapErr(opWrite, key, err)
}

// putImmutable writes content-addressed bytes. An existing object must hold identical bytes:
// different bytes under the same name are corruption, never silently replaced.
func (r *Repository) putImmutable(ctx context.Context, key string, data []byte, contentType string, limit int64) error {
	_, err := r.putConditional(ctx, key, data, contentType, blob.IfAbsent())
	switch {
	case err == nil:
		return nil
	case !blob.IsPreconditionFailed(err) && !blob.IsAmbiguous(err):
		return err
	}
	existing, _, readErr := r.readObject(ctx, key, limit)
	switch {
	case readErr != nil:
		return readErr
	case existing == nil:
		return err
	case !bytes.Equal(existing, data):
		return fmt.Errorf("%w: %s already exists with different bytes", errUtils.ErrArtifactIntegrityFailed, r.uri(key))
	}
	return nil
}

// deleteObject removes one object; a missing object is not an error.
func (r *Repository) deleteObject(ctx context.Context, key string) error {
	return r.mapErr(opDelete, key, r.store.Delete(ctx, key, blob.DeleteOptions{}))
}

// deleteConditional removes one object only while its ETag is unchanged.
func (r *Repository) deleteConditional(ctx context.Context, key, etag string) error {
	return r.mapErr(opDelete, key, r.store.Delete(ctx, key, blob.DeleteOptions{IfMatch: etag}))
}

// deleteAll removes every key, continuing past failures and returning the first.
func (r *Repository) deleteAll(ctx context.Context, keys []string) error {
	var first error
	for _, key := range keys {
		if err := r.deleteObject(ctx, key); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// stat returns the object attributes, or nil when the object is missing.
func (r *Repository) stat(ctx context.Context, key string) (*blob.ObjectInfo, error) {
	info, err := r.store.Stat(ctx, key, blob.StatOptions{})
	if blob.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, r.mapErr(opRead, key, err)
	}
	return info, nil
}

// listPrefix returns every object below a prefix, following the pagination cursor.
func (r *Repository) listPrefix(ctx context.Context, prefix string) ([]blob.ObjectInfo, error) {
	objects, _, err := blob.ListAll(ctx, r.store, blob.ListOptions{Prefix: prefix})
	if err != nil {
		return nil, r.mapErr(opRead, prefix, err)
	}
	return objects, nil
}

// hasAny reports whether at least one object exists below a prefix.
func (r *Repository) hasAny(ctx context.Context, prefix string) (bool, error) {
	page, err := r.store.List(ctx, blob.ListOptions{Prefix: prefix, Limit: 1})
	if err != nil {
		return false, r.mapErr(opRead, prefix, err)
	}
	return len(page.Objects) > 0, nil
}

// forEach runs fn for every index below count with at most the configured number of calls in
// flight, stopping at the first failure.
func (r *Repository) forEach(ctx context.Context, count int, fn func(ctx context.Context, index int) error) error {
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(max(r.settings.concurrency, 1))
	for index := range count {
		group.Go(func() error { return fn(groupCtx, index) })
	}
	return group.Wait()
}
