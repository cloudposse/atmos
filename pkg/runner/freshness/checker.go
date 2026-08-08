package freshness

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/cloudposse/atmos/pkg/condition"
	"github.com/cloudposse/atmos/pkg/hashfile"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// StateDir returns the project-relative directory where freshness state (recorded sources
// hashes for checksum.changed) persists, rooted under the project's own base path rather than
// an XDG user-cache directory specifically so it composes with the existing CI-cache feature:
// a user can add this same directory to ci.cache.includes: and freshness state survives across
// CI runs the same way `.terraform/` would.
func StateDir(basePath string) string {
	defer perf.Track(nil, "freshness.StateDir")()

	return filepath.Join(basePath, ".atmos", "cache", "freshness")
}

// Facts carries the freshness facts computed for one step evaluation, merged into the caller's
// schema.ConditionContext before evaluating `when:` (see pkg/condition's ChecksumChanged/
// TimestampChanged/PreconditionSuccess/Sources/Artifacts fields).
type Facts struct {
	ChecksumChanged     bool
	TimestampChanged    bool
	PreconditionSuccess bool
	// Sources/Artifacts are structured per-file records, populated only when a step's `when:`
	// actually references the bare `sources`/`artifacts` identifiers (see Compute) -- building
	// per-file mtime/checksum data is wasted work for the common case where only the
	// checksum.changed/timestamp.changed convenience facts are used.
	Sources   []condition.FileFact
	Artifacts []condition.FileFact
}

// LookupTool resolves one precondition.tools entry, mirroring exec.LookPath's signature.
// Abstracted for testability; the default is exec.LookPath directly -- no shell involved, so
// there's no which-vs-where cross-platform mismatch to work around.
type LookupTool func(name string) (string, error)

// Checker computes freshness Facts for a step's schema.Inputs/Artifacts/Precondition and
// persists checksum state across runs. Constructed via NewChecker with Options (>2-3 logical
// dependencies), defaulting to real production implementations.
type Checker struct {
	globber Globber
	hasher  func([]string) (string, error)
	store   StateStore
	lookup  LookupTool
}

// Option configures a Checker.
type Option func(*Checker)

// WithGlobber overrides the Globber (default: NewGlobber(), wrapping pkg/filesystem).
func WithGlobber(g Globber) Option {
	defer perf.Track(nil, "freshness.WithGlobber")()

	return func(c *Checker) { c.globber = g }
}

// WithHasher overrides the content hasher (default: pkg/hashfile.HashFiles).
func WithHasher(h func([]string) (string, error)) Option {
	defer perf.Track(nil, "freshness.WithHasher")()

	return func(c *Checker) { c.hasher = h }
}

// WithStateStore overrides the StateStore (default: NewStateStore(), one JSON file per key).
func WithStateStore(s StateStore) Option {
	defer perf.Track(nil, "freshness.WithStateStore")()

	return func(c *Checker) { c.store = s }
}

// WithLookupTool overrides the LookupTool (default: exec.LookPath).
func WithLookupTool(l LookupTool) Option {
	defer perf.Track(nil, "freshness.WithLookupTool")()

	return func(c *Checker) { c.lookup = l }
}

// NewChecker constructs a Checker with real production defaults, overridable via Option.
func NewChecker(opts ...Option) *Checker {
	defer perf.Track(nil, "freshness.NewChecker")()

	c := &Checker{
		globber: NewGlobber(),
		hasher:  hashfile.HashFiles,
		store:   NewStateStore(),
		lookup:  exec.LookPath,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Implicit `when:` defaults synthesized when a step declares Inputs/Artifacts/Precondition but no
// explicit `when:`. Checksum.changed already means "needs to run" (positive polarity), but
// precondition.success means the opposite: the precondition already holds, i.e. skip -- so the
// run-signal is its negation. See schema.Precondition's doc comment for why.
var (
	implicitChecksumChanged                    = schema.MustCondition("!cel checksum.changed")
	implicitPreconditionUnmet                  = schema.MustCondition("!cel !precondition.success")
	implicitChecksumChangedOrPreconditionUnmet = schema.MustCondition("!cel checksum.changed || !precondition.success")
)

// StepDeclarations groups a step's three freshness-related declarations (Inputs/Artifacts/
// Precondition are deliberate siblings, not one nested inside another -- see their own doc
// comments in pkg/schema/task.go for why). Grouped into one struct so EffectiveWhen/Compute stay
// within the project's per-function argument limit, and so callers build the group once and pass
// it to both.
type StepDeclarations struct {
	Inputs       *schema.Inputs
	Artifacts    *schema.Artifacts
	Precondition *schema.Precondition
}

// StepIdentity groups the location/identity parameters needed to compute and persist freshness
// state for one step.
type StepIdentity struct {
	BaseDir  string
	StateDir string
	Scope    string
	StepName string
}

// EffectiveWhen returns when, or an implicit condition synthesized from which of
// declared.Inputs/Artifacts/Precondition are present, if when is unset (zero) -- so declaring
// `inputs:`/`artifacts:`/`precondition:` alone (no explicit `when:`) is enough to skip a step
// whose work is already done, matching go-task's zero-boilerplate default.
func EffectiveWhen(when schema.Condition, declared StepDeclarations) schema.Condition {
	defer perf.Track(nil, "freshness.EffectiveWhen")()

	if !when.IsZero() {
		return when
	}
	hasFreshness := declared.Inputs != nil || declared.Artifacts != nil
	hasPrecondition := declared.Precondition != nil
	switch {
	case hasFreshness && hasPrecondition:
		return implicitChecksumChangedOrPreconditionUnmet
	case hasFreshness:
		return implicitChecksumChanged
	case hasPrecondition:
		return implicitPreconditionUnmet
	default:
		return when
	}
}

// freshnessFactIdentifiers lists every CEL identifier a freshness Checker can populate (see
// pkg/condition's Context.ChecksumChanged/TimestampChanged/PreconditionSuccess/Sources/Artifacts
// fields and pkg/condition/cel.go's matching declarations).
var freshnessFactIdentifiers = []string{"checksum", "timestamp", "precondition", "sources", "artifacts"}

// MentionsAnyFreshnessFact reports whether when references any freshness-derived CEL identifier.
// Callers that need a cheap "might this step run" answer without a freshness.Checker on hand yet
// (facts would all read as their Go zero value, i.e. always "unchanged" -- wrong even for a
// step's very first-ever run) should treat a true result as "assume runnable" and defer the real
// decision to wherever Compute's actual facts get evaluated, rather than evaluating when here
// against an empty Context.
func MentionsAnyFreshnessFact(when schema.Condition) bool {
	defer perf.Track(nil, "freshness.MentionsAnyFreshnessFact")()

	for _, ident := range freshnessFactIdentifiers {
		if when.MentionsCELIdentifier(ident) {
			return true
		}
	}
	return false
}

// freshnessNeeds records which facts a step's effective `when:` actually references, so Compute
// only does the expensive work (hashing, statting, building per-file records) for what's needed.
type freshnessNeeds struct {
	checksum        bool
	timestamp       bool
	sourceRecords   bool
	artifactRecords bool
}

func computeNeeds(effectiveWhen schema.Condition) freshnessNeeds {
	return freshnessNeeds{
		checksum:        effectiveWhen.MentionsCELIdentifier("checksum"),
		timestamp:       effectiveWhen.MentionsCELIdentifier("timestamp"),
		sourceRecords:   effectiveWhen.MentionsCELIdentifier("sources"),
		artifactRecords: effectiveWhen.MentionsCELIdentifier("artifacts"),
	}
}

func (n freshnessNeeds) sourceGlob() bool   { return n.checksum || n.timestamp || n.sourceRecords }
func (n freshnessNeeds) artifactGlob() bool { return n.checksum || n.timestamp || n.artifactRecords }

func sourceAndArtifactPatterns(inputs *schema.Inputs, artifacts *schema.Artifacts) ([]string, []string) {
	var sourcePatterns []string
	if inputs != nil {
		sourcePatterns = inputs.Sources
	}
	var artifactPatterns []string
	if artifacts != nil {
		artifactPatterns = artifacts.Paths
	}
	return sourcePatterns, artifactPatterns
}

// globResult carries the resolved sources/artifacts matches for one Compute call, grouped into
// a struct (rather than 4 separate return values) to stay within the project's per-function
// return-count limit.
type globResult struct {
	sourceMatches   []string
	artifactMatches []string
	// artifactsAllExist reports whether every artifacts.paths pattern matched at least one file
	// -- a missing artifact always forces a rerun regardless of what checksumChanged/
	// timestampChanged say.
	artifactsAllExist bool
}

// globSourcesAndArtifacts resolves the sources/artifacts glob patterns needed by needs.
func (c *Checker) globSourcesAndArtifacts(baseDir string, sourcePatterns, artifactPatterns []string, needs freshnessNeeds) (globResult, error) {
	result := globResult{artifactsAllExist: true}
	if needs.sourceGlob() && len(sourcePatterns) > 0 {
		matches, err := globAll(c.globber, baseDir, sourcePatterns)
		if err != nil {
			return globResult{}, err
		}
		result.sourceMatches = matches
	}
	if needs.artifactGlob() && len(artifactPatterns) > 0 {
		for _, pattern := range artifactPatterns {
			matches, globErr := c.globber.Glob(baseDir, pattern)
			if globErr != nil {
				return globResult{}, globErr
			}
			if len(matches) == 0 {
				result.artifactsAllExist = false
			}
			result.artifactMatches = append(result.artifactMatches, matches...)
		}
	}
	return result, nil
}

// buildStructuredRecords builds the per-file {path, mtime, checksum} records exposed as the bare
// `sources`/`artifacts` CEL facts, only for whichever side needs.sourceRecords/artifactRecords
// actually asks for.
func (c *Checker) buildStructuredRecords(needs freshnessNeeds, result globResult) (sources, artifacts []condition.FileFact, err error) {
	if needs.sourceRecords {
		if sources, err = c.buildFileFacts(result.sourceMatches); err != nil {
			return nil, nil, err
		}
	}
	if needs.artifactRecords {
		if artifacts, err = c.buildFileFacts(result.artifactMatches); err != nil {
			return nil, nil, err
		}
	}
	return sources, artifacts, nil
}

// computeChangedFacts computes the checksum.changed/timestamp.changed convenience facts, only
// for whichever needs.checksum/needs.timestamp actually asks for.
func (c *Checker) computeChangedFacts(needs freshnessNeeds, result globResult, id StepIdentity, sourcePatterns []string) (timestampChangedVal, checksumChangedVal bool, err error) {
	if needs.timestamp {
		timestampChangedVal = timestampChanged(result.sourceMatches, result.artifactMatches, result.artifactsAllExist)
	}
	if needs.checksum {
		key := c.stateKey(id.Scope, id.StepName, id.BaseDir, sourcePatterns)
		if checksumChangedVal, err = c.checksumChanged(result.sourceMatches, result.artifactsAllExist, id.StateDir, key); err != nil {
			return false, false, err
		}
	}
	return timestampChangedVal, checksumChangedVal, nil
}

// Compute lazily computes only the facts effectiveWhen actually references (see
// schema.Condition.MentionsCELIdentifier), so a step whose `when:` only mentions `timestamp`
// never pays the cost of hashing file content, and a step that never references the bare
// `sources`/`artifacts` identifiers never pays the cost of building per-file records. Id.BaseDir
// is the step's own working directory; id.StateDir is where checksum state persists;
// id.Scope+id.StepName form the stable per-step identity used to key that state (see stateKey).
func (c *Checker) Compute(effectiveWhen schema.Condition, declared StepDeclarations, id StepIdentity) (Facts, error) {
	defer perf.Track(nil, "freshness.Checker.Compute")()

	var facts Facts

	if declared.Precondition != nil {
		facts.PreconditionSuccess = c.resolvePrecondition(declared.Precondition)
	}
	if declared.Inputs == nil && declared.Artifacts == nil {
		return facts, nil
	}

	needs := computeNeeds(effectiveWhen)
	sourcePatterns, artifactPatterns := sourceAndArtifactPatterns(declared.Inputs, declared.Artifacts)

	result, err := c.globSourcesAndArtifacts(id.BaseDir, sourcePatterns, artifactPatterns, needs)
	if err != nil {
		return Facts{}, err
	}

	if facts.Sources, facts.Artifacts, err = c.buildStructuredRecords(needs, result); err != nil {
		return Facts{}, err
	}

	if facts.TimestampChanged, facts.ChecksumChanged, err = c.computeChangedFacts(needs, result, id, sourcePatterns); err != nil {
		return Facts{}, err
	}

	return facts, nil
}

// RecordSuccess persists the current sources checksum for the next run's comparison. Call ONLY
// after the step's own Execute() returns success -- a failed step must never mark itself falsely
// up to date. Precondition has no persisted state (exec.LookPath is always evaluated live), so
// it has nothing to record here -- callers gate this call on inputs/artifacts being declared at
// all, not on inputs.Sources being non-empty: an artifacts-only step (inputs == nil) still needs
// a record of the (empty) sources hash, or checksumChanged's `!found` branch reports "changed"
// forever and the step never stabilizes to "skip" even once its artifacts exist and are unchanged.
func (c *Checker) RecordSuccess(inputs *schema.Inputs, baseDir, stateDir, scope, stepName string) error {
	defer perf.Track(nil, "freshness.Checker.RecordSuccess")()

	var sourcePatterns []string
	if inputs != nil {
		sourcePatterns = inputs.Sources
	}
	sources, err := globAll(c.globber, baseDir, sourcePatterns)
	if err != nil {
		return err
	}
	hash, err := c.hasher(sources)
	if err != nil {
		return err
	}
	key := c.stateKey(scope, stepName, baseDir, sourcePatterns)
	return c.store.Save(stateDir, key, Record{SourcesHash: hash})
}

// resolvePrecondition reports whether every declared tool resolves on PATH. An empty Tools list
// is vacuously true (there is nothing left unsatisfied), though callers are expected to validate
// that a Precondition block declares at least one tool.
func (c *Checker) resolvePrecondition(precondition *schema.Precondition) bool {
	for _, tool := range precondition.Tools {
		if _, err := c.lookup(tool); err != nil {
			return false
		}
	}
	return true
}

// buildFileFacts stats each resolved path for its content-modification time and hashes its
// individual content, producing the structured per-file records exposed to `when:` as the bare
// `sources`/`artifacts` CEL lists (e.g. `sources.exists(s, artifacts.all(a, s.mtime > a.mtime))`).
// A path that can no longer be stat'd (removed between glob and stat) is skipped rather than
// erroring -- the same "missing is a signal, not a failure" stance the rest of this package takes.
func (c *Checker) buildFileFacts(paths []string) ([]condition.FileFact, error) {
	facts := make([]condition.FileFact, 0, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		hash, err := c.hasher([]string{p})
		if err != nil {
			return nil, err
		}
		facts = append(facts, condition.FileFact{Path: p, Mtime: info.ModTime().Unix(), Checksum: hash})
	}
	return facts, nil
}

func (c *Checker) checksumChanged(sources []string, artifactsAllExist bool, stateDir, key string) (bool, error) {
	if !artifactsAllExist {
		return true, nil
	}
	hash, err := c.hasher(sources)
	if err != nil {
		return false, err
	}
	record, found, err := c.store.Load(stateDir, key)
	if err != nil {
		return false, err
	}
	if !found {
		return true, nil
	}
	return record.SourcesHash != hash, nil
}

// timestampChanged compares the newest source mtime against the oldest artifact mtime,
// stateless (no persisted record) -- the tradeoff for being the cheap option: unreliable right
// after a fresh checkout, since git resets every file's mtime to checkout time.
func timestampChanged(sources, artifacts []string, artifactsAllExist bool) bool {
	if !artifactsAllExist {
		return true
	}
	if len(artifacts) == 0 {
		// Nothing to compare against; a bare `timestamp.changed` reference with no
		// artifacts: can't determine staleness statelessly, so default to always-rerun
		// rather than silently never-rerun.
		return len(sources) > 0
	}

	var maxSource time.Time
	for _, p := range sources {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.ModTime().After(maxSource) {
			maxSource = info.ModTime()
		}
	}

	var minArtifact time.Time
	for i, p := range artifacts {
		info, err := os.Stat(p)
		if err != nil {
			return true
		}
		if i == 0 || info.ModTime().Before(minArtifact) {
			minArtifact = info.ModTime()
		}
	}

	return maxSource.After(minArtifact)
}

// stateKey builds the stable per-step identity used to key persisted checksum state: scope
// (e.g. the declaring command/workflow's identity) + stepName + baseDir (absolute, so the same
// step in different checkouts/worktrees doesn't cross-contaminate) + sorted sources patterns
// (so editing the sources: list invalidates prior state).
func (c *Checker) stateKey(scope, stepName, baseDir string, sourcesPatterns []string) string {
	sorted := make([]string, len(sourcesPatterns))
	copy(sorted, sourcesPatterns)
	sort.Strings(sorted)

	h := sha256.New()
	for _, part := range append([]string{scope, stepName, baseDir}, sorted...) {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
