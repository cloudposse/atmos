package local

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/perf"
)

// defaultPageSize is the entries per page when ListOptions.Limit is zero, matching S3's maximum.
const defaultPageSize = 1000

// entry is one object found during a walk.
type entry struct {
	key  string
	info os.FileInfo
}

// List implements [blob.Store].
func (s *Store) List(ctx context.Context, opts blob.ListOptions) (*blob.ListPage, error) {
	defer perf.Track(nil, "blob.local.List")()

	if err := checkContext(ctx, blob.OpList, opts.Prefix); err != nil {
		return nil, err
	}
	if err := validatePrefix(opts.Prefix); err != nil {
		return nil, err
	}
	var entries []entry
	var snapshotID string
	if id, last, ok := parseSnapshotCursor(opts.Cursor); ok {
		opts.Cursor = last
		snapshotID = id
		entries = s.lookupSnapshot(id, opts)
	}
	if entries == nil {
		var err error
		entries, err = s.walk(ctx, opts.Prefix)
		if err != nil {
			return nil, blob.NewError(blob.KindOther, blob.OpList, opts.Prefix, err)
		}
		snapshotID = ""
	}
	page := paginate(entries, opts)
	if page.NextCursor != "" {
		if snapshotID == "" {
			snapshotID = s.rememberSnapshot(entries, opts)
		}
		if snapshotID != "" {
			page.NextCursor = snapshotCursor(snapshotID, page.NextCursor)
		}
	}
	return page, nil
}

// walk returns every object whose key starts with prefix, sorted by key.
func (s *Store) walk(ctx context.Context, prefix string) ([]entry, error) {
	root, err := s.openRoot(false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	start := "."
	if idx := strings.LastIndexByte(prefix, '/'); idx >= 0 {
		start = prefix[:idx]
	}
	var entries []entry
	err = fs.WalkDir(root.FS(), start, func(name string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return ignoreMissing(walkErr)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		found, ok, err := s.objectAt(name, d, prefix)
		if err != nil || !ok {
			return err
		}
		entries = append(entries, found)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
	return entries, nil
}

// ignoreMissing treats a path that vanished during the walk as absent.
func ignoreMissing(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// objectAt returns the object stored at a walked path, or false for directories, bookkeeping
// files, keys outside the prefix, and files that vanished.
func (s *Store) objectAt(name string, d fs.DirEntry, prefix string) (entry, bool, error) {
	if d.IsDir() || reserved(name, d.Name()) {
		return entry{}, false, nil
	}
	key := name
	if !strings.HasPrefix(key, prefix) {
		return entry{}, false, nil
	}
	info, err := d.Info()
	if err != nil {
		return entry{}, false, ignoreMissing(err)
	}
	return entry{key: key, info: info}, info.Mode().IsRegular(), nil
}

// reserved reports whether a file is one of the store's own bookkeeping files.
func reserved(name, base string) bool {
	return strings.HasPrefix(base, tempPrefix) ||
		strings.HasSuffix(base, attrsSuffix) ||
		(base == lockName && path.Dir(name) == ".")
}

// paginate applies the delimiter roll-up, cursor, and limit to sorted entries.
func paginate(entries []entry, opts blob.ListOptions) *blob.ListPage {
	limit := opts.Limit
	if limit <= 0 || limit > defaultPageSize {
		limit = defaultPageSize
	}
	page := &blob.ListPage{}
	count := 0
	lastToken := ""
	for _, e := range entries {
		token, rolled := rollUp(e.key, opts)
		if opts.Cursor != "" && token <= opts.Cursor {
			continue
		}
		if rolled && token == lastToken {
			continue
		}
		if count == limit {
			page.NextCursor = lastToken
			return page
		}
		if rolled {
			page.CommonPrefixes = append(page.CommonPrefixes, token)
		} else {
			page.Objects = append(page.Objects, blob.ObjectInfo{Key: e.key, Size: e.info.Size(), LastModified: e.info.ModTime()})
		}
		lastToken = token
		count++
	}
	return page
}

// rollUp returns the ordering token of a key: the key itself, or its common prefix when the
// delimiter occurs after the listing prefix.
func rollUp(key string, opts blob.ListOptions) (string, bool) {
	if opts.Delimiter == "" {
		return key, false
	}
	rest := key[len(opts.Prefix):]
	idx := strings.Index(rest, opts.Delimiter)
	if idx < 0 {
		return key, false
	}
	return opts.Prefix + rest[:idx+len(opts.Delimiter)], true
}

// validatePrefix rejects listing prefixes that could leave the root.
func validatePrefix(prefix string) error {
	for _, segment := range strings.Split(prefix, "/") {
		if segment == ".." {
			return blob.NewError(blob.KindInvalid, blob.OpList, prefix, fmt.Errorf("%w: prefix contains a relative segment", errUtils.ErrBlobInvalid))
		}
	}
	if strings.ContainsRune(prefix, '\\') || strings.ContainsRune(prefix, 0) || strings.HasPrefix(prefix, "/") {
		return blob.NewError(blob.KindInvalid, blob.OpList, prefix, fmt.Errorf("%w: prefix must be a relative slash path", errUtils.ErrBlobInvalid))
	}
	return nil
}
