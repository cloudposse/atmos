package authoritytest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
)

// LoggedRequest is one request the fake GitHub server received. The Authorization header is not
// stored; HasBearer says whether a bearer token was sent.
type LoggedRequest struct {
	Method    string
	Path      string
	Query     string
	Header    http.Header
	Body      string
	HasBearer bool
}

// KeyRecord is a key stored on the fake server.
type KeyRecord struct {
	ID    int64
	Login string
	// Use is authority.UseSigning or authority.UseAuthentication.
	Use authority.Use
	// Key is the key in authorized_keys form, as stored.
	Key   string
	Title string
}

// Failure makes the fake server answer with an error instead of handling a request.
type Failure struct {
	// Method and PathPrefix select the requests. An empty Method matches any method.
	Method     string
	PathPrefix string
	// Status and Body are the answer. Header adds response headers.
	Status int
	Body   string
	Header http.Header
	// Times is how many matching requests fail. Zero means one, and a negative number means all.
	Times int
}

type fakeToken struct {
	login  string
	scopes []string
}

// GitHubServer is an in-memory fake of the GitHub key-management API. It models token scopes,
// paginates, rejects requests without a User-Agent, and can be told to fail.
type GitHubServer struct {
	*httptest.Server

	// PageSize caps how many keys one page returns. Zero means the client's per_page.
	PageSize int
	// MissingScopeStatus is the status for a token without the scope: 404 (the default) or 403.
	MissingScopeStatus int

	mu       sync.Mutex
	tokens   map[string]fakeToken
	keys     []KeyRecord
	nextID   int64
	requests []LoggedRequest
	failures []*Failure
}

// NewGitHubServer starts a fake server and stops it when the test ends.
func NewGitHubServer(t *testing.T) *GitHubServer {
	t.Helper()
	s := &GitHubServer{tokens: map[string]fakeToken{}, nextID: 1000, MissingScopeStatus: http.StatusNotFound}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

// AddToken registers a classic token that belongs to login and holds scopes.
func (s *GitHubServer) AddToken(token, login string, scopes ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = fakeToken{login: login, scopes: scopes}
}

// AddKey stores a key on an account and returns its ID.
func (s *GitHubServer) AddKey(login string, use authority.Use, key ssh.PublicKey, title string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addKeyLocked(login, use, authority.AuthorizedKey(key), title)
}

func (s *GitHubServer) addKeyLocked(login string, use authority.Use, authorized, title string) int64 {
	s.nextID++
	s.keys = append(s.keys, KeyRecord{ID: s.nextID, Login: login, Use: use, Key: authorized, Title: title})
	return s.nextID
}

// Keys returns the keys stored for login and use.
func (s *GitHubServer) Keys(login string, use authority.Use) []KeyRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []KeyRecord
	for _, k := range s.keys {
		if k.Login == login && k.Use == use {
			out = append(out, k)
		}
	}
	return out
}

// KeyCount counts the stored keys, on any account, that match key for use.
func (s *GitHubServer) KeyCount(key ssh.PublicKey, use authority.Use) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, k := range s.keys {
		if k.Use == use && sameKeyBody(k.Key, authority.AuthorizedKey(key)) {
			n++
		}
	}
	return n
}

// Fail queues a failure.
func (s *GitHubServer) Fail(f Failure) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f.Times == 0 {
		f.Times = 1
	}
	s.failures = append(s.failures, &f)
}

// Requests returns a copy of the request log.
func (s *GitHubServer) Requests() []LoggedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]LoggedRequest(nil), s.requests...)
}

// ResetRequests clears the request log.
func (s *GitHubServer) ResetRequests() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = nil
}

// LogText renders the request log as text for leak checks. It includes every header except
// Authorization.
func (s *GitHubServer) LogText() string {
	var b strings.Builder
	for _, r := range s.Requests() {
		fmt.Fprintf(&b, "%s %s?%s %v %s\n", r.Method, r.Path, r.Query, r.Header, r.Body)
	}
	return b.String()
}

var scopeFamilies = map[authority.Use]string{
	authority.UseSigning:        "ssh_signing_key",
	authority.UseAuthentication: "public_key",
}

var scopeLevels = map[string]int{"read": 1, "write": 2, "admin": 3}

// ScopesFor returns the scopes that grant at least read (write=false) or write (write=true) on use.
func ScopesFor(use authority.Use, write bool) []string {
	family := scopeFamilies[use]
	if write {
		return []string{"admin:" + family, "write:" + family}
	}
	return []string{"admin:" + family, "write:" + family, "read:" + family}
}

func hasScope(scopes []string, use authority.Use, level int) bool {
	family := scopeFamilies[use]
	for _, s := range scopes {
		prefix, rest, _ := strings.Cut(s, ":")
		if rest == family && scopeLevels[prefix] >= level {
			return true
		}
	}
	return false
}

func sameKeyBody(a, b string) bool {
	fa, fb := strings.Fields(a), strings.Fields(b)
	return len(fa) >= 2 && len(fb) >= 2 && fa[0] == fb[0] && fa[1] == fb[1]
}

func (s *GitHubServer) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	auth := r.Header.Get("Authorization")
	header := r.Header.Clone()
	header.Del("Authorization")
	s.mu.Lock()
	s.requests = append(s.requests, LoggedRequest{
		Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Header: header,
		Body: string(body), HasBearer: strings.HasPrefix(auth, "Bearer "),
	})
	failure := s.matchFailureLocked(r)
	s.mu.Unlock()

	if r.Header.Get("User-Agent") == "" {
		writeJSON(w, http.StatusForbidden, nil, map[string]string{"message": "Request forbidden by administrative rules. Please make sure your request has a User-Agent header."})
		return
	}
	if failure != nil {
		for k, v := range failure.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(failure.Status)
		_, _ = io.WriteString(w, failure.Body)
		return
	}
	s.serve(w, r, auth, body)
}

func (s *GitHubServer) matchFailureLocked(r *http.Request) *Failure {
	for _, f := range s.failures {
		if f.Times == 0 || (f.Method != "" && f.Method != r.Method) || !strings.HasPrefix(r.URL.Path, f.PathPrefix) {
			continue
		}
		if f.Times > 0 {
			f.Times--
		}
		out := *f
		return &out
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, header http.Header, v any) {
	for k, vals := range header {
		w.Header()[k] = vals
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func (s *GitHubServer) serve(w http.ResponseWriter, r *http.Request, auth string, body []byte) {
	token, ok := strings.CutPrefix(auth, "Bearer ")
	s.mu.Lock()
	tok, known := s.tokens[token]
	s.mu.Unlock()
	if !ok || !known {
		writeJSON(w, http.StatusUnauthorized, nil, map[string]string{"message": "Bad credentials"})
		return
	}
	scopeHeaders := func(use authority.Use, level int) http.Header {
		h := http.Header{}
		h.Set("X-OAuth-Scopes", strings.Join(tok.scopes, ", "))
		if use != "" {
			h.Set("X-Accepted-OAuth-Scopes", strings.Join(ScopesFor(use, level == 2), ", "))
		}
		return h
	}
	if r.URL.Path == "/user" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, scopeHeaders("", 0), map[string]string{"login": tok.login})
		return
	}
	use, id, ok := parseKeyPath(r.URL.Path)
	if !ok {
		writeJSON(w, http.StatusNotFound, nil, map[string]string{"message": "Not Found"})
		return
	}
	level := 1
	if r.Method != http.MethodGet {
		level = 2
	}
	h := scopeHeaders(use, level)
	if !hasScope(tok.scopes, use, level) {
		writeJSON(w, s.MissingScopeStatus, h, map[string]string{"message": "Not Found"})
		return
	}
	switch {
	case r.Method == http.MethodGet && id == 0:
		s.list(w, r, tok.login, use, h)
	case r.Method == http.MethodPost && id == 0:
		s.create(w, tok.login, use, body, h)
	case r.Method == http.MethodDelete && id != 0:
		s.remove(w, tok.login, use, id, h)
	default:
		writeJSON(w, http.StatusNotFound, h, map[string]string{"message": "Not Found"})
	}
}

func parseKeyPath(p string) (authority.Use, int64, bool) {
	var use authority.Use
	var rest string
	switch {
	case strings.HasPrefix(p, "/user/ssh_signing_keys"):
		use, rest = authority.UseSigning, strings.TrimPrefix(p, "/user/ssh_signing_keys")
	case strings.HasPrefix(p, "/user/keys"):
		use, rest = authority.UseAuthentication, strings.TrimPrefix(p, "/user/keys")
	default:
		return "", 0, false
	}
	if rest == "" {
		return use, 0, true
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(rest, "/"), 10, 64)
	return use, id, err == nil && strings.HasPrefix(rest, "/")
}

func (s *GitHubServer) list(w http.ResponseWriter, r *http.Request, login string, use authority.Use, h http.Header) {
	size := 30
	if n, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && n > 0 {
		size = n
	}
	if s.PageSize > 0 && s.PageSize < size {
		size = s.PageSize
	}
	page := 1
	if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && n > 0 {
		page = n
	}
	mine := s.Keys(login, use)
	sort.Slice(mine, func(i, j int) bool { return mine[i].ID < mine[j].ID })
	start, end := min((page-1)*size, len(mine)), min(page*size, len(mine))
	out := make([]map[string]any, 0, end-start)
	for _, k := range mine[start:end] {
		out = append(out, map[string]any{"id": k.ID, "key": k.Key, "title": k.Title})
	}
	if end < len(mine) {
		next := fmt.Sprintf("%s%s?per_page=%d&page=%d", s.URL, r.URL.Path, size, page+1)
		h.Set("Link", fmt.Sprintf(`<%s>; rel="next"`, next))
	}
	writeJSON(w, http.StatusOK, h, out)
}

func (s *GitHubServer) create(w http.ResponseWriter, login string, use authority.Use, body []byte, h http.Header) {
	invalid := func(message string) {
		writeJSON(w, http.StatusUnprocessableEntity, h, map[string]any{
			"message": "Validation Failed",
			"errors":  []map[string]string{{"message": message}},
		})
	}
	var in struct {
		Key   string `json:"key"`
		Title string `json:"title"`
	}
	if json.Unmarshal(body, &in) != nil {
		invalid("key is invalid")
		return
	}
	if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(in.Key)); err != nil {
		invalid("key is invalid")
		return
	}
	s.mu.Lock()
	for _, k := range s.keys {
		if k.Use == use && sameKeyBody(k.Key, in.Key) {
			s.mu.Unlock()
			invalid("key is already in use")
			return
		}
	}
	id := s.addKeyLocked(login, use, in.Key, in.Title)
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, h, map[string]any{"id": id, "key": in.Key, "title": in.Title})
}

func (s *GitHubServer) remove(w http.ResponseWriter, login string, use authority.Use, id int64, h http.Header) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, k := range s.keys {
		if k.ID == id && k.Login == login && k.Use == use {
			s.keys = append(s.keys[:i], s.keys[i+1:]...)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, h, map[string]string{"message": "Not Found"})
}

// ReadScope and WriteScope name the least scope that grants read or write access to use's keys.
func ReadScope(use authority.Use) string { return "read:" + scopeFamilies[use] }

// WriteScope names the least scope that grants write access to use's keys.
func WriteScope(use authority.Use) string { return "write:" + scopeFamilies[use] }

// Tokens the Environment registers.
const (
	FullToken    = "ghp_FULLaccessTOKEN0123456789abcdef"
	RevokedToken = "ghp_REVOKEDtoken9876543210"
	Login        = "octocat"
)

// LimitedToken returns the token that holds every scope except those for use.
func LimitedToken(use authority.Use) string {
	return "ghp_LIMITED" + string(use) + "TOKEN0123456789"
}

// Environment wires the server to an authority built against it, for the conformance suite. It
// registers a full-scope token, one token per use that lacks that use's scopes, and hooks that
// look at the server's state.
func (s *GitHubServer) Environment(a authority.Authority) *Environment {
	uses := []authority.Use{authority.UseAuthentication, authority.UseSigning}
	var all []string
	for _, use := range uses {
		all = append(all, ScopesFor(use, true)...)
	}
	s.AddToken(FullToken, Login, all...)
	secrets := []string{FullToken, RevokedToken}
	for _, use := range uses {
		var scopes []string
		for _, other := range uses {
			if other != use {
				scopes = append(scopes, ScopesFor(other, true)...)
			}
		}
		s.AddToken(LimitedToken(use), Login, scopes...)
		secrets = append(secrets, LimitedToken(use))
	}
	return &Environment{
		Authority:   a,
		Credentials: map[string]string{authority.CredentialToken: FullToken},
		Invalid:     map[string]string{authority.CredentialToken: RevokedToken},
		Restricted: func(use authority.Use) map[string]string {
			return map[string]string{authority.CredentialToken: LimitedToken(use)}
		},
		Permission: ReadScope,
		Seed: func(t *testing.T, key ssh.PublicKey, use authority.Use) string {
			t.Helper()
			return strconv.FormatInt(s.AddKey(Login, use, key, "pre-existing"), 10)
		},
		Enrollments: s.KeyCount,
		Log:         s.LogText,
		Secrets:     secrets,
	}
}
