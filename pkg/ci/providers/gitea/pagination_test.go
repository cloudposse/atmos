package gitea

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

var errVisitStop = errors.New("visit stop")

type pagedItem struct {
	ID int64 `json:"id"`
}

func pageOf(first int64, n int) []pagedItem {
	items := make([]pagedItem, n)
	for i := range items {
		items[i].ID = first + int64(i)
	}
	return items
}

func TestForEachPage(t *testing.T) {
	tests := []struct {
		name      string
		page      func(page int) []pagedItem
		visitErr  error
		wantErr   error
		wantPages int
		wantIDs   []int64 // First and last visited IDs.
		wantCount int
	}{
		{
			name:      "short page followed by empty page ends listing",
			page:      func(page int) []pagedItem { return pageOf(int64(page*pageSize), map[int]int{1: pageSize, 2: 3}[page]) },
			wantPages: 3,
			wantIDs:   []int64{pageSize, 2*pageSize + 2},
			wantCount: pageSize + 3,
		},
		{
			name: "multiple server-capped pages",
			page: func(page int) []pagedItem {
				if page > 3 {
					return nil
				}
				return pageOf(int64(page*10), 2)
			},
			wantPages: 4,
			wantIDs:   []int64{10, 31},
			wantCount: 6,
		},
		{
			name:      "repeated page stops without revisiting",
			page:      func(int) []pagedItem { return pageOf(1, pageSize) },
			wantPages: 2,
			wantIDs:   []int64{1, pageSize},
			wantCount: pageSize,
		},
		{
			name:      "page cap fails with the caller's error",
			page:      func(page int) []pagedItem { return pageOf(int64(page*pageSize), pageSize) },
			wantErr:   errUtils.ErrCIStatusFetchFailed,
			wantPages: maxPages,
			wantIDs:   []int64{pageSize, maxPages*pageSize + pageSize - 1},
			wantCount: maxPages * pageSize,
		},
		{
			name:      "visit error stops listing",
			page:      func(int) []pagedItem { return pageOf(1, pageSize) },
			visitErr:  errVisitStop,
			wantErr:   errVisitStop,
			wantPages: 1,
			wantIDs:   []int64{1, pageSize},
			wantCount: pageSize,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pages := 0
			testAPI(t, func(w http.ResponseWriter, r *http.Request) {
				pages++
				page, err := strconv.Atoi(r.URL.Query().Get("page"))
				require.NoError(t, err)
				require.Equal(t, strconv.Itoa(pageSize), r.URL.Query().Get("limit"))
				require.Equal(t, "recentupdate", r.URL.Query().Get("sort"))
				require.NoError(t, json.NewEncoder(w).Encode(tt.page(page)))
			})
			var visited []int64
			items := listing[pagedItem]{path: "/items?sort=recentupdate", id: func(i pagedItem) int64 { return i.ID }, limitErr: errUtils.ErrCIStatusFetchFailed}
			err := forEachPage(context.Background(), NewProvider(), items, func(page []pagedItem) error {
				for _, i := range page {
					visited = append(visited, i.ID)
				}
				return tt.visitErr
			})
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantPages, pages)
			require.Len(t, visited, tt.wantCount)
			require.Equal(t, tt.wantIDs, []int64{visited[0], visited[len(visited)-1]}, fmt.Sprint(tt.name))
		})
	}
}
