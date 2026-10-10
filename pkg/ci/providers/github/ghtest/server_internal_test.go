package ghtest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A page of comments larger than net/http's write buffer reaches the client before the
// handler returns, and a JSON decoder finishes as soon as the array is complete. The
// request must already be listed at that point, or a client that immediately fetches the
// next page (and a test that then reads Requests) sees the pages missing or reordered.
func TestServer_RecordsRequestsOnArrival(t *testing.T) {
	const perPage = 100
	seeded := make([]Comment, 0, perPage+1)
	for i := 1; i <= perPage+1; i++ {
		seeded = append(seeded, Comment{ID: int64(i), Body: fmt.Sprintf("comment %d with enough text to exceed the write buffer", i)})
	}
	s := NewServer(t, WithSeedComments("o", "r", 7, seeded...))

	released := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(released) }) }
	// Registered after NewServer so it runs before the server's own cleanup, which
	// waits for handlers; a failed assertion must not hang the test on a held handler.
	t.Cleanup(release)
	s.afterRoute = func() { <-released }

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, s.URL()+"/repos/o/r/issues/7/comments?per_page=100", http.NoBody)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var page []json.RawMessage
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&page), "the first page decodes while the handler is still held open")
	require.Len(t, page, perPage)
	assert.Contains(t, resp.Header.Get("Link"), `rel="next"`)

	reqs := s.Requests()
	require.Len(t, reqs, 1, "the request is listed before its handler returns")
	assert.Equal(t, "/repos/o/r/issues/7/comments", reqs[0].Path)
	assert.Equal(t, "per_page=100", reqs[0].RawQuery)
	assert.Zero(t, reqs[0].Status, "status is unknown until the handler returns")

	release()
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		r := s.Requests()
		return len(r) == 1 && r[0].Status == http.StatusOK
	}, 2*time.Second, 10*time.Millisecond, "status is filled in once the handler returns")
}
