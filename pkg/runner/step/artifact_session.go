package step

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/artifact/report"
	"github.com/cloudposse/atmos/pkg/perf"
)

// artifactSession is the artifact state a [Variables] carries across the attempts and steps of one
// execution: the retained operation identities, and the report collector that execution
// boundaries record results into.
//
// A wrapper that retries a step (a workflow, a custom command, or a hook) calls the handler again
// with the same Variables, so the session is where an operation keeps its identity between
// attempts. Clones of a Variables share the session, so a parallel branch and its parent agree.
type artifactSession struct {
	mu         sync.Mutex
	collector  report.Collector
	operations map[string]*artifactOperation
	root       *artifactScope
}

// artifactScope numbers the logical operations of one sequential flow, and positions the parallel
// groups that flow starts, so a report orders operations by where they were scheduled rather than
// by when they finished. The root scope belongs to the session; ForkAt gives each branch of a
// parallel group a scope of its own, nested under the position of the group.
//
// A position is a list of integers compared lexicographically. Within a scope, operation n is
// positioned at path+[2n], and a parallel group that starts after n operations is positioned at
// path+[2n-1] with the branch index appended, so a group sorts between the operation before it and
// the operation after it. The parent scope is blocked while its group runs, so the count the group
// reads is stable.
type artifactScope struct {
	mu   sync.Mutex
	path []int
	next int
}

// nextPosition assigns the position of the next logical operation of the scope.
func (s *artifactScope) nextPosition() []int {
	s.mu.Lock()
	defer s.mu.Unlock()

	position := append(append(make([]int, 0, len(s.path)+1), s.path...), 2*s.next)
	s.next++
	return position
}

// branchPath returns the path of the scope of the branch at the given index of a parallel group
// that starts now.
func (s *artifactScope) branchPath(index int) []int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append(append(make([]int, 0, len(s.path)+2), s.path...), 2*s.next-1, index)
}

// artifactOperation is the identity of one logical operation: the ID and the UTC preparation time
// are captured once, before the first attempt, and reused by every retry.
type artifactOperation struct {
	id         string
	preparedAt time.Time
	attempts   int
	position   []int
}

func newArtifactSession() *artifactSession {
	return &artifactSession{collector: report.Discard{}, operations: make(map[string]*artifactOperation), root: &artifactScope{}}
}

// Clocks and ID generation are variables so tests can pin them.
var (
	artifactNow   = func() time.Time { return time.Now().UTC() }
	artifactNewID = uuid.NewString
)

// UseArtifactIdentity pins the clock and the operation ID generator that artifact steps use when they
// capture a new logical operation, and returns a function that restores them. It is the seam tests use
// to compare the results of different entry points byte for byte; production code never calls it.
func UseArtifactIdentity(newID func() string, now func() time.Time) (restore func()) {
	defer perf.Track(nil, "step.UseArtifactIdentity")()

	previousNow, previousID := artifactNow, artifactNewID
	artifactNow, artifactNewID = now, newID
	return func() { artifactNow, artifactNewID = previousNow, previousID }
}

// SetArtifactCollector injects the collector that artifact steps record their results into. A nil
// collector restores the no-op default. The collector is passed through the runner's services,
// never through a context value, and is shared by every Variables cloned from this one.
func (v *Variables) SetArtifactCollector(collector report.Collector) {
	defer perf.Track(nil, "step.Variables.SetArtifactCollector")()

	session := v.artifactSession()
	session.mu.Lock()
	defer session.mu.Unlock()
	if collector == nil {
		collector = report.Discard{}
	}
	session.collector = collector
}

// artifactScopeOrRoot returns the scope that numbers this Variables' operations: the scope of the
// parallel branch it was forked into, or the session's root scope.
func (v *Variables) artifactScopeOrRoot() *artifactScope {
	if v.artifactScope != nil {
		return v.artifactScope
	}
	return v.artifactSession().root
}

// ForkAt returns a copy of the variables for the branch at the given scheduled index of a parallel
// group. Artifact operations the branch runs are positioned under the group, in branch order,
// whatever order the branches finish in. The copy shares the session, so the branches still record
// into one collector.
func (v *Variables) ForkAt(index int) *Variables {
	defer perf.Track(nil, "step.Variables.ForkAt")()

	branch := v.Clone()
	branch.artifactScope = &artifactScope{path: v.artifactScopeOrRoot().branchPath(index)}
	return branch
}

// artifactSession returns the session, creating it for a Variables built without NewVariables.
func (v *Variables) artifactSession() *artifactSession {
	if v.artifacts == nil {
		v.artifacts = newArtifactSession()
	}
	return v.artifacts
}

// begin returns the identity for the logical operation with the given key and counts the attempt.
// The first call captures the ID, the preparation time, and the scheduled position in the scope;
// later calls, which are wrapper retries of the same step with the same resolved input, return
// them unchanged.
func (s *artifactSession) begin(key string, scope *artifactScope) (operation artifactOperation, collector report.Collector) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.operations[key]
	if !ok {
		existing = &artifactOperation{id: artifactNewID(), preparedAt: artifactNow(), position: scope.nextPosition()}
		s.operations[key] = existing
	}
	existing.attempts++
	return *existing, s.collector
}

// finish ends the logical operation after a success, so the same step run again later is a new
// operation. A failed attempt keeps its identity: the next attempt is a retry of the same operation.
func (s *artifactSession) finish(key string, succeeded bool) {
	if !succeeded {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.operations, key)
}

// artifactOperationKey identifies a logical operation by the step and its fully resolved input.
// Explicit user metadata participates in the identity; the execution fields do not.
func artifactOperationKey(stepName string, params *invoke.Params) string {
	identity := *params
	identity.Execution.OperationID = ""
	identity.Execution.PreparedAt = time.Time{}
	identity.Execution.MaxAttempts = 0
	encoded, err := json.Marshal(&identity)
	if err != nil {
		// Params holds only strings, booleans, and string maps, so encoding cannot fail; falling
		// back to the step name still yields a stable, if coarser, identity.
		return stepName
	}
	sum := sha256.Sum256(append([]byte(stepName+"\x00"), encoded...))
	return hex.EncodeToString(sum[:])
}
