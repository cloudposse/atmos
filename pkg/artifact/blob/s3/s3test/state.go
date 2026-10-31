package s3test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// object is a stored object.
type object struct {
	key         string
	data        []byte
	size        int64
	etag        string
	modified    time.Time
	contentType string
	metadata    map[string]string
	tags        map[string]string
}

// Object is a read-only snapshot of a stored object.
type Object struct {
	Key         string
	Data        []byte
	Size        int64
	ETag        string
	ContentType string
	Metadata    map[string]string
	Tags        map[string]string
}

func (o *object) snapshot() Object {
	return Object{
		Key: o.key, Data: append([]byte(nil), o.data...), Size: o.size, ETag: o.etag,
		ContentType: o.contentType, Metadata: cloneMap(o.metadata), Tags: cloneMap(o.tags),
	}
}

func cloneMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// etagLength is the number of hex characters in a fake ETag, the length of an MD5 digest.
const etagLength = 32

// digestHex returns a short opaque digest. Real S3 ETags are MD5-based for unencrypted single
// requests, but they are concurrency tokens and callers must never rely on that, so the fake
// uses a different hash to catch code that does.
func digestHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:etagLength]
}

// quotedDigest returns the ETag of a single-request object.
func quotedDigest(data []byte) string {
	return `"` + digestHex(data) + `"`
}

// Object returns a snapshot of the object stored under key.
func (s *Server) Object(key string) (Object, bool) {
	defer perf.Track(nil, "s3test.Server.Object")()

	s.mu.Lock()
	defer s.mu.Unlock()
	obj, ok := s.objects[key]
	if !ok {
		return Object{}, false
	}
	return obj.snapshot(), true
}

// Keys returns every stored key in order.
func (s *Server) Keys() []string {
	defer perf.Track(nil, "s3test.Server.Keys")()

	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.objects))
	for key := range s.objects {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Seed stores an object directly, bypassing the HTTP surface and the request log.
func (s *Server) Seed(key string, data []byte, tags map[string]string) Object {
	defer perf.Track(nil, "s3test.Server.Seed")()

	s.mu.Lock()
	defer s.mu.Unlock()
	obj := &object{
		key: key, data: append([]byte(nil), data...), size: int64(len(data)), etag: quotedDigest(data),
		modified: timeNow(), tags: cloneMap(tags),
	}
	s.objects[key] = obj
	return obj.snapshot()
}

// OpenUploads returns the IDs of multipart uploads that were neither completed nor aborted.
func (s *Server) OpenUploads() []string {
	defer perf.Track(nil, "s3test.Server.OpenUploads")()

	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.uploads))
	for id := range s.uploads {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// AbortedUploads returns how many multipart uploads were aborted.
func (s *Server) AbortedUploads() int {
	defer perf.Track(nil, "s3test.Server.AbortedUploads")()

	return int(s.aborted.Load())
}

// CompletedUploads returns how many multipart uploads were completed.
func (s *Server) CompletedUploads() int {
	defer perf.Track(nil, "s3test.Server.CompletedUploads")()

	return int(s.completed.Load())
}

// PartBytesReceived returns the bytes of every UploadPart body received so far.
func (s *Server) PartBytesReceived() int64 {
	defer perf.Track(nil, "s3test.Server.PartBytesReceived")()

	return s.partBytes.Load()
}

// MaxConcurrentParts returns the most UploadPart requests that were in flight at once.
func (s *Server) MaxConcurrentParts() int {
	defer perf.Track(nil, "s3test.Server.MaxConcurrentParts")()

	return int(s.maxPartsInFlow.Load())
}

// newID returns a unique upload ID.
func (s *Server) newID() string {
	s.nextID++
	return fmt.Sprintf("upload-%d", s.nextID)
}
