package ghtest

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	defaultPerPage = 30
	decimalBase    = 10
	int64Bits      = 64

	// Route wildcard names.
	pathOwner  = "owner"
	pathRepo   = "repo"
	pathNumber = "number"
	pathRef    = "ref"
	pathSHA    = "sha"
	pathID     = "id"

	keyRef      = "ref"
	msgBadJSON  = "Problems parsing JSON"
	msgNotFound = "Not Found"
)

// routes builds the route table. Anything not listed answers 404.
func (s *Server) routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /repos/{owner}/{repo}/issues/{number}/comments", s.listComments)
	mux.HandleFunc("POST /repos/{owner}/{repo}/issues/{number}/comments", s.createComment)
	mux.HandleFunc("PATCH /repos/{owner}/{repo}/issues/comments/{id}", s.editComment)

	mux.HandleFunc("GET /repos/{owner}/{repo}/commits/{sha}/comments", s.listCommitComments)
	mux.HandleFunc("POST /repos/{owner}/{repo}/commits/{sha}/comments", s.createCommitComment)
	mux.HandleFunc("PATCH /repos/{owner}/{repo}/comments/{id}", s.editCommitComment)

	mux.HandleFunc("POST /repos/{owner}/{repo}/statuses/{sha}", s.createStatus)
	mux.HandleFunc("GET /repos/{owner}/{repo}/commits/{ref}/status", s.combinedStatus)
	mux.HandleFunc("GET /repos/{owner}/{repo}/commits/{ref}/check-runs", s.listCheckRuns)

	mux.HandleFunc("POST /repos/{owner}/{repo}/code-scanning/sarifs", s.uploadSARIF)

	mux.HandleFunc("GET /repos/{owner}/{repo}/pulls", s.listPulls)
	mux.HandleFunc("GET /repos/{owner}/{repo}/pulls/{number}", s.getPull)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("ghtest: no route for %s %s", r.Method, r.URL.Path))
	})

	return mux
}

func commentURL(c *Comment) string {
	if c.SHA != "" {
		return fmt.Sprintf("https://github.com/%s/%s/commit/%s#commitcomment-%d", c.Owner, c.Repo, c.SHA, c.ID)
	}
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d#issuecomment-%d", c.Owner, c.Repo, c.Number, c.ID)
}

func commentJSON(c *Comment) map[string]any {
	return map[string]any{
		"id":       c.ID,
		"html_url": c.HTMLURL,
		"body":     c.Body,
		"user":     map[string]any{"login": "github-actions[bot]"},
	}
}

func (s *Server) listCommitComments(w http.ResponseWriter, r *http.Request) {
	key := commitKey{r.PathValue(pathOwner), r.PathValue(pathRepo), r.PathValue(pathSHA)}

	s.mu.Lock()
	all := append([]Comment(nil), s.commitComments[key]...)
	s.mu.Unlock()

	start, end, next := page(r, len(all))
	out := make([]map[string]any, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, commentJSON(&all[i]))
	}
	s.setNextLink(w, r, next)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createCommitComment(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, msgBadJSON)
		return
	}

	s.mu.Lock()
	c := s.addComment(&Comment{
		Owner: r.PathValue(pathOwner),
		Repo:  r.PathValue(pathRepo),
		SHA:   r.PathValue(pathSHA),
		Body:  payload.Body,
	})
	s.commentWrites = append(s.commentWrites, c)
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, commentJSON(&c))
}

// editCommitComment handles PATCH /repos/{owner}/{repo}/comments/{id}, which edits a commit comment.
func (s *Server) editCommitComment(w http.ResponseWriter, r *http.Request) {
	editStoredComment(s, w, r, s.commitComments, func(k commitKey) repoKey { return repoKey{k.owner, k.repo} })
}

// page returns the [start, end) window for the request's page/per_page and
// whether another page follows.
func page(r *http.Request, total int) (start, end int, next int) {
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage <= 0 {
		perPage = defaultPerPage
	}
	pageNum, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if pageNum <= 0 {
		pageNum = 1
	}

	start = min((pageNum-1)*perPage, total)
	end = min(start+perPage, total)
	if end < total {
		next = pageNum + 1
	}

	return start, end, next
}

// setNextLink adds an RFC 5988 Link header pointing at the next page.
func (s *Server) setNextLink(w http.ResponseWriter, r *http.Request, next int) {
	if next == 0 {
		return
	}
	q := url.Values{}
	for k, v := range r.URL.Query() {
		q[k] = v
	}
	q.Set("page", strconv.Itoa(next))
	w.Header().Set("Link", fmt.Sprintf(`<%s%s?%s>; rel="next"`, s.srv.URL, r.URL.Path, q.Encode()))
}

func (s *Server) listComments(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(r.PathValue(pathNumber))
	if err != nil {
		writeError(w, http.StatusNotFound, msgNotFound)
		return
	}

	s.mu.Lock()
	all := append([]Comment(nil), s.comments[issueKey{r.PathValue(pathOwner), r.PathValue(pathRepo), number}]...)
	s.mu.Unlock()

	start, end, next := page(r, len(all))
	out := make([]map[string]any, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, commentJSON(&all[i]))
	}
	s.setNextLink(w, r, next)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createComment(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(r.PathValue(pathNumber))
	if err != nil {
		writeError(w, http.StatusNotFound, msgNotFound)
		return
	}
	var payload struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, msgBadJSON)
		return
	}

	s.mu.Lock()
	c := s.addComment(&Comment{
		Owner:  r.PathValue(pathOwner),
		Repo:   r.PathValue(pathRepo),
		Number: number,
		Body:   payload.Body,
	})
	s.commentWrites = append(s.commentWrites, c)
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, commentJSON(&c))
}

func (s *Server) editComment(w http.ResponseWriter, r *http.Request) {
	editStoredComment(s, w, r, s.comments, func(k issueKey) repoKey { return repoKey{k.owner, k.repo} })
}

// editStoredComment applies a comment edit request to the comment in store whose ID is in the
// request path, among the comments of the requested repository. The repoOf argument maps a store
// key to its repository, which lets issue and commit comments share the same edit logic.
func editStoredComment[K comparable](s *Server, w http.ResponseWriter, r *http.Request, store map[K][]Comment, repoOf func(K) repoKey) {
	id, err := strconv.ParseInt(r.PathValue(pathID), decimalBase, int64Bits)
	if err != nil {
		writeError(w, http.StatusNotFound, msgNotFound)
		return
	}
	var payload struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, msgBadJSON)
		return
	}

	want := repoKey{r.PathValue(pathOwner), r.PathValue(pathRepo)}

	s.mu.Lock()
	defer s.mu.Unlock()

	for key, list := range store {
		if repoOf(key) != want {
			continue
		}
		for i := range list {
			if list[i].ID != id {
				continue
			}
			list[i].Body = payload.Body
			edited := list[i]
			edited.Edited = true
			s.commentWrites = append(s.commentWrites, edited)
			writeJSON(w, http.StatusOK, commentJSON(&list[i]))
			return
		}
	}

	writeError(w, http.StatusNotFound, msgNotFound)
}

// validStatusStates are the states the GitHub commit status API accepts.
var validStatusStates = map[string]bool{"error": true, "failure": true, "pending": true, "success": true}

func (s *Server) createStatus(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		State       string `json:"state"`
		Context     string `json:"context"`
		Description string `json:"description"`
		TargetURL   string `json:"target_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, msgBadJSON)
		return
	}
	if !validStatusStates[payload.State] {
		writeError(w, http.StatusUnprocessableEntity, "Validation Failed: state is invalid")
		return
	}

	st := Status{
		Owner:       r.PathValue(pathOwner),
		Repo:        r.PathValue(pathRepo),
		SHA:         r.PathValue(pathSHA),
		State:       payload.State,
		Context:     payload.Context,
		Description: payload.Description,
		TargetURL:   payload.TargetURL,
	}

	s.mu.Lock()
	s.statuses = append(s.statuses, st)
	id := len(s.statuses)
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, statusJSON(&st, id))
}

func statusJSON(st *Status, id int) map[string]any {
	return map[string]any{
		"id":          id,
		"state":       st.State,
		"context":     st.Context,
		"description": st.Description,
		"target_url":  st.TargetURL,
	}
}

// combinedStatus returns the latest status per context for a ref, with GitHub's
// rollup rule: any failure/error -> failure, else any pending -> pending, else success.
func (s *Server) combinedStatus(w http.ResponseWriter, r *http.Request) {
	owner, repo, ref := r.PathValue(pathOwner), r.PathValue(pathRepo), r.PathValue(pathRef)

	s.mu.Lock()
	latest := latestStatuses(s.statuses, owner, repo, ref)
	s.mu.Unlock()

	out := make([]map[string]any, 0, len(latest))
	for i := range latest {
		out = append(out, statusJSON(&latest[i], i+1))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"state":       rollupState(latest),
		"sha":         ref,
		"total_count": len(out),
		"statuses":    out,
	})
}

// latestStatuses returns the most recent status of each context for the ref, in first-seen order.
func latestStatuses(all []Status, owner, repo, ref string) []Status {
	index := map[string]int{}
	var out []Status
	for _, st := range all {
		if st.Owner != owner || st.Repo != repo || st.SHA != ref {
			continue
		}
		if i, ok := index[st.Context]; ok {
			out[i] = st
			continue
		}
		index[st.Context] = len(out)
		out = append(out, st)
	}

	return out
}

// rollupState computes the combined state of a set of statuses.
func rollupState(statuses []Status) string {
	if len(statuses) == 0 {
		return "pending"
	}
	state := "success"
	for i := range statuses {
		switch statuses[i].State {
		case "failure", "error":
			return "failure"
		case "pending":
			state = "pending"
		}
	}

	return state
}

func (s *Server) listCheckRuns(w http.ResponseWriter, r *http.Request) {
	key := refKey{r.PathValue(pathOwner), r.PathValue(pathRepo), r.PathValue(pathRef)}

	s.mu.Lock()
	runs := append([]CheckRun(nil), s.checkRuns[key]...)
	s.mu.Unlock()

	out := make([]map[string]any, 0, len(runs))
	for i, cr := range runs {
		out = append(out, map[string]any{
			"id":          i + 1,
			"name":        cr.Name,
			"status":      cr.Status,
			"conclusion":  cr.Conclusion,
			"details_url": cr.DetailsURL,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"total_count": len(out), "check_runs": out})
}

func (s *Server) uploadSARIF(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		CommitSHA string `json:"commit_sha"`
		Ref       string `json:"ref"`
		Sarif     string `json:"sarif"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, msgBadJSON)
		return
	}
	decoded, err := decodeSARIF(payload.Sarif)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ghtest: invalid sarif field: "+err.Error())
		return
	}

	owner, repo := r.PathValue(pathOwner), r.PathValue(pathRepo)

	s.mu.Lock()
	s.nextSARIF++
	id := fmt.Sprintf("sarif-%d", s.nextSARIF)
	s.sarif = append(s.sarif, SARIFUpload{
		Owner:     owner,
		Repo:      repo,
		CommitSHA: payload.CommitSHA,
		Ref:       payload.Ref,
		SARIF:     decoded,
		ID:        id,
	})
	s.mu.Unlock()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":  id,
		"url": fmt.Sprintf("%s/repos/%s/%s/code-scanning/sarifs/%s", s.srv.URL, owner, repo, id),
	})
}

// decodeSARIF reverses the upload encoding: base64 of gzip.
func decodeSARIF(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	defer gz.Close()
	out, err := io.ReadAll(gz)
	if err != nil {
		return "", err
	}

	return string(out), nil
}

func pullJSON(pr *PullRequest) map[string]any {
	return map[string]any{
		"number":   pr.Number,
		"title":    pr.Title,
		"html_url": pr.HTMLURL,
		"head":     map[string]any{keyRef: pr.HeadRef, "sha": pr.HeadSHA},
		"base":     map[string]any{keyRef: pr.BaseRef},
	}
}

// listPulls filters by the "head" query ("owner:branch"), the only filter the
// provider uses.
func (s *Server) listPulls(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	all := append([]PullRequest(nil), s.pulls[repoKey{r.PathValue(pathOwner), r.PathValue(pathRepo)}]...)
	s.mu.Unlock()

	head := r.URL.Query().Get("head")
	if _, branch, ok := strings.Cut(head, ":"); ok {
		head = branch
	}

	out := make([]map[string]any, 0, len(all))
	for i := range all {
		if head == "" || all[i].HeadRef == head {
			out = append(out, pullJSON(&all[i]))
		}
	}

	start, end, next := page(r, len(out))
	s.setNextLink(w, r, next)
	writeJSON(w, http.StatusOK, out[start:end])
}

func (s *Server) getPull(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(r.PathValue(pathNumber))
	if err != nil {
		writeError(w, http.StatusNotFound, msgNotFound)
		return
	}

	s.mu.Lock()
	all := append([]PullRequest(nil), s.pulls[repoKey{r.PathValue(pathOwner), r.PathValue(pathRepo)}]...)
	s.mu.Unlock()

	for i := range all {
		if all[i].Number == number {
			writeJSON(w, http.StatusOK, pullJSON(&all[i]))
			return
		}
	}
	writeError(w, http.StatusNotFound, msgNotFound)
}
