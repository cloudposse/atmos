package gitea

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// The maxPages cap bounds every paginated listing (200 pages of 50 is 10,000 items).
const maxPages = 200

// listing describes a paginated Gitea endpoint: its path (query allowed), how to key
// an item for de-duplication, and the error to wrap when it exceeds maxPages.
type listing[T any] struct {
	path     string
	id       func(T) int64
	limitErr error
}

// forEachPage walks a paginated Gitea listing, handing each page's previously unseen
// items to visit. It stops on an empty page or on a page with no unseen items (a server
// that ignores the page parameter repeats one page forever), and wraps the listing's limitErr when
// the listing exceeds maxPages so a runaway listing fails instead of looping.
func forEachPage[T any](ctx context.Context, p *Provider, l listing[T], visit func([]T) error) error {
	separator := "?"
	if strings.Contains(l.path, "?") {
		separator = "&"
	}
	seen := map[int64]struct{}{}
	for page := 1; page <= maxPages; page++ {
		var items []T
		pagePath := l.path + separator + "limit=" + strconv.Itoa(pageSize) + "&page=" + strconv.Itoa(page)
		if err := p.request(ctx, http.MethodGet, pagePath, nil, &items); err != nil {
			return err
		}
		unseen := make([]T, 0, len(items))
		for _, item := range items {
			key := l.id(item)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			unseen = append(unseen, item)
		}
		if err := visit(unseen); err != nil {
			return err
		}
		if len(unseen) == 0 {
			return nil
		}
	}
	return fmt.Errorf("%w: more than %d pages", l.limitErr, maxPages)
}
