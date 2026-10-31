package s3test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// upload is an open multipart upload.
type upload struct {
	id          string
	key         string
	contentType string
	metadata    map[string]string
	tags        map[string]string
	parts       map[int]part
}

type part struct {
	etag   string
	digest string
	size   int64
	data   []byte
}

type initiateResult struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadID string   `xml:"UploadId"`
}

type completeRequest struct {
	XMLName xml.Name `xml:"CompleteMultipartUpload"`
	Parts   []struct {
		PartNumber int    `xml:"PartNumber"`
		ETag       string `xml:"ETag"`
	} `xml:"Part"`
}

type completeResult struct {
	XMLName xml.Name `xml:"CompleteMultipartUploadResult"`
	Bucket  string   `xml:"Bucket"`
	Key     string   `xml:"Key"`
	ETag    string   `xml:"ETag"`
}

func (s *Server) createUpload(w http.ResponseWriter, r *http.Request, key string) {
	attrs, err := attributes(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "InvalidTag", err.Error())
		return
	}
	s.mu.Lock()
	id := s.newID()
	s.uploads[id] = &upload{id: id, key: key, contentType: attrs.contentType, metadata: attrs.metadata, tags: attrs.tags, parts: map[int]part{}}
	s.mu.Unlock()
	writeXML(w, initiateResult{Bucket: s.cfg.bucket, Key: key, UploadID: id})
}

// maxPartNumber is the highest multipart part number S3 accepts.
const maxPartNumber = 10000

// trackConcurrency records the peak number of UploadPart requests in flight. The returned function
// ends the request's contribution.
func (s *Server) trackConcurrency() func() {
	inFlight := s.partsInFlight.Add(1)
	for {
		current := s.maxPartsInFlow.Load()
		if inFlight <= current || s.maxPartsInFlow.CompareAndSwap(current, inFlight) {
			break
		}
	}
	return func() { s.partsInFlight.Add(-1) }
}

func (s *Server) uploadPart(w http.ResponseWriter, r *http.Request, key string) {
	defer s.trackConcurrency()()

	data, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "IncompleteBody", err.Error())
		return
	}
	number, err := strconv.Atoi(r.URL.Query().Get("partNumber"))
	if err != nil || number < 1 || number > maxPartNumber {
		writeError(w, http.StatusBadRequest, "InvalidArgument", "Part number must be an integer between 1 and 10000")
		return
	}
	s.partBytes.Add(int64(len(data)))
	p := newPart(data, s.cfg.discard)

	s.mu.Lock()
	defer s.mu.Unlock()
	up, ok := s.uploads[r.URL.Query().Get("uploadId")]
	if !ok || up.key != key {
		writeError(w, http.StatusNotFound, "NoSuchUpload", "The specified upload does not exist.")
		return
	}
	up.parts[number] = p
	w.Header().Set("ETag", p.etag)
	w.WriteHeader(http.StatusOK)
}

// newPart builds a stored part, dropping the bytes when bodies are discarded.
func newPart(data []byte, discard bool) part {
	p := part{etag: quotedDigest(data), digest: digestHex(data), size: int64(len(data))}
	if !discard {
		p.data = data
	}
	return p
}

func (s *Server) completeUpload(w http.ResponseWriter, r *http.Request, key string) {
	body, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "IncompleteBody", err.Error())
		return
	}
	var req completeRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "MalformedXML", err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	up, ok := s.uploads[r.URL.Query().Get("uploadId")]
	if !ok || up.key != key {
		writeError(w, http.StatusNotFound, "NoSuchUpload", "The specified upload does not exist.")
		return
	}
	obj, problem := s.assemble(up, req)
	if problem != "" {
		writeError(w, http.StatusBadRequest, "InvalidPart", problem)
		return
	}
	if reject(w, s.evaluate(r, s.objects[key])) {
		return
	}
	s.objects[key] = obj
	delete(s.uploads, up.id)
	s.completed.Add(1)
	writeXML(w, completeResult{Bucket: s.cfg.bucket, Key: key, ETag: obj.etag})
}

// assemble validates the requested parts and builds the final object. A non-empty string is the
// reason the parts were rejected.
func (s *Server) assemble(up *upload, req completeRequest) (*object, string) {
	if len(req.Parts) == 0 {
		return nil, "You must specify at least one part"
	}
	var (
		data   []byte
		sums   strings.Builder
		size   int64
		lastNo = 0
	)
	for i, requested := range req.Parts {
		stored, ok := up.parts[requested.PartNumber]
		switch {
		case !ok || !etagMatches(requested.ETag, stored.etag):
			return nil, fmt.Sprintf("part %d is missing or its ETag does not match", requested.PartNumber)
		case requested.PartNumber <= lastNo:
			return nil, "parts must be in ascending order"
		case i < len(req.Parts)-1 && stored.size < s.cfg.minPartSize:
			return nil, "Your proposed upload is smaller than the minimum allowed size"
		}
		lastNo = requested.PartNumber
		sums.WriteString(stored.digest)
		size += stored.size
		data = append(data, stored.data...)
	}
	obj := &object{
		key: up.key, size: size, modified: timeNow(), contentType: up.contentType, metadata: up.metadata, tags: up.tags,
		etag: fmt.Sprintf(`"%s-%d"`, digestHex([]byte(sums.String())), len(req.Parts)),
	}
	if !s.cfg.discard {
		obj.data = data
	}
	return obj, ""
}

func (s *Server) abortUpload(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := r.URL.Query().Get("uploadId")
	if _, ok := s.uploads[id]; !ok {
		writeError(w, http.StatusNotFound, "NoSuchUpload", "The specified upload does not exist.")
		return
	}
	delete(s.uploads, id)
	s.aborted.Add(1)
	w.WriteHeader(http.StatusNoContent)
}
