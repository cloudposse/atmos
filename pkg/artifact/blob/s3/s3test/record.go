package s3test

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// S3 operation names as reported in [Request.Op].
const (
	OpPutObject               = "PutObject"
	OpGetObject               = "GetObject"
	OpHeadObject              = "HeadObject"
	OpDeleteObject            = "DeleteObject"
	OpListObjectsV2           = "ListObjectsV2"
	OpGetObjectTagging        = "GetObjectTagging"
	OpCreateMultipartUpload   = "CreateMultipartUpload"
	OpUploadPart              = "UploadPart"
	OpCompleteMultipartUpload = "CompleteMultipartUpload"
	OpAbortMultipartUpload    = "AbortMultipartUpload"
	opUnknown                 = "Unknown"
)

// Request is one recorded request.
type Request struct {
	// Seq is the 1-based arrival order.
	Seq int

	// Op is the S3 operation name.
	Op string

	// Method is the HTTP method.
	Method string

	// Key is the object key, empty for bucket-level requests.
	Key string

	// Query holds the query parameters.
	Query url.Values

	// Header holds the request headers.
	Header http.Header

	// Bytes is the number of body bytes received.
	Bytes int64

	// Status is the status the server answered, or 0 when the connection was dropped.
	Status int

	// Faulted reports that an injected fault affected the request.
	Faulted bool

	// At is the arrival time.
	At time.Time
}

// recorder collects requests.
type recorder struct {
	mu       sync.Mutex
	requests []Request
}

// begin records an arriving request and returns its index.
func (r *recorder) begin(req *Request) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	req.Seq = len(r.requests) + 1
	r.requests = append(r.requests, *req)
	return len(r.requests) - 1
}

func (r *recorder) update(index int, fn func(*Request)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(&r.requests[index])
}

// Requests returns a copy of every recorded request in arrival order.
func (s *Server) Requests() []Request {
	defer perf.Track(nil, "s3test.Server.Requests")()

	s.rec.mu.Lock()
	defer s.rec.mu.Unlock()
	return append([]Request(nil), s.rec.requests...)
}

// RequestsFor returns the recorded requests of one operation.
func (s *Server) RequestsFor(op string) []Request {
	defer perf.Track(nil, "s3test.Server.RequestsFor")()

	var matched []Request
	for _, req := range s.Requests() {
		if req.Op == op {
			matched = append(matched, req)
		}
	}
	return matched
}

// ResetRequests clears the request log.
func (s *Server) ResetRequests() {
	defer perf.Track(nil, "s3test.Server.ResetRequests")()

	s.rec.mu.Lock()
	defer s.rec.mu.Unlock()
	s.rec.requests = nil
}

// operations maps an HTTP method and the query marker that distinguishes the operation to its
// S3 name. An empty marker is the method's default operation.
var operations = map[string]map[string]string{
	http.MethodHead:   {"": OpHeadObject},
	http.MethodGet:    {"tagging": OpGetObjectTagging, "": OpGetObject},
	http.MethodPut:    {"uploadId": OpUploadPart, "": OpPutObject},
	http.MethodPost:   {"uploads": OpCreateMultipartUpload, "uploadId": OpCompleteMultipartUpload},
	http.MethodDelete: {"uploadId": OpAbortMultipartUpload, "": OpDeleteObject},
}

// classifyRequest names the S3 operation for a request.
func classifyRequest(method string, query url.Values, key string) string {
	if key == "" && method == http.MethodGet {
		return OpListObjectsV2
	}
	byMarker := operations[method]
	for marker, op := range byMarker {
		if marker != "" && query.Has(marker) {
			return op
		}
	}
	if op, ok := byMarker[""]; ok {
		return op
	}
	return opUnknown
}

// splitPath separates the bucket and key from a path-style request path.
func splitPath(path string) (bucket, key string) {
	trimmed := strings.TrimPrefix(path, "/")
	bucket, key, _ = strings.Cut(trimmed, "/")
	return bucket, key
}
