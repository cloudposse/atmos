package git

import (
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// HTTPResponse contains only rate-limit metadata from the final HTTP response.
// No URLs, credentials, cookies or arbitrary header values are retained.
type HTTPResponse struct {
	Status     int
	RetryAfter *time.Duration
	ResetAt    time.Time
	Remaining  *int
}

const (
	httpTraceLineLimit = 64 * 1024
	decimalBase        = 10
	integerBits        = 64
)

var gitTracePrefix = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}\.\d+\s+\S+:\d+\s+`)

// httpMetadataWriter consumes curl trace lines before they reach any diagnostic sink.
// Git tracing is confined to stderr, with payload tracing disabled. Oversized lines
// are discarded, never partially forwarded (a truncated prefix could expose headers).
type httpMetadataWriter struct {
	sink     io.Writer
	response HTTPResponse
	line     []byte
	dropping bool
}

func (w *httpMetadataWriter) Write(p []byte) (int, error) {
	for i, b := range p {
		switch {
		case b == '\n':
			if err := w.flush(); err != nil {
				return i + 1, err
			}
		case w.dropping:
			continue
		case len(w.line) >= httpTraceLineLimit:
			w.line = w.line[:0]
			w.dropping = true
		default:
			w.line = append(w.line, b)
		}
	}
	return len(p), nil
}

func (w *httpMetadataWriter) flush() error {
	defer func() { w.line = w.line[:0]; w.dropping = false }()
	if w.dropping {
		return nil
	}
	line := string(w.line)
	if prefix := gitTracePrefix.FindStringIndex(line); prefix != nil {
		if header, ok := strings.CutPrefix(line[prefix[1]:], "<= Recv header: "); ok {
			w.header(strings.TrimSpace(header))
		}
		return nil
	}
	// Also discard trace markers should a different Git version omit its prefix.
	if strings.Contains(line, "=> Send") || strings.Contains(line, "<= Recv") || strings.Contains(line, "== Info:") {
		return nil
	}
	_, err := w.sink.Write(append(w.line, '\n'))
	return err
}

func (w *httpMetadataWriter) header(line string) {
	if status, ok := responseStatus(line); ok {
		w.response = HTTPResponse{Status: status}
		return
	}
	name, value, ok := strings.Cut(line, ":")
	if !ok {
		return
	}
	value = strings.TrimSpace(value)
	switch strings.ToLower(name) {
	case "retry-after":
		w.response.RetryAfter = parseRetryAfter(value)
	case "x-ratelimit-reset":
		if seconds, err := strconv.ParseInt(value, decimalBase, integerBits); err == nil && seconds > 0 {
			w.response.ResetAt = time.Unix(seconds, 0)
		}
	case "x-ratelimit-remaining":
		if remaining, err := strconv.Atoi(value); err == nil && remaining >= 0 {
			w.response.Remaining = &remaining
		}
	}
}

func responseStatus(line string) (int, bool) {
	if !strings.HasPrefix(line, "HTTP/") {
		return 0, false
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, true
	}
	status, _ := strconv.Atoi(fields[1])
	return status, true
}

func parseRetryAfter(value string) *time.Duration {
	if seconds, err := strconv.ParseUint(value, decimalBase, integerBits); err == nil || errors.Is(err, strconv.ErrRange) {
		delay := time.Duration(math.MaxInt64)
		if seconds <= uint64(math.MaxInt64/int64(time.Second)) {
			delay = time.Duration(seconds) * time.Second
		}
		return &delay
	}
	if at, err := http.ParseTime(value); err == nil {
		delay := max(time.Duration(0), time.Until(at))
		return &delay
	}
	return nil
}

func httpMetadataEnv(env []string) []string {
	if env == nil {
		env = os.Environ()
	}
	overrides := map[string]string{"GIT_TRACE_CURL": "1", "GIT_TRACE_CURL_NO_DATA": "1", "GIT_TRACE_REDACT": "1", "GIT_TRACE_BRIEF": "0", "GIT_HTTP_MAX_RETRIES": "0"}
	out := make([]string, 0, len(env)+len(overrides))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if _, replace := overrides[key]; !replace && key != "GIT_CURL_VERBOSE" {
			out = append(out, entry)
		}
	}
	for key, value := range overrides {
		out = append(out, key+"="+value)
	}
	return out
}
