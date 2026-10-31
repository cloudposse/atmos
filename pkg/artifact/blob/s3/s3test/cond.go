package s3test

import (
	"net/http"
	"strings"
)

// conditionResult is the outcome of evaluating If-None-Match and If-Match.
type conditionResult int

const (
	conditionOK conditionResult = iota
	conditionFailed
	conditionMissing
	conditionNotImplemented
)

// evaluate checks the conditional headers of r against the current object (nil when absent).
func (s *Server) evaluate(r *http.Request, current *object) conditionResult {
	ifNoneMatch := r.Header.Get("If-None-Match")
	ifMatch := r.Header.Get("If-Match")
	if ifNoneMatch == "" && ifMatch == "" {
		return conditionOK
	}
	switch s.cfg.conditional {
	case ConditionalIgnore:
		return conditionOK
	case ConditionalNotImplemented:
		return conditionNotImplemented
	case ConditionalEnforce:
	}
	return enforce(ifNoneMatch, ifMatch, current)
}

// enforce applies If-None-Match and If-Match to the current object (nil when absent).
func enforce(ifNoneMatch, ifMatch string, current *object) conditionResult {
	if ifNoneMatch != "" && current != nil && (ifNoneMatch == "*" || etagMatches(ifNoneMatch, current.etag)) {
		return conditionFailed
	}
	if ifMatch == "" {
		return conditionOK
	}
	switch {
	case current == nil:
		return conditionMissing
	case ifMatch != "*" && !etagMatches(ifMatch, current.etag):
		return conditionFailed
	default:
		return conditionOK
	}
}

// etagMatches compares ETags ignoring quotes and the weak prefix.
func etagMatches(header, etag string) bool {
	return normalizeETag(header) == normalizeETag(etag)
}

func normalizeETag(etag string) string {
	return strings.Trim(strings.TrimPrefix(strings.TrimSpace(etag), "W/"), `"`)
}

// reject writes the response for a failed condition and reports whether it did.
func reject(w http.ResponseWriter, result conditionResult) bool {
	switch result {
	case conditionFailed:
		writeError(w, http.StatusPreconditionFailed, "PreconditionFailed", "At least one of the pre-conditions you specified did not hold")
	case conditionMissing:
		writeError(w, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.")
	case conditionNotImplemented:
		writeError(w, http.StatusNotImplemented, "NotImplemented", "A header you provided implies functionality that is not implemented")
	case conditionOK:
		return false
	}
	return true
}
