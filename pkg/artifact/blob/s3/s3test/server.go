package s3test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Server is an in-process S3-compatible endpoint serving one bucket in path-style addressing.
type Server struct {
	t      testing.TB
	cfg    settings
	http   *httptest.Server
	rec    recorder
	faults faults

	mu      sync.Mutex
	objects map[string]*object
	uploads map[string]*upload
	nextID  int

	aborted        atomic.Int64
	completed      atomic.Int64
	partBytes      atomic.Int64
	partsInFlight  atomic.Int64
	maxPartsInFlow atomic.Int64
}

// NewServer starts a server and registers its shutdown with t.Cleanup.
func NewServer(t testing.TB, opts ...Option) *Server {
	defer perf.Track(nil, "s3test.NewServer")()

	t.Helper()
	cfg := settings{bucket: defaultBucket, pageCap: maxKeysCap}
	for _, opt := range opts {
		opt(&cfg)
	}
	server := &Server{t: t, cfg: cfg, objects: map[string]*object{}, uploads: map[string]*upload{}}
	server.http = httptest.NewServer(http.HandlerFunc(server.serve))
	t.Cleanup(server.http.Close)
	return server
}

// Endpoint returns the base URL to configure as the S3 endpoint.
func (s *Server) Endpoint() string {
	defer perf.Track(nil, "s3test.Server.Endpoint")()

	return s.http.URL
}

// Close stops the server early, for tests of connection failures. It is safe to call twice.
func (s *Server) Close() {
	defer perf.Track(nil, "s3test.Server.Close")()

	s.http.Close()
}

// Bucket returns the served bucket name.
func (s *Server) Bucket() string {
	defer perf.Track(nil, "s3test.Server.Bucket")()

	return s.cfg.bucket
}

// Client returns an SDK client wired to the server with path-style addressing, static credentials,
// and optional request checksums disabled (the settings the generic store uses).
func (s *Server) Client() *s3.Client {
	defer perf.Track(nil, "s3test.Server.Client")()

	return s.client(func(o *s3.Options) {
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})
}

// LegacyClient returns a client that keeps the SDK's default checksum behavior, as the legacy CI
// adapter does.
func (s *Server) LegacyClient() *s3.Client {
	defer perf.Track(nil, "s3test.Server.LegacyClient")()

	return s.client()
}

func (s *Server) client(extra ...func(*s3.Options)) *s3.Client {
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
	}
	optFns := append([]func(*s3.Options){func(o *s3.Options) {
		o.BaseEndpoint = aws.String(s.http.URL)
		o.UsePathStyle = true
	}}, extra...)
	return s3.NewFromConfig(cfg, optFns...)
}

// serve records, applies faults to, and dispatches one request.
func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	bucket, key := splitPath(r.URL.Path)
	query := r.URL.Query()
	req := Request{Op: classifyRequest(r.Method, query, key), Method: r.Method, Key: key, Query: query, Header: r.Header.Clone(), At: timeNow()}
	index := s.rec.begin(&req)

	fault := s.faults.match(&req)
	if fault != nil {
		s.rec.update(index, func(rr *Request) { rr.Faulted = true })
	}
	rw := &statusWriter{ResponseWriter: w}
	if fault != nil && s.applyEarlyFault(rw, r, fault) {
		s.finish(index, rw)
		return
	}
	if bucket != s.cfg.bucket {
		writeError(rw, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist")
		s.finish(index, rw)
		return
	}
	if fault != nil && fault.Action == DropAfterCommit {
		s.handle(&mockDiscardWriter{header: http.Header{}}, r, key, req.Op)
		s.drop(w)
		s.rec.update(index, func(rr *Request) { rr.Status = 0 })
		return
	}
	s.handle(rw, r, key, req.Op)
	s.finish(index, rw)
}

// applyEarlyFault applies faults that decide the request before it is handled. It reports whether
// the request is finished.
func (s *Server) applyEarlyFault(w *statusWriter, r *http.Request, fault *Fault) bool {
	switch fault.Action {
	case FailStatus:
		_, _ = io.Copy(io.Discard, r.Body)
		status, code := fault.status()
		writeError(w, status, code, "injected fault")
		return true
	case DropBeforeCommit:
		_, _ = io.Copy(io.Discard, r.Body)
		s.drop(w.ResponseWriter)
		w.dropped = true
		return true
	case Delay:
		sleep(r.Context(), fault.Delay)
	case DropAfterCommit:
	}
	return false
}

// finish records the response status and body size of a handled request.
func (s *Server) finish(index int, w *statusWriter) {
	s.rec.update(index, func(rr *Request) {
		if !w.dropped {
			rr.Status = w.status()
		}
	})
}

// drop closes the client connection without writing a response.
func (s *Server) drop(w http.ResponseWriter) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		s.t.Errorf("s3test: response writer cannot be hijacked")
		return
	}
	conn, _, err := hijacker.Hijack()
	if err != nil {
		s.t.Errorf("s3test: hijack failed: %v", err)
		return
	}
	_ = conn.Close()
}

// handle dispatches a request to its operation.
func (s *Server) handle(w http.ResponseWriter, r *http.Request, key, op string) {
	handlers := map[string]func(){
		OpPutObject:               func() { s.putObject(w, r, key) },
		OpGetObject:               func() { s.getObject(w, r, key, false) },
		OpHeadObject:              func() { s.getObject(w, r, key, true) },
		OpDeleteObject:            func() { s.deleteObject(w, r, key) },
		OpListObjectsV2:           func() { s.listObjects(w, r) },
		OpGetObjectTagging:        func() { s.getTagging(w, key) },
		OpCreateMultipartUpload:   func() { s.createUpload(w, r, key) },
		OpUploadPart:              func() { s.uploadPart(w, r, key) },
		OpCompleteMultipartUpload: func() { s.completeUpload(w, r, key) },
		OpAbortMultipartUpload:    func() { s.abortUpload(w, r) },
	}
	if handler, ok := handlers[op]; ok {
		handler()
		return
	}
	writeError(w, http.StatusNotImplemented, "NotImplemented", "operation is not supported by the fake")
}

// statusWriter remembers the status code written.
type statusWriter struct {
	http.ResponseWriter
	code    int
	dropped bool
}

func (w *statusWriter) WriteHeader(code int) {
	defer perf.Track(nil, "s3test.statusWriter.WriteHeader")()

	if w.code == 0 {
		w.code = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	defer perf.Track(nil, "s3test.statusWriter.Write")()

	if w.code == 0 {
		w.code = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

func (w *statusWriter) status() int {
	if w.code == 0 {
		return http.StatusOK
	}
	return w.code
}

// mockDiscardWriter swallows a response, used when the response is dropped after the commit.
type mockDiscardWriter struct{ header http.Header }

func (d *mockDiscardWriter) Header() http.Header         { return d.header }
func (d *mockDiscardWriter) Write(p []byte) (int, error) { return len(p), nil }
func (d *mockDiscardWriter) WriteHeader(int)             {}
