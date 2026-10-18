package policy

import (
	"sort"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Explanation describes evaluation without exposing input values or expression errors.
type Explanation struct {
	Kind   InputKind `json:"kind"`
	Rule   string    `json:"rule,omitempty"`
	Status string    `json:"status"`
}

var explanationState struct {
	sync.Mutex
	emit    func(Explanation)
	reached map[InputKind]bool
}

// BeginExplanation installs an invocation-scoped, serialized diagnostic sink.
// The returned cleanup reports configured input kinds never reached by this invocation.
func BeginExplanation(emit func(Explanation), rules []Rule) func() {
	defer perf.Track(nil, "policy.BeginExplanation")()

	explanationState.Lock()
	previous, reached := explanationState.emit, explanationState.reached
	explanationState.emit, explanationState.reached = emit, make(map[InputKind]bool)
	explanationState.Unlock()
	return func() {
		explanationState.Lock()
		defer explanationState.Unlock()
		if emit != nil {
			missing := make(map[InputKind]bool)
			for i := range rules {
				rule := &rules[i]
				if !explanationState.reached[rule.On] {
					missing[rule.On] = true
				}
			}
			kinds := make([]string, 0, len(missing))
			for kind := range missing {
				kinds = append(kinds, string(kind))
			}
			sort.Strings(kinds)
			for _, kind := range kinds {
				emit(Explanation{Kind: InputKind(kind), Status: "not reached; no verdict"})
			}
		}
		explanationState.emit, explanationState.reached = previous, reached
	}
}

// Explain emits only explicit diagnostic metadata; it never changes enforcement.
func Explain(kind InputKind, rule, status string) {
	defer perf.Track(nil, "policy.Explain")()

	explanationState.Lock()
	defer explanationState.Unlock()
	if explanationState.emit != nil {
		explanationState.reached[kind] = true
		explanationState.emit(Explanation{Kind: kind, Rule: rule, Status: status})
	}
}
