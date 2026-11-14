package api

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

const (
	// DefaultLogQueue bounds the entries a LogStream holds while Atmos renders earlier ones.
	DefaultLogQueue = 1024
	// DefaultLogBatch bounds the entries sent in one call, so a backlog drains in a few calls.
	DefaultLogBatch = 64
	// MaxLogBatch matches the host bridge limit on entries per Write call.
	MaxLogBatch = 1024
	// DefaultLogFlush is how long Close waits for Atmos to render the entries still queued. Zero waits until
	// every entry is delivered or the call is cancelled, so no entry is lost to a slow terminal.
	DefaultLogFlush = time.Duration(0)
	// DefaultLogBytes bounds the encoded size of the entries Close returns, including the omission warning, so
	// that they stay well under the 16 MiB plugin message limit together with the rest of the response.
	DefaultLogBytes = 1 << 20
)

// omissionFormat is the message of the entry that reports how many entries were not shown.
const omissionFormat = "%d log events were not shown"

// logTagBytes is the largest encoded tag of a log entry field in a response; two bytes hold field numbers up
// to 2047.
const logTagBytes = 2

// omissionReserve is the encoded size of the largest possible omission warning, which the byte bound always
// leaves room for.
var omissionReserve = encodedLogSize(omission(math.MaxInt))

// LogOption configures a LogStream.
type LogOption func(*LogStream)

// WithLogQueue sets how many entries the stream holds before Send waits for Atmos to render some. Values below
// one are ignored.
//
//nolint:lintroller // Pure SDK helper; the standalone SDK cannot import host perf tracking.
func WithLogQueue(entries int) LogOption {
	return func(s *LogStream) {
		if entries > 0 {
			s.capacity = entries
		}
	}
}

// WithLogBatch sets how many entries one call to Atmos carries at most, capped at MaxLogBatch.
// Values below one are ignored.
//
//nolint:lintroller // Pure SDK helper; the standalone SDK cannot import host perf tracking.
func WithLogBatch(entries int) LogOption {
	return func(s *LogStream) {
		if entries > 0 {
			s.batch = min(entries, MaxLogBatch)
		}
	}
}

// WithLogBytes sets the encoded size, in bytes, that the entries Close returns may take in a response,
// including the omission warning. Entries that do not fit are counted as omitted. Values below one are ignored.
//
//nolint:lintroller // Pure SDK helper; the standalone SDK cannot import host perf tracking.
func WithLogBytes(size int) LogOption {
	return func(s *LogStream) {
		if size > 0 {
			s.bytes = size
		}
	}
}

// WithLogFlush caps how long Close waits for queued entries to reach Atmos; entries cut off by the cap are
// counted as omitted. The default is no cap. Values below zero are ignored.
//
//nolint:lintroller // Pure SDK helper; the standalone SDK cannot import host perf tracking.
func WithLogFlush(wait time.Duration) LogOption {
	return func(s *LogStream) {
		if wait >= 0 {
			s.flush = wait
		}
	}
}

// LogStream delivers a plugin's log entries to Atmos while a call is still running, so that Atmos shows them
// as they happen instead of when the call ends. Entries are sent in order by one goroutine, and none is dropped:
// when Atmos renders more slowly than the plugin logs, Send waits for room in the queue, so the plugin slows to
// the speed of the terminal. Send never fails the call, and it stops waiting when the call is cancelled.
//
// Entries that Atmos did not take, because the log service is unavailable, are retained up to the queue
// capacity and the response byte bound for Close to return in the response; further entries are counted as
// omitted. The default capacity is 1,024 entries and WithLogQueue changes it; the default byte bound is 1 MiB
// of encoded entries and WithLogBytes changes it. Entries delivered live are
// never returned, so nothing is shown twice. An entry whose delivery was cut off in flight, by cancellation or
// an optional flush cap or an ambiguous transport failure, is counted as omitted rather than returned: it
// may already be on screen.
type LogStream struct {
	sink     pluginv1.LogClient
	ctx      context.Context
	cancel   context.CancelFunc
	capacity int
	bytes    int
	batch    int
	flush    time.Duration
	wake     chan struct{}
	done     chan struct{}
	// space wakes senders waiting for room in the queue.
	space *sync.Cond

	mu          sync.Mutex
	queue       []*pluginv1.LogEntry
	undelivered []*pluginv1.LogEntry
	// The encoded size of undelivered as a repeated field of a response.
	retained    int
	omitted     int
	unavailable bool
	closing     bool
	result      []*pluginv1.LogEntry
	closed      bool
}

// OpenLogs starts delivering log entries for the invocation. It never fails: when the Atmos API is not
// configured or the invocation is unknown, Close returns up to the queue capacity (1,024 by default) and the
// byte bound (1 MiB by default), followed by an omission warning if more entries were sent. The caller must call Close before returning.
//
//nolint:lintroller // Pure SDK helper; the standalone SDK cannot import host perf tracking.
func (c *Client) OpenLogs(ctx context.Context, invocationID string, options ...LogOption) *LogStream {
	stream := &LogStream{capacity: DefaultLogQueue, bytes: DefaultLogBytes, batch: DefaultLogBatch, flush: DefaultLogFlush, wake: make(chan struct{}, 1), done: make(chan struct{})}
	for _, apply := range options {
		apply(stream)
	}
	stream.space = sync.NewCond(&stream.mu)
	stream.ctx, stream.cancel = context.WithCancel(WithInvocation(ctx, invocationID))
	// A cancelled call must not leave senders waiting for room.
	context.AfterFunc(stream.ctx, stream.wakeSenders)
	if c == nil || invocationID == "" {
		stream.unavailable = true
		close(stream.done)
		return stream
	}
	conn, err := c.conn()
	if err != nil {
		stream.unavailable = true
		close(stream.done)
		return stream
	}
	stream.sink = pluginv1.NewLogClient(conn)
	go stream.run()
	return stream
}

// Send queues an entry for delivery. It is safe for concurrent use. It waits while the queue is full, and does
// nothing after Close; when the call is cancelled while it waits, the entry is counted as omitted.
//
//nolint:lintroller // Pure SDK helper; the standalone SDK cannot import host perf tracking.
func (s *LogStream) Send(entry *pluginv1.LogEntry) {
	if s == nil || entry == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for !s.closing && !s.unavailable && s.ctx.Err() == nil && len(s.queue) >= s.capacity {
		s.space.Wait()
	}
	if s.closing {
		return
	}
	if s.unavailable {
		s.keepLocked(entry)
		return
	}
	if s.ctx.Err() != nil {
		s.omitted++
		return
	}
	s.queue = append(s.queue, entry)
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// wakeSenders releases senders waiting for room, after the stream closes or its call is cancelled.
func (s *LogStream) wakeSenders() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.space.Broadcast()
}

// keepLocked remembers an undelivered entry for the response, up to the queue bound and the byte bound, which
// keeps the response within the transport's message size. An entry that does not fit in the bytes left is
// counted as omitted, and room for the omission warning is always kept. The caller holds the lock.
func (s *LogStream) keepLocked(entry *pluginv1.LogEntry) {
	size := encodedLogSize(entry)
	if len(s.undelivered) >= s.capacity || s.retained+size > s.bytes-omissionReserve {
		s.omitted++
		return
	}
	s.undelivered = append(s.undelivered, entry)
	s.retained += size
}

// encodedLogSize is the size of an entry as one element of a repeated log field in a response.
func encodedLogSize(entry *pluginv1.LogEntry) int {
	return logTagBytes + protowire.SizeBytes(proto.Size(entry))
}

// omission is the entry that reports how many entries were not shown.
func omission(count int) *pluginv1.LogEntry {
	return &pluginv1.LogEntry{Level: pluginv1.LogLevel_LOG_LEVEL_WARN, Message: fmt.Sprintf(omissionFormat, count)}
}

// Close waits for queued entries to reach Atmos, for at most the flush cap when one is set, stops the stream, and returns
// the entries Atmos did not take, in order, followed by one entry that says how many were omitted when any
// were. It is safe to call more than once; later calls return the same entries.
//
//nolint:lintroller // Pure SDK helper; the standalone SDK cannot import host perf tracking.
func (s *LogStream) Close() []*pluginv1.LogEntry {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		result := append([]*pluginv1.LogEntry(nil), s.result...)
		s.mu.Unlock()
		return result
	}
	s.closing = true
	s.space.Broadcast()
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
	s.awaitDrained()
	s.cancel()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.omitted += len(s.queue)
	s.queue = nil
	s.result = s.undelivered
	if s.omitted > 0 {
		s.result = append(s.result, omission(s.omitted))
	}
	s.closed = true
	return append([]*pluginv1.LogEntry(nil), s.result...)
}

// awaitDrained waits until every queued entry is delivered or the call is cancelled, and gives up after the
// flush cap when one is set.
func (s *LogStream) awaitDrained() {
	if s.flush <= 0 {
		<-s.done
		return
	}
	timer := time.NewTimer(s.flush)
	defer timer.Stop()
	select {
	case <-s.done:
	case <-timer.C:
		// Abort the call in flight; the goroutine counts what it carried as omitted.
		s.cancel()
		<-s.done
	}
}

// run sends queued entries in order until the stream is closed and drained, or its context ends.
func (s *LogStream) run() {
	defer close(s.done)
	for {
		batch, more := s.take()
		if !more {
			return
		}
		s.deliver(batch)
	}
}

// take removes the next batch from the queue, waiting for one. It reports false when no more will come.
func (s *LogStream) take() ([]*pluginv1.LogEntry, bool) {
	for {
		s.mu.Lock()
		if len(s.queue) > 0 {
			size := min(s.batch, len(s.queue))
			batch := append([]*pluginv1.LogEntry(nil), s.queue[:size]...)
			s.queue = s.queue[size:]
			s.space.Broadcast()
			s.mu.Unlock()
			return batch, true
		}
		closing := s.closing
		s.mu.Unlock()
		if closing || s.ctx.Err() != nil {
			return nil, false
		}
		select {
		case <-s.wake:
		case <-s.ctx.Done():
			return nil, false
		}
	}
}

// deliver sends one batch. Atmos acknowledges a call after it rendered the entries, so success means shown.
func (s *LogStream) deliver(batch []*pluginv1.LogEntry) {
	_, err := s.sink.Write(s.ctx, &pluginv1.LogRequest{Entries: batch})
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !notDelivered(err) {
		// Cut off or failed in an unknown state: the entries may be on screen, so they are not repeated.
		s.omitted += len(batch)
		return
	}
	// Atmos took nothing. Stop calling it and hand everything back, in order, for the response.
	s.unavailable = true
	for _, entry := range batch {
		s.keepLocked(entry)
	}
	for _, entry := range s.queue {
		s.keepLocked(entry)
	}
	s.queue = nil
	s.space.Broadcast()
}

// notDelivered reports whether a failed call certainly delivered nothing, so its entries are safe to return in
// the response instead.
func notDelivered(err error) bool {
	switch status.Code(err) {
	case codes.PermissionDenied, codes.Unimplemented, codes.FailedPrecondition, codes.InvalidArgument, codes.ResourceExhausted:
		return true
	default:
		return false
	}
}
