package s3test

import (
	"encoding/base64"
	"encoding/xml"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type listResult struct {
	XMLName               xml.Name         `xml:"ListBucketResult"`
	Name                  string           `xml:"Name"`
	Prefix                string           `xml:"Prefix"`
	Delimiter             string           `xml:"Delimiter,omitempty"`
	EncodingType          string           `xml:"EncodingType,omitempty"`
	KeyCount              int              `xml:"KeyCount"`
	MaxKeys               int              `xml:"MaxKeys"`
	IsTruncated           bool             `xml:"IsTruncated"`
	ContinuationToken     string           `xml:"ContinuationToken,omitempty"`
	NextContinuationToken string           `xml:"NextContinuationToken,omitempty"`
	Contents              []listContent    `xml:"Contents"`
	CommonPrefixes        []commonPrefixXM `xml:"CommonPrefixes"`
}

type listContent struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

type commonPrefixXM struct {
	Prefix string `xml:"Prefix"`
}

// listEntry is one item of a sorted listing: an object or a rolled-up prefix.
type listEntry struct {
	token  string
	object *object
}

func (s *Server) listObjects(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	prefix, delimiter := query.Get("prefix"), query.Get("delimiter")
	limit := min(s.cfg.pageCap, maxKeysCap)
	if raw := query.Get("max-keys"); raw != "" {
		requested, err := strconv.Atoi(raw)
		if err != nil || requested < 0 {
			writeError(w, http.StatusBadRequest, "InvalidArgument", "max-keys must be a non-negative integer")
			return
		}
		limit = min(limit, requested)
	}
	cursor, err := decodeToken(query.Get("continuation-token"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "InvalidArgument", "The continuation token provided is incorrect")
		return
	}
	if start := query.Get("start-after"); start > cursor {
		cursor = start
	}
	page := s.page(prefix, delimiter, cursor, limit)

	encode := func(v string) string { return v }
	if query.Get("encoding-type") == "url" {
		encode = func(v string) string { return strings.ReplaceAll(url.QueryEscape(v), "+", "%20") }
	}
	result := listResult{
		Name: s.cfg.bucket, Prefix: encode(prefix), Delimiter: delimiter, EncodingType: query.Get("encoding-type"),
		MaxKeys: limit, ContinuationToken: query.Get("continuation-token"),
	}
	for _, entry := range page.entries {
		if entry.object == nil {
			result.CommonPrefixes = append(result.CommonPrefixes, commonPrefixXM{Prefix: encode(entry.token)})
			continue
		}
		result.Contents = append(result.Contents, listContent{
			Key: encode(entry.object.key), LastModified: entry.object.modified.UTC().Format("2006-01-02T15:04:05.000Z"),
			ETag: entry.object.etag, Size: entry.object.size, StorageClass: "STANDARD",
		})
	}
	result.KeyCount = len(page.entries)
	if page.next != "" {
		result.IsTruncated = true
		result.NextContinuationToken = base64.RawURLEncoding.EncodeToString([]byte(page.next))
	}
	writeXML(w, result)
}

func decodeToken(token string) (string, error) {
	if token == "" {
		return "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	return string(raw), err
}

type listPage struct {
	entries []listEntry
	next    string
}

// snapshotUnder returns the sorted keys under prefix and a copy of their objects.
func (s *Server) snapshotUnder(prefix string) ([]string, map[string]object) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.objects))
	for key := range s.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	snapshot := make(map[string]object, len(keys))
	for _, key := range keys {
		snapshot[key] = *s.objects[key]
	}
	return keys, snapshot
}

// rollUp returns the ordering token of a key: the key itself, or its common prefix when the
// delimiter occurs after the listing prefix.
func rollUp(key, prefix, delimiter string) (string, bool) {
	if delimiter == "" {
		return key, false
	}
	rest := key[len(prefix):]
	idx := strings.Index(rest, delimiter)
	if idx < 0 {
		return key, false
	}
	return prefix + rest[:idx+len(delimiter)], true
}

// page builds one page: keys under prefix, delimiter roll-up, resumed after the cursor token.
func (s *Server) page(prefix, delimiter, cursor string, limit int) listPage {
	keys, snapshot := s.snapshotUnder(prefix)

	var page listPage
	last := ""
	for _, key := range keys {
		token, rolled := rollUp(key, prefix, delimiter)
		if (cursor != "" && token <= cursor) || (rolled && token == last) {
			continue
		}
		if len(page.entries) == limit {
			page.next = last
			return page
		}
		entry := listEntry{token: token}
		if !rolled {
			obj := snapshot[key]
			entry.object = &obj
		}
		page.entries = append(page.entries, entry)
		last = token
	}
	return page
}
