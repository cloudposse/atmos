package step

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/artifact/report"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// artifactSession is the artifact state a [Variables] carries across the attempts and steps of one
// execution: the retained operation identities, and the report collector that execution
// boundaries record results into.
//
// A wrapper that retries a step (a workflow, a custom command, or a hook) calls the handler again
// with the same Variables, so the session is where an operation keeps its identity between
// attempts. Clones of a Variables share the session, so a parallel branch and its parent agree.
type artifactSession struct {
	mu             sync.Mutex
	collector      report.Collector
	operations     map[string]*artifactOperation
	inputs         map[string]invoke.Params
	fingerprintKey [32]byte
}

// artifactOperation is the identity of one logical operation: the ID and the UTC preparation time
// are captured once, before the first attempt, and reused by every retry.
type artifactOperation struct {
	id         string
	preparedAt time.Time
	attempts   int
}

func newArtifactSession() *artifactSession {
	session := &artifactSession{collector: report.Discard{}, operations: make(map[string]*artifactOperation), inputs: make(map[string]invoke.Params)}
	// Go 1.26+ guarantees Read fills the key or terminates the process on entropy failure.
	if _, err := rand.Read(session.fingerprintKey[:]); err != nil {
		panic("generate artifact session fingerprint key: " + err.Error())
	}
	return session
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

// artifactSession returns the session, creating it for a Variables built without NewVariables.
func (v *Variables) artifactSession() *artifactSession {
	if v.artifacts == nil {
		v.artifacts = newArtifactSession()
	}
	return v.artifacts
}

// begin returns the identity for the logical operation with the given key and counts the attempt.
// The first call captures the ID and preparation time; later calls, which are wrapper retries of
// the same step with the same resolved input, return them unchanged.
func (s *artifactSession) begin(key string) (operation artifactOperation, collector report.Collector) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.operations[key]
	if !ok {
		existing = &artifactOperation{id: artifactNewID(), preparedAt: artifactNow()}
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

// stepKey identifies the authored call independently of resolved templates. The fingerprint is
// scoped to this session and keyed with a secret, since step environment values may be sensitive.
func (s *artifactSession) stepKey(step *schema.WorkflowStep) (string, error) {
	encoded, err := json.Marshal(step)
	if err != nil {
		return "", fmt.Errorf("%w: cannot identify artifact step: %w", errUtils.ErrArtifactStepInvalidField, err)
	}
	return s.fingerprint("authored-step", encoded), nil
}

func (s *artifactSession) retainedInput(key string) (*invoke.Params, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.inputs[key]
	return &value, ok
}

func (s *artifactSession) retainInput(key string, params *invoke.Params) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inputs[key] = *params
}

func (s *artifactSession) forgetInput(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inputs, key)
}

// fingerprint produces a session-local, domain-separated identifier without retaining a
// deterministic digest of potentially sensitive authored or resolved values.
func (s *artifactSession) fingerprint(domain string, encoded []byte) string {
	mac := hmac.New(sha256.New, s.fingerprintKey[:])
	mac.Write([]byte(domain + "\x00"))
	mac.Write(encoded)
	return hex.EncodeToString(mac.Sum(nil))
}

// operationKey identifies a logical operation by the step and its fully resolved input.
// Explicit user metadata participates in the identity; the execution fields do not.
func (s *artifactSession) operationKey(stepName string, params *invoke.Params) string {
	identity := *params
	identity.Execution.OperationID = ""
	identity.Execution.PreparedAt = time.Time{}
	identity.Execution.MaxAttempts = 0
	encoded, err := json.Marshal(&identity)
	if err != nil {
		// Params holds only strings, booleans, and string maps, so encoding cannot fail.
		return s.fingerprint("operation-fallback", []byte(stepName))
	}
	return s.fingerprint("operation", append([]byte(stepName+"\x00"), encoded...))
}
