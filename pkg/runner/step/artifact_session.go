package step

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
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
// by when they finished. The root scope belongs to the session; each branch of a parallel group
// gets a scope of its own, nested under the position of the group.
//
// A position is a list of integers compared lexicographically. Every operation and every parallel
// group of a scope takes the next slot k of the scope: the operation is positioned at path+[k], and
// the branch at the given index of the group is scoped at path+[k, index]. Because a group takes a
// slot of its own, consecutive groups with no operation between them never share a prefix, and a
// group sorts between the operation before it and the operation after it.
type artifactScope struct {
	mu   sync.Mutex
	path []int
	next int
}

// reserve takes the next slot of the scope and returns the position of that slot.
func (s *artifactScope) reserve() []int {
	s.mu.Lock()
	defer s.mu.Unlock()

	position := append(append(make([]int, 0, len(s.path)+1), s.path...), s.next)
	s.next++
	return position
}

// key identifies the scope in an operation key, so identical steps in different branches stay
// separate operations while every retry in one branch, which shares the scope's path, stays one.
func (s *artifactScope) key() string {
	parts := make([]string, len(s.path))
	for index, slot := range s.path {
		parts[index] = strconv.Itoa(slot)
	}
	return strings.Join(parts, ".")
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

// BeginArtifactStep reserves a distinct scope for one scheduled step and returns a
// restore function. Call it around the entire retry loop, not each attempt.
func (v *Variables) BeginArtifactStep() func() {
	defer perf.Track(nil, "step.Variables.BeginArtifactStep")()

	previous := v.artifactScope
	v.artifactScope = &artifactScope{path: v.artifactScopeOrRoot().reserve()}
	return func() { v.artifactScope = previous }
}

// ScheduledGroup is one parallel group whose position in the scope of its parent has been reserved.
// The branches forked from it are positioned under the group, in branch order, whatever order they
// finish in.
type ScheduledGroup struct {
	parent *Variables
	prefix []int
}

// ScheduleGroup reserves the position of a parallel group that starts now, and returns the group the
// scheduler forks its branches from. Call it once per group, before the first branch starts: two
// groups with no artifact operation between them each get their own position, so their branches
// never interleave.
func (v *Variables) ScheduleGroup() *ScheduledGroup {
	defer perf.Track(nil, "step.Variables.ScheduleGroup")()

	return &ScheduledGroup{parent: v, prefix: v.artifactScopeOrRoot().reserve()}
}

// ForkAt returns a copy of the parent variables for the branch at the given scheduled index of the
// group. The copy shares the session, so the branches still record into one collector, and a retry
// of the branch forks again at the same index and keeps the branch's operation identities.
func (g *ScheduledGroup) ForkAt(index int) *Variables {
	defer perf.Track(nil, "step.ScheduledGroup.ForkAt")()

	branch := g.parent.Clone()
	path := append(append(make([]int, 0, len(g.prefix)+1), g.prefix...), index)
	branch.artifactScope = &artifactScope{path: path}
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
		existing = &artifactOperation{id: artifactNewID(), preparedAt: artifactNow(), position: scope.reserve()}
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

// artifactOperationLabel domain-separates operation keys; it is a public label, not a secret.
var artifactOperationLabel = []byte("atmos.artifact.step.operation.v1")

// artifactOperationKey identifies a logical operation by the scope it runs in, the step, and its
// fully resolved input. Explicit user metadata participates in the identity; the execution fields
// do not. The scope keeps identical steps of overlapping parallel branches separate operations,
// while the retries of one branch, which run in the same scope, stay one.
func artifactOperationKey(scope *artifactScope, stepName string, params *invoke.Params) string {
	identity := *params
	identity.Execution.OperationID = ""
	identity.Execution.PreparedAt = time.Time{}
	identity.Execution.MaxAttempts = 0
	encoded, err := json.Marshal(&identity)
	if err != nil {
		// Params holds only strings, booleans, and string maps, so encoding cannot fail; falling
		// back to the step name still yields a stable, if coarser, identity.
		return scope.key() + "\x00" + stepName
	}
	// The key only names an in-memory operation. A fixed public label keys the digest so it is not
	// mistaken for hashing a credential that the encoded parameters may carry.
	mac := hmac.New(sha256.New, artifactOperationLabel)
	mac.Write([]byte(scope.key() + "\x00" + stepName + "\x00"))
	mac.Write(encoded)
	return hex.EncodeToString(mac.Sum(nil))
}
