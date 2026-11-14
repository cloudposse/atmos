package blob

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ListAll follows the pagination cursor until the listing is complete and returns every object and
// common prefix. Callers that can stop early should page manually.
func ListAll(ctx context.Context, store Store, opts ListOptions) ([]ObjectInfo, []string, error) {
	defer perf.Track(nil, "blob.ListAll")()

	var (
		objects  []ObjectInfo
		prefixes []string
	)
	for {
		page, err := store.List(ctx, opts)
		if err != nil {
			return nil, nil, err
		}
		objects = append(objects, page.Objects...)
		prefixes = append(prefixes, page.CommonPrefixes...)
		if page.NextCursor == "" {
			return objects, prefixes, nil
		}
		opts.Cursor = page.NextCursor
	}
}
