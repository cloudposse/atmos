package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"golang.org/x/sync/singleflight"

	errUtils "github.com/cloudposse/atmos/errors"
	httppkg "github.com/cloudposse/atmos/pkg/http"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	defaultReadHeaderTimeout = 30 * time.Second
	defaultShutdownTimeout   = 5 * time.Second
)

// Options configure a Server.
type Options struct {
	// Mirrors are consulted in order; the first whose Handles returns true owns the
	// request.
	Mirrors []Mirror
	// Store is the cache backend.
	Store Store
	// Client performs upstream fetches. Defaults to pkg/http.NewDefaultClient().
	Client Doer
	// MetadataTTL is how long KindMetadata stays fresh. Zero serves metadata as
	// fresh forever (no revalidation).
	MetadataTTL time.Duration
	// StaleWhileRevalidate is the window past MetadataTTL during which stale
	// metadata is served while a background revalidation runs.
	StaleWhileRevalidate time.Duration
	// ReadHeaderTimeout bounds slow-header attacks. Defaults to 30s.
	ReadHeaderTimeout time.Duration
	// TLSCertificate, when set, makes the proxy serve HTTPS with this certificate and
	// the base URL uses the https scheme. Terraform/OpenTofu require provider network
	// mirrors to be https, so the cache always sets this. Nil serves plain HTTP.
	TLSCertificate *tls.Certificate
}

// Server is the ephemeral caching proxy.
type Server struct {
	opts    Options
	stats   *Stats
	httpSrv *http.Server
	ln      net.Listener
	baseURL string
	// group collapses concurrent cold-key fills within this process so exactly one
	// goroutine fetches a given key upstream while the rest wait and serve from disk.
	group singleflight.Group
	// baseCtx bounds the lifetime of a shared fill. A fill runs in a detached
	// singleflight goroutine on behalf of many waiters, so it must not be tied to any
	// single request's context (one client disconnecting must not abort the shared
	// fetch). baseCtx is canceled on Shutdown so in-flight fills stop with the server.
	baseCtx    context.Context
	baseCancel context.CancelFunc
}

// NewServer constructs a Server. Call Start to bind and serve.
func NewServer(opts Options) *Server { //nolint:gocritic // Options is a constructor argument passed once by value.
	defer perf.Track(nil, "proxy.NewServer")()

	if opts.Client == nil {
		opts.Client = httppkg.NewDefaultClient()
	}
	if opts.ReadHeaderTimeout == 0 {
		opts.ReadHeaderTimeout = defaultReadHeaderTimeout
	}
	baseCtx, baseCancel := context.WithCancel(context.Background())
	return &Server{opts: opts, stats: &Stats{}, baseCtx: baseCtx, baseCancel: baseCancel}
}

// Start binds an ephemeral loopback listener (127.0.0.1:0), serves in a background
// goroutine, and returns the proxy's base URL. The scheme is https when a
// TLSCertificate is configured (the cache always configures one), else http.
func (s *Server) Start(_ context.Context) (string, error) {
	defer perf.Track(nil, "proxy.Server.Start")()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("%w: binding proxy listener: %w", errUtils.ErrHTTPRequestFailed, err)
	}
	s.ln = ln
	port := ln.Addr().(*net.TCPAddr).Port

	s.httpSrv = &http.Server{
		Handler:           s,
		ReadHeaderTimeout: s.opts.ReadHeaderTimeout,
	}

	scheme := "http"
	if s.opts.TLSCertificate != nil {
		scheme = "https"
		s.httpSrv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{*s.opts.TLSCertificate},
			MinVersion:   tls.VersionTLS12,
		}
	}
	s.baseURL = fmt.Sprintf("%s://127.0.0.1:%d/", scheme, port)

	go func() {
		serveErr := s.runListener(ln)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Debug("Registry cache proxy server error", "error", serveErr)
		}
	}()

	return s.baseURL, nil
}

// runListener runs the HTTP or HTTPS server depending on whether TLS is configured.
// ServeTLS with empty cert/key paths uses TLSConfig.Certificates.
func (s *Server) runListener(ln net.Listener) error {
	if s.opts.TLSCertificate != nil {
		return s.httpSrv.ServeTLS(ln, "", "")
	}
	return s.httpSrv.Serve(ln)
}

// BaseURL returns the proxy base URL (empty before Start).
func (s *Server) BaseURL() string {
	defer perf.Track(nil, "proxy.Server.BaseURL")()

	return s.baseURL
}

// Stats returns a snapshot of per-run cache statistics.
func (s *Server) Stats() StatsSnapshot {
	defer perf.Track(nil, "proxy.Server.Stats")()

	return s.stats.Snapshot()
}

// Shutdown gracefully stops the proxy. Safe to call once.
func (s *Server) Shutdown(ctx context.Context) error {
	defer perf.Track(nil, "proxy.Server.Shutdown")()

	if s.baseCancel != nil {
		s.baseCancel() // Stop any in-flight fills bound to the server lifetime.
	}
	if s.httpSrv == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, defaultShutdownTimeout)
	defer cancel()
	return s.httpSrv.Shutdown(shutdownCtx)
}

// ServeHTTP routes the request to the first mirror that handles it, then runs the
// cache-or-fetch flow.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer perf.Track(nil, "proxy.Server.ServeHTTP")()

	mirror := s.mirrorFor(r)
	if mirror == nil {
		http.Error(w, "no mirror handles this request", http.StatusNotFound)
		return
	}

	route, err := mirror.Route(r)
	if err != nil {
		log.Debug("Registry cache routing failed", "path", r.URL.Path, "error", err)
		http.Error(w, "routing failed", http.StatusBadGateway)
		return
	}

	s.serve(w, r, &route)
}

func (s *Server) mirrorFor(r *http.Request) Mirror {
	for _, m := range s.opts.Mirrors {
		if m.Handles(r) {
			return m
		}
	}
	return nil
}
