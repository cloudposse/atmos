package s3test

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Number parsing parameters and the S3 tag limit.
const (
	maxTagCount = 10
	decimalBase = 10
	bitSize64   = 64
)

// objectAttrs are the attributes a write request carries.
type objectAttrs struct {
	contentType string
	metadata    map[string]string
	tags        map[string]string
}

// errTooManyTags reports a tag set above the S3 limit.
var errTooManyTags = errors.New("object tags cannot be greater than 10")

// attributes extracts content type, user metadata, and tags from request headers.
func attributes(r *http.Request) (objectAttrs, error) {
	attrs := objectAttrs{contentType: r.Header.Get("Content-Type")}
	for name, values := range r.Header {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "x-amz-meta-") && len(values) > 0 {
			if attrs.metadata == nil {
				attrs.metadata = map[string]string{}
			}
			attrs.metadata[strings.TrimPrefix(lower, "x-amz-meta-")] = values[0]
		}
	}
	raw := r.Header.Get("X-Amz-Tagging")
	if raw == "" {
		return attrs, nil
	}
	parsed, err := url.ParseQuery(raw)
	if err != nil {
		return objectAttrs{}, err
	}
	if len(parsed) > maxTagCount {
		return objectAttrs{}, errTooManyTags
	}
	attrs.tags = make(map[string]string, len(parsed))
	for key, values := range parsed {
		attrs.tags[key] = values[0]
	}
	return attrs, nil
}

func (s *Server) putObject(w http.ResponseWriter, r *http.Request, key string) {
	data, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "IncompleteBody", err.Error())
		return
	}
	attrs, err := attributes(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "InvalidTag", err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if reject(w, s.evaluate(r, s.objects[key])) {
		return
	}
	obj := &object{
		key: key, size: int64(len(data)), etag: quotedDigest(data), modified: timeNow(),
		contentType: attrs.contentType, metadata: attrs.metadata, tags: attrs.tags,
	}
	if !s.cfg.discard {
		obj.data = data
	}
	s.objects[key] = obj
	w.Header().Set("ETag", obj.etag)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) getObject(w http.ResponseWriter, r *http.Request, key string, head bool) {
	s.mu.Lock()
	obj, ok := s.objects[key]
	var snapshot object
	if ok {
		snapshot = *obj
	}
	s.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.")
		return
	}
	window, valid := parseRange(r.Header.Get("Range"), snapshot.size)
	if !valid {
		writeError(w, http.StatusRequestedRangeNotSatisfiable, "InvalidRange", "The requested range is not satisfiable")
		return
	}
	setObjectHeaders(w, &snapshot)
	w.Header().Set("Content-Length", strconv.FormatInt(window.end-window.start, decimalBase))
	if window.partial {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", window.start, window.end-1, snapshot.size))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if head {
		return
	}
	if s.cfg.discard {
		s.t.Errorf("s3test: cannot read object %q: bodies are discarded", key)
		return
	}
	_, _ = w.Write(snapshot.data[window.start:window.end])
}

// setObjectHeaders writes the response headers shared by GET and HEAD.
func setObjectHeaders(w http.ResponseWriter, obj *object) {
	header := w.Header()
	header.Set("ETag", obj.etag)
	header.Set("Last-Modified", obj.modified.UTC().Format(http.TimeFormat))
	if obj.contentType != "" {
		header.Set("Content-Type", obj.contentType)
	}
	for key, value := range obj.metadata {
		header.Set("X-Amz-Meta-"+key, value)
	}
	if len(obj.tags) > 0 {
		header.Set("X-Amz-Tagging-Count", strconv.Itoa(len(obj.tags)))
	}
}

// byteRange is a half-open window into an object.
type byteRange struct {
	start, end int64
	partial    bool
}

// parseRange resolves a Range header against size. It reports false for an unsatisfiable range.
func parseRange(header string, size int64) (byteRange, bool) {
	if header == "" {
		return byteRange{end: size}, true
	}
	spec, found := strings.CutPrefix(header, "bytes=")
	first, last, hasDash := strings.Cut(spec, "-")
	if !found || !hasDash {
		return byteRange{}, false
	}
	start, err := strconv.ParseInt(first, decimalBase, bitSize64)
	if err != nil || start > size || (start == size && size > 0) {
		return byteRange{}, false
	}
	end, ok := rangeEnd(last, start, size)
	return byteRange{start: start, end: end, partial: true}, ok
}

// rangeEnd resolves the exclusive end of a byte range from its last-byte text.
func rangeEnd(last string, start, size int64) (int64, bool) {
	if last == "" {
		return size, true
	}
	through, err := strconv.ParseInt(last, decimalBase, bitSize64)
	if err != nil || through < start {
		return 0, false
	}
	return min(through+1, size), true
}

func (s *Server) deleteObject(w http.ResponseWriter, r *http.Request, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Header.Get("If-Match") != "" || r.Header.Get("If-None-Match") != "" {
		if reject(w, s.evaluate(r, s.objects[key])) {
			return
		}
	}
	delete(s.objects, key)
	w.WriteHeader(http.StatusNoContent)
}

// tagging is the GetObjectTagging response.
type tagging struct {
	XMLName xml.Name `xml:"Tagging"`
	TagSet  struct {
		Tags []tag `xml:"Tag"`
	} `xml:"TagSet"`
}

type tag struct {
	Key   string `xml:"Key"`
	Value string `xml:"Value"`
}

func (s *Server) getTagging(w http.ResponseWriter, key string) {
	s.mu.Lock()
	obj, ok := s.objects[key]
	var tags map[string]string
	if ok {
		tags = cloneMap(obj.tags)
	}
	s.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.")
		return
	}
	var doc tagging
	for key, value := range tags {
		doc.TagSet.Tags = append(doc.TagSet.Tags, tag{Key: key, Value: value})
	}
	writeXML(w, doc)
}
