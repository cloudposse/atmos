package proxy

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"golang.org/x/sync/singleflight"

	log "github.com/cloudposse/atmos/pkg/logger"
)

// serve runs the cache-or-fetch flow for a routed request.
func (s *Server) serve(w http.ResponseWriter, r *http.Request, route *Route) {
	if route.Kind == KindPassthrough {
		s.servePassthrough(w, r, route)
		return
	}
	s.serveCacheable(w, r, route)
}

// servePassthrough fetches upstream once and streams the response straight back,
// preserving status and headers (e.g. a module download's X-Terraform-Get). Never
// cached. No retries.
func (s *Server) servePassthrough(w http.ResponseWriter, r *http.Request, route *Route) {
	res, err := s.fetchUpstream(r.Context(), r, route)
	if err != nil {
		log.Debug("Registry cache passthrough fetch failed", "url", route.Upstream.URL, "error", err)
		http.Error(w, "upstream fetch failed", http.StatusBadGateway)
		return
	}
	defer res.body.Close()

	copyHeader(w.Header(), res.header)
	if route.HeaderRewrite != nil {
		route.HeaderRewrite(w.Header(), s.baseURL)
	}
	w.WriteHeader(res.statusCode)
	_, _ = io.Copy(w, res.body)
}

// serveCacheable serves a cache hit, or on a miss collapses the herd and serves the
// filled object. It uses three tiers of concurrency control:
//
//  1. A lock-free hit fast path. Commit writes the object and its sidecar atomically
//     (temp file + rename), so an unlocked reader always sees a whole old/new object.
//  2. In-process singleflight: exactly one goroutine per process fills a cold key;
//     the rest wait on the shared result (cancelable, no timeout, no busy-poll).
//  3. A cross-process file lock inside the fill (see fillIfStale), held only around
//     fetch + commit, never while streaming to the client.
//
// Serving always happens from disk after every lock is released, so a slow client
// can never wedge other requesters of the same key.
func (s *Server) serveCacheable(w http.ResponseWriter, r *http.Request, route *Route) {
	// Tier 1: lock-free hit.
	if s.tryServeHit(w, route) {
		return
	}

	// Tier 2: collapse the in-process herd. The fill runs on s.baseCtx (not
	// r.Context()) so one client disconnecting cannot abort the shared fetch — each
	// waiter independently stops waiting via the select below.
	ch := s.group.DoChan(route.Key, func() (any, error) {
		return s.fillIfStale(r, route)
	})

	var res singleflight.Result
	select {
	case <-r.Context().Done():
		return // This client went away; the fill continues for the others.
	case res = <-ch:
	}

	if res.Err != nil {
		log.Debug("Registry cache fill failed", "key", route.Key, "error", res.Err)
		http.Error(w, "cache fetch failed", http.StatusBadGateway)
		return
	}

	fr, _ := res.Val.(fillResult)
	if !fr.cached {
		// A non-cacheable upstream response (e.g. a 404) is replayed to every waiter.
		s.writeUpstreamResponse(w, fr)
		return
	}

	// Tier 3 committed the object; serve it from disk, outside every lock.
	if _, err := s.serveStored(w, route); err != nil {
		log.Debug("Registry cache read failed after fill", "key", route.Key, "error", err)
		http.Error(w, "cache read failed", http.StatusBadGateway)
	}
}

// fillResult is the shared outcome of a singleflight fill, fanned out to every
// waiter on the key. Either the object was committed (serve it from disk) or the
// upstream returned a non-cacheable response that each waiter replays.
type fillResult struct {
	cached bool        // true: the object was committed; serve it from disk.
	status int         // non-cacheable upstream status to replay when !cached.
	header http.Header // upstream headers to replay when !cached.
	body   []byte      // buffered upstream body to replay when !cached.
}

// fillIfStale acquires the cross-process lock for the key and, unless another
// process committed it while we waited, fetches and commits it. It never writes to a
// client: it returns a fillResult that serveCacheable renders for every waiter. Runs
// on s.baseCtx so the shared fetch outlives any single requester; r is used only for
// upstream credential/header/User-Agent propagation (identical across callers of the
// same key).
func (s *Server) fillIfStale(r *http.Request, route *Route) (fillResult, error) {
	var fr fillResult
	err := s.opts.Store.Lock(route.Key).WithLockContext(s.baseCtx, func() error {
		// Re-check under the lock: another process may have committed the object while
		// we waited on the file lock.
		if meta, ok, statErr := s.opts.Store.Stat(route.Key); statErr == nil && ok && s.servable(route.Kind, meta) {
			fr = fillResult{cached: true}
			return nil
		}
		var ferr error
		fr, ferr = s.fetchVerifyCommit(r, route)
		return ferr
	})
	return fr, err
}

// tryServeHit serves the object from cache when present and servable. Returns true
// when it wrote a response. For stale-but-revalidatable metadata it serves
// immediately (a background revalidation can be added later).
func (s *Server) tryServeHit(w http.ResponseWriter, route *Route) bool {
	meta, ok, err := s.opts.Store.Stat(route.Key)
	if err != nil || !ok {
		return false
	}
	if !s.servable(route.Kind, meta) {
		return false
	}

	n, err := s.serveStored(w, route)
	if err != nil {
		return false
	}
	s.stats.RecordHit(n)
	return true
}

// servable reports whether a cached object may be served: fresh, or (for metadata)
// stale but still within the stale-while-revalidate window.
func (s *Server) servable(kind ArtifactKind, meta Meta) bool {
	if s.isFresh(kind, meta) {
		return true
	}
	return kind == KindMetadata && s.withinSWR(meta)
}

// fetchVerifyCommit performs the single upstream fetch for a cold key, applies
// rewrite/verify, commits atomically, and records the miss. It does not write to any
// client. On a non-2xx upstream it buffers the response into a fillResult to replay
// without caching. Runs under the per-key file lock.
func (s *Server) fetchVerifyCommit(r *http.Request, route *Route) (fillResult, error) {
	// Composite metadata: the mirror produces the body from one or more propagated
	// upstream calls (e.g. building a provider <version>.json).
	if route.Produce != nil {
		return s.produceCommit(r, route)
	}

	res, err := s.fetchUpstream(s.baseCtx, r, route)
	if err != nil {
		return fillResult{}, err
	}
	defer res.body.Close()

	// Non-2xx upstream: buffer and replay verbatim, do not cache.
	if res.statusCode < http.StatusOK || res.statusCode >= http.StatusMultipleChoices {
		body, rerr := io.ReadAll(res.body)
		if rerr != nil {
			return fillResult{}, rerr
		}
		s.stats.RecordMiss()
		return fillResult{status: res.statusCode, header: res.header, body: body}, nil
	}

	data, err := s.bodyForCommit(route, res)
	if err != nil {
		return fillResult{}, err
	}

	contentType := resolveContentType(route.ContentType, res.contentType)
	meta, err := s.opts.Store.Commit(s.baseCtx, CommitRequest{
		Key:         route.Key,
		Data:        data,
		Kind:        route.Kind,
		ContentType: contentType,
		Verify:      route.Verify,
	})
	if err != nil {
		return fillResult{}, err
	}
	s.stats.RecordCached(meta.Size)
	return fillResult{cached: true}, nil
}

// bodyForCommit returns the reader to commit, applying a metadata rewrite (e.g.
// rewriting provider download URLs to proxy URLs) when configured.
func (s *Server) bodyForCommit(route *Route, res *upstreamResult) (io.Reader, error) {
	if route.Kind != KindMetadata || route.Rewrite == nil {
		return res.body, nil
	}
	body, err := io.ReadAll(res.body)
	if err != nil {
		return nil, err
	}
	rewritten, err := route.Rewrite(body, s.baseURL)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(rewritten), nil
}

// produceCommit runs a mirror's composite Produce and commits the result as
// metadata. It does not write to any client. Runs under the per-key file lock.
func (s *Server) produceCommit(r *http.Request, route *Route) (fillResult, error) {
	fetch := s.boundFetcher(r)
	body, contentType, err := route.Produce(s.baseCtx, fetch, s.baseURL)
	if err != nil {
		return fillResult{}, err
	}
	contentType = resolveContentType(route.ContentType, contentType)
	meta, err := s.opts.Store.Commit(s.baseCtx, CommitRequest{
		Key:         route.Key,
		Data:        bytes.NewReader(body),
		Kind:        route.Kind,
		ContentType: contentType,
		Verify:      route.Verify,
	})
	if err != nil {
		return fillResult{}, err
	}
	s.stats.RecordCached(meta.Size)
	return fillResult{cached: true}, nil
}

// writeUpstreamResponse replays a non-cacheable upstream response (status, headers,
// body) captured during the fill to a waiting client.
func (s *Server) writeUpstreamResponse(w http.ResponseWriter, fr fillResult) {
	copyHeader(w.Header(), fr.header)
	if fr.status == 0 {
		// No upstream status was captured (e.g. client canceled before the fill ran);
		// surface a gateway error rather than writing a bare 200.
		http.Error(w, "cache fetch failed", http.StatusBadGateway)
		return
	}
	w.WriteHeader(fr.status)
	_, _ = w.Write(fr.body)
}

// serveStored opens a committed object, writes it to the response, and returns the
// number of bytes actually written to the client (so the hit path can record real
// bytes served, not the on-disk object size).
func (s *Server) serveStored(w http.ResponseWriter, route *Route) (int64, error) {
	rc, m, err := s.opts.Store.Open(route.Key)
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	n, copyErr := s.writeObject(w, route, m, rc)
	if copyErr != nil {
		// Post-header failure (e.g. the client disconnected mid-stream): the status is
		// already sent, so we cannot surface a 502. Log it and report the bytes actually
		// delivered.
		log.Debug("Registry cache: client write interrupted", "key", route.Key, "bytes", n, "error", copyErr)
	}
	return n, nil
}

// boundFetcher returns a Fetcher that applies the same propagation as the main
// fetch path, bound to the inbound request.
func (s *Server) boundFetcher(inbound *http.Request) Fetcher {
	return func(ctx context.Context, up UpstreamRequest) (*http.Response, error) {
		req, err := buildUpstreamRequest(ctx, inbound, up)
		if err != nil {
			return nil, err
		}
		return s.opts.Client.Do(req)
	}
}

// fetchUpstream performs the upstream request exactly once (no retries — the
// caller owns retry/backoff).
func (s *Server) fetchUpstream(ctx context.Context, inbound *http.Request, route *Route) (*upstreamResult, error) {
	req, err := buildUpstreamRequest(ctx, inbound, route.Upstream)
	if err != nil {
		return nil, err
	}
	resp, err := s.opts.Client.Do(req) //nolint:bodyclose // body is returned in upstreamResult and closed by the caller (defer res.body.Close()).
	if err != nil {
		return nil, err
	}
	return &upstreamResult{
		body:        resp.Body,
		statusCode:  resp.StatusCode,
		header:      resp.Header,
		contentType: resp.Header.Get("Content-Type"),
	}, nil
}

// writeObject writes a cached object to the response with the resolved Content-Type.
// It returns the number of bytes copied to the client and any copy error (which occurs
// after the status line is sent, so it cannot change the response status).
func (s *Server) writeObject(w http.ResponseWriter, route *Route, meta Meta, body io.Reader) (int64, error) {
	if ct := resolveContentType(route.ContentType, meta.ContentType); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(http.StatusOK)
	return io.Copy(w, body)
}

// resolveContentType prefers the route's content type, falling back to upstream's.
func resolveContentType(routeCT, fallback string) string {
	if routeCT != "" {
		return routeCT
	}
	return fallback
}

// isFresh reports whether a cached object is within its freshness window.
func (s *Server) isFresh(kind ArtifactKind, meta Meta) bool {
	if kind == KindArtifact {
		return true // Artifacts are immutable; cached forever.
	}
	if s.opts.MetadataTTL <= 0 {
		return true // No TTL configured: metadata is fresh forever.
	}
	return time.Since(meta.FetchedAt) < s.opts.MetadataTTL
}

// withinSWR reports whether stale metadata is still within the
// stale-while-revalidate window.
func (s *Server) withinSWR(meta Meta) bool {
	if s.opts.StaleWhileRevalidate <= 0 {
		return false
	}
	return time.Since(meta.FetchedAt) < s.opts.MetadataTTL+s.opts.StaleWhileRevalidate
}

// copyHeader copies all header values from src into dst.
func copyHeader(dst, src http.Header) {
	for k, vals := range src {
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}
