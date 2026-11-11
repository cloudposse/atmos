package api

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

// logFixture implements the log service over the real wire protocol. A gate holds each call until released.
type logFixture struct {
	pluginv1.UnimplementedLogServer
	mu         sync.Mutex
	messages   []string
	invocation string
	calls      int
	gate       chan struct{}
	received   chan struct{}
	failWith   error
	maxBatch   int
}

func newLogFixture() *logFixture { return &logFixture{received: make(chan struct{}, 1024)} }

func (f *logFixture) Write(ctx context.Context, request *pluginv1.LogRequest) (*emptypb.Empty, error) {
	if f.maxBatch > 0 && len(request.GetEntries()) > f.maxBatch {
		return nil, status.Error(codes.InvalidArgument, "batch too large")
	}
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.failWith != nil {
		return nil, f.failWith
	}
	f.mu.Lock()
	f.calls++
	if values := metadata.ValueFromIncomingContext(ctx, InvocationMetadata); len(values) == 1 {
		f.invocation = values[0]
	}
	for _, entry := range request.GetEntries() {
		f.messages = append(f.messages, entry.GetMessage())
	}
	f.mu.Unlock()
	f.received <- struct{}{}
	return &emptypb.Empty{}, nil
}

func (f *logFixture) delivered() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.messages...)
}

func logClient(t *testing.T, fixture *logFixture) *Client {
	t.Helper()
	client, _ := serviceClient(t, func(s *grpc.Server) {
		if fixture != nil {
			pluginv1.RegisterLogServer(s, fixture)
		}
	})
	return client
}

func entry(message string) *pluginv1.LogEntry {
	return &pluginv1.LogEntry{Level: pluginv1.LogLevel_LOG_LEVEL_DEBUG, Message: message}
}

func messages(entries []*pluginv1.LogEntry) []string {
	result := make([]string, 0, len(entries))
	for _, e := range entries {
		result = append(result, e.GetMessage())
	}
	return result
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLogStreamDeliversBeforeCloseAndCarriesTheInvocation(t *testing.T) {
	t.Parallel()
	fixture := newLogFixture()
	client := logClient(t, fixture)
	stream := client.OpenLogs(t.Context(), "invocation-7")
	stream.Send(entry("early"))
	// The entry reaches Atmos while the stream is open, which is the point of live delivery: the call that
	// produced it has not returned.
	select {
	case <-fixture.received:
	case <-time.After(5 * time.Second):
		t.Fatal("entry was not delivered while the stream was open")
	}
	if got := fixture.delivered(); !equal(got, []string{"early"}) {
		t.Fatalf("delivered = %v", got)
	}
	if undelivered := stream.Close(); len(undelivered) != 0 {
		t.Fatalf("a delivered entry came back: %v", messages(undelivered))
	}
	if fixture.invocation != "invocation-7" {
		t.Fatalf("invocation metadata = %q", fixture.invocation)
	}
}

func TestLogStreamPreservesOrderAcrossBatches(t *testing.T) {
	t.Parallel()
	fixture := newLogFixture()
	stream := logClient(t, fixture).OpenLogs(t.Context(), "id", WithLogBatch(3))
	var want []string
	for i := range 50 {
		want = append(want, fmt.Sprintf("event-%02d", i))
		stream.Send(entry(want[i]))
	}
	if undelivered := stream.Close(); len(undelivered) != 0 {
		t.Fatalf("undelivered: %v", messages(undelivered))
	}
	// Close flushes: every entry is on screen before the call may return its result.
	if got := fixture.delivered(); !equal(got, want) {
		t.Fatalf("delivered order = %v", got)
	}
	if fixture.calls < 2 {
		t.Fatalf("expected the backlog to use more than one call, got %d", fixture.calls)
	}
}

func TestLogStreamCapsOversizedBatches(t *testing.T) {
	fixture := newLogFixture()
	fixture.maxBatch = MaxLogBatch
	fixture.gate = make(chan struct{})
	stream := logClient(t, fixture).OpenLogs(t.Context(), "id", WithLogQueue(2048), WithLogBatch(2048))
	for i := range 1500 {
		stream.Send(entry(fmt.Sprint(i)))
	}
	close(fixture.gate)
	if got := stream.Close(); len(got) != 0 {
		t.Fatalf("undelivered: %v", messages(got))
	}
	if got := fixture.delivered(); len(got) != 1500 {
		t.Fatalf("delivered %d entries, want 1500", len(got))
	}
}

func TestLogStreamReturnsEntriesWhenTheServiceIsUnavailable(t *testing.T) {
	t.Parallel()
	denied := &logFixture{failWith: status.Error(codes.PermissionDenied, "denied"), received: make(chan struct{}, 8)}
	for name, build := range map[string]func(*testing.T) *Client{
		"not served":        func(t *testing.T) *Client { return logClient(t, nil) },
		"permission denied": func(t *testing.T) *Client { return logClient(t, denied) },
		"unbound client":    func(*testing.T) *Client { return &Client{} },
		"nil client":        func(*testing.T) *Client { return nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			stream := build(t).OpenLogs(t.Context(), "id")
			stream.Send(entry("one"))
			stream.Send(entry("two"))
			// Wait for the failed attempt so the later entry exercises the unavailable path directly.
			time.Sleep(50 * time.Millisecond)
			stream.Send(entry("three"))
			got := messages(stream.Close())
			if !equal(got, []string{"one", "two", "three"}) {
				t.Fatalf("returned = %v", got)
			}
		})
	}
}

func TestLogStreamDoesNotRepeatEntriesCutOffInFlight(t *testing.T) {
	t.Parallel()
	for name, failure := range map[string]error{
		"internal":    status.Error(codes.Internal, "boom"),
		"unavailable": status.Error(codes.Unavailable, "connection lost"),
		"unknown":     errors.New("unknown"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newLogFixture()
			fixture.failWith = failure
			stream := logClient(t, fixture).OpenLogs(t.Context(), "id")
			stream.Send(entry("maybe shown"))
			got := messages(stream.Close())
			if !equal(got, []string{"1 log events were not shown"}) {
				t.Fatalf("an entry that may be on screen was returned or the omission was not reported: %v", got)
			}
		})
	}
}

func TestLogStreamWaitsForRoomAndLosesNothing(t *testing.T) {
	t.Parallel()
	fixture := newLogFixture()
	fixture.gate = make(chan struct{})
	stream := logClient(t, fixture).OpenLogs(t.Context(), "id", WithLogQueue(3), WithLogBatch(1))
	stream.Send(entry("inflight"))
	// Wait until the sender holds the first entry in a blocked call, so the queue is empty and then full.
	deadline := time.Now().Add(5 * time.Second)
	for {
		stream.mu.Lock()
		empty := len(stream.queue) == 0
		stream.mu.Unlock()
		if empty {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("sender never took the first entry")
		}
		time.Sleep(time.Millisecond)
	}
	for i := range 3 {
		stream.Send(entry(fmt.Sprintf("queued-%d", i)))
	}
	// The queue is full and the receiver is stalled: the next Send waits instead of dropping anything.
	finished := make(chan struct{})
	go func() {
		stream.Send(entry("queued-3"))
		close(finished)
	}()
	select {
	case <-finished:
		t.Fatal("Send returned while the queue was full and the receiver stalled")
	case <-time.After(100 * time.Millisecond):
	}
	close(fixture.gate)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("Send did not resume after the receiver drained the queue")
	}
	if got := stream.Close(); len(got) != 0 {
		t.Fatalf("nothing should be returned or omitted, got %v", messages(got))
	}
	if got := fixture.delivered(); !equal(got, []string{"inflight", "queued-0", "queued-1", "queued-2", "queued-3"}) {
		t.Fatalf("delivered = %v", got)
	}
}

func TestLogStreamCancellationReleasesWaitingSenders(t *testing.T) {
	t.Parallel()
	fixture := newLogFixture()
	fixture.gate = make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	stream := logClient(t, fixture).OpenLogs(ctx, "id", WithLogQueue(1), WithLogBatch(1))
	stream.Send(entry("inflight"))
	stream.Send(entry("queued"))
	finished := make(chan struct{})
	go func() {
		stream.Send(entry("waiting"))
		close(finished)
	}()
	select {
	case <-finished:
		t.Fatal("Send returned while the queue was full")
	case <-time.After(100 * time.Millisecond):
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation left a sender waiting")
	}
	got := messages(stream.Close())
	if len(got) != 1 || !strings.HasSuffix(got[0], "log events were not shown") {
		t.Fatalf("a cancelled call must report what it could not show: %v", got)
	}
}

func TestLogStreamFlushCapStillBoundsClose(t *testing.T) {
	t.Parallel()
	fixture := newLogFixture()
	fixture.gate = make(chan struct{})
	stream := logClient(t, fixture).OpenLogs(t.Context(), "id", WithLogFlush(100*time.Millisecond))
	stream.Send(entry("stuck"))
	begin := time.Now()
	got := messages(stream.Close())
	if elapsed := time.Since(begin); elapsed > 3*time.Second {
		t.Fatalf("Close waited %s past its flush cap", elapsed)
	}
	if !equal(got, []string{"1 log events were not shown"}) {
		t.Fatalf("returned = %v", got)
	}
}

func TestLogStreamStopsWithoutLeakingOnCancelAndIgnoresLateSends(t *testing.T) {
	fixture := newLogFixture()
	fixture.gate = make(chan struct{})
	client := logClient(t, fixture)
	// Establish the connection and the server goroutines before counting.
	warm := client.OpenLogs(t.Context(), "warm")
	warm.Send(entry("warm"))
	fixture.gate <- struct{}{}
	<-fixture.received
	warm.Close()
	baseline := stabilised()

	ctx, cancel := context.WithCancel(t.Context())
	stream := client.OpenLogs(ctx, "id")
	stream.Send(entry("held"))
	time.Sleep(20 * time.Millisecond)
	cancel()
	begin := time.Now()
	stream.Close()
	if elapsed := time.Since(begin); elapsed > 2*time.Second {
		t.Fatalf("Close after cancel took %s", elapsed)
	}
	// Late sends and repeated closes are harmless.
	stream.Send(entry("late"))
	stream.Close()
	waitForGoroutines(t, baseline)
}

func TestLogStreamCloseIsIdempotentAndNilSafe(t *testing.T) {
	t.Parallel()
	var missing *LogStream
	missing.Send(entry("ignored"))
	if missing.Close() != nil {
		t.Fatal("nil stream returned entries")
	}
	stream := (*Client)(nil).OpenLogs(t.Context(), "id")
	stream.Send(nil)
	stream.Send(entry("kept"))
	first, second := stream.Close(), stream.Close()
	if !equal(messages(first), []string{"kept"}) || !equal(messages(second), []string{"kept"}) {
		t.Fatalf("first = %v, second = %v", messages(first), messages(second))
	}
	first[0] = nil
	if second[0] == nil {
		t.Fatal("results alias each other")
	}
}

func TestLogStreamOptionsIgnoreInvalidValues(t *testing.T) {
	t.Parallel()
	stream := (*Client)(nil).OpenLogs(t.Context(), "id", WithLogQueue(0), WithLogBatch(-1), WithLogFlush(-time.Second))
	defer stream.Close()
	if stream.capacity != DefaultLogQueue || stream.batch != DefaultLogBatch || stream.flush != DefaultLogFlush {
		t.Fatalf("invalid options changed defaults: %d %d %s", stream.capacity, stream.batch, stream.flush)
	}
	limited := (*Client)(nil).OpenLogs(t.Context(), "id", WithLogQueue(1))
	limited.Send(entry("a"))
	limited.Send(entry("b"))
	if got := messages(limited.Close()); !equal(got, []string{"a", "1 log events were not shown"}) {
		t.Fatalf("unavailable stream is not bounded: %v", got)
	}
}

func stabilised() int {
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	return runtime.NumGoroutine()
}

func waitForGoroutines(t *testing.T, baseline int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > baseline {
		if time.Now().After(deadline) {
			buffer := make([]byte, 1<<16)
			t.Fatalf("goroutines leaked: %d > %d\n%s", runtime.NumGoroutine(), baseline, buffer[:runtime.Stack(buffer, true)])
		}
		time.Sleep(10 * time.Millisecond)
	}
}
