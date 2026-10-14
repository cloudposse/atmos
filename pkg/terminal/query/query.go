// Package query answers terminal capability queries on behalf of a terminal that cannot.
//
// TUI libraries such as bubbletea v1 and termenv probe the terminal at startup with
// OSC 10/OSC 11 (foreground/background color) and CSI 6n (cursor position) and wait
// several seconds for each reply. When Atmos owns the pseudo-terminal (for example a
// recorded session or a PTY without forwarded host input) nothing answers, so every
// child process would stall. Responder scans PTY output for those queries and writes
// the replies back to the PTY so children start immediately.
package query

import (
	"bytes"
	"io"
	"sort"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Replies sent for each supported query. A dark background and light foreground are reported.
const (
	foregroundReply     = "\x1b]10;rgb:ffff/ffff/ffff\x1b\\"
	backgroundReply     = "\x1b]11;rgb:0000/0000/0000\x1b\\"
	cursorPositionReply = "\x1b[1;1R"
)

// queries lists every recognized query sequence and the reply it triggers.
// OSC queries are accepted with either BEL or ST terminators.
var queries = []struct {
	sequence []byte
	reply    string
}{
	{[]byte("\x1b]10;?\x07"), foregroundReply},
	{[]byte("\x1b]10;?\x1b\\"), foregroundReply},
	{[]byte("\x1b]11;?\x07"), backgroundReply},
	{[]byte("\x1b]11;?\x1b\\"), backgroundReply},
	{[]byte("\x1b[6n"), cursorPositionReply},
}

// maxQueryLen is the length of the longest recognized query sequence.
var maxQueryLen = func() int {
	longest := 0
	for _, q := range queries {
		if len(q.sequence) > longest {
			longest = len(q.sequence)
		}
	}
	return longest
}()

// Responder detects terminal queries in a stream of output and writes replies to a sink.
// It is safe for concurrent use and tolerates queries split across Scan calls.
type Responder struct {
	mu   sync.Mutex
	sink io.Writer
	tail []byte
}

// NewResponder returns a Responder that writes replies to sink (typically the PTY master).
// A nil sink disables replies.
func NewResponder(sink io.Writer) *Responder {
	defer perf.Track(nil, "query.NewResponder")()

	return &Responder{sink: sink}
}

// Scan inspects a chunk of terminal output and answers every query that completes in it.
// A query is answered exactly once even when it spans multiple chunks.
func (r *Responder) Scan(chunk []byte) {
	defer perf.Track(nil, "query.Responder.Scan")()

	if r == nil || r.sink == nil || len(chunk) == 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	data := make([]byte, 0, len(r.tail)+len(chunk))
	data = append(data, r.tail...)
	data = append(data, chunk...)

	// Replies are issued in the order the queries appeared in the stream.
	type match struct {
		end   int
		reply string
	}
	var matches []match
	for _, q := range queries {
		for _, end := range matchEnds(data, q.sequence) {
			// Matches ending inside the retained tail were already answered by a previous Scan.
			if end > len(r.tail) {
				matches = append(matches, match{end: end, reply: q.reply})
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].end < matches[j].end })
	for _, m := range matches {
		_, _ = io.WriteString(r.sink, m.reply)
	}

	keep := maxQueryLen - 1
	if len(data) < keep {
		keep = len(data)
	}
	r.tail = append(r.tail[:0], data[len(data)-keep:]...)
}

// matchEnds returns the end offsets of every occurrence of sequence in data.
func matchEnds(data, sequence []byte) []int {
	var ends []int
	for offset := 0; offset < len(data); {
		i := bytes.Index(data[offset:], sequence)
		if i < 0 {
			break
		}
		ends = append(ends, offset+i+len(sequence))
		offset += i + 1
	}
	return ends
}

// Reader forwards everything read from the source unchanged while answering terminal queries.
type Reader struct {
	src       io.Reader
	responder *Responder
}

// NewReader wraps src so any terminal query read from it is answered on sink.
// The bytes returned by Read are identical to those produced by src.
func NewReader(src io.Reader, sink io.Writer) *Reader {
	defer perf.Track(nil, "query.NewReader")()

	return &Reader{src: src, responder: NewResponder(sink)}
}

// Read implements io.Reader.
func (r *Reader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		r.responder.Scan(p[:n])
	}
	return n, err
}
