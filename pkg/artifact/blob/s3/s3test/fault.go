package s3test

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// FaultAction is what an injected fault does to a matching request.
type FaultAction int8

// Fault actions.
const (
	// FailStatus answers with an S3 error and does not apply the request.
	FailStatus FaultAction = iota

	// DropAfterCommit applies the request, then closes the connection without answering. The client
	// sees a transport error although the mutation happened: response ambiguity.
	DropAfterCommit

	// DropBeforeCommit closes the connection without applying the request.
	DropBeforeCommit

	// Delay sleeps before handling the request normally.
	Delay
)

// Fault describes a failure to inject. A request matches when Op and KeyPrefix match (empty
// matches anything). The first Skip matches pass through untouched ("fail after N requests"), then
// the fault applies Times times; Times zero means once and a negative Times means forever.
type Fault struct {
	// Op is the S3 operation name, for example "PutObject" or "CompleteMultipartUpload".
	Op string

	// KeyPrefix restricts the fault to keys beginning with it.
	KeyPrefix string

	// Skip is the number of matching requests that pass before the fault applies.
	Skip int32

	// Times is how many requests the fault applies to.
	Times int32

	// Action is the fault behavior.
	Action FaultAction

	// Status and Code describe the S3 error for [FailStatus]; they default to 500 InternalError.
	Status int32
	Code   string

	// Delay is the pause for [Delay].
	Delay time.Duration
}

// faultRule is a registered fault with its remaining counters.
type faultRule struct {
	Fault
	skip  int32
	times int32
}

// faults holds the registered rules.
type faults struct {
	mu    sync.Mutex
	rules []*faultRule
}

func (f *faults) add(fault *Fault) {
	f.mu.Lock()
	defer f.mu.Unlock()
	times := fault.Times
	if times == 0 {
		times = 1
	}
	f.rules = append(f.rules, &faultRule{Fault: *fault, skip: fault.Skip, times: times})
}

func (f *faults) clear() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules = nil
}

// match returns the fault to apply to req, consuming its counters.
func (f *faults) match(req *Request) *Fault {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rule := range f.rules {
		if rule.Op != "" && rule.Op != req.Op {
			continue
		}
		if rule.KeyPrefix != "" && !strings.HasPrefix(req.Key, rule.KeyPrefix) {
			continue
		}
		if rule.skip > 0 {
			rule.skip--
			continue
		}
		if rule.times == 0 {
			continue
		}
		if rule.times > 0 {
			rule.times--
		}
		fault := rule.Fault
		return &fault
	}
	return nil
}

// status returns the HTTP status and S3 code of a FailStatus fault.
func (f *Fault) status() (int, string) {
	status, code := int(f.Status), f.Code
	if status == 0 {
		status = http.StatusInternalServerError
	}
	if code == "" {
		code = "InternalError"
	}
	return status, code
}

// Inject registers a fault.
func (s *Server) Inject(fault Fault) {
	defer perf.Track(nil, "s3test.Server.Inject")()

	s.faults.add(&fault)
}

// ClearFaults removes every registered fault.
func (s *Server) ClearFaults() {
	defer perf.Track(nil, "s3test.Server.ClearFaults")()

	s.faults.clear()
}
