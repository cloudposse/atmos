package freshness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/condition"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeGlobber returns a fixed match list per pattern, ignoring baseDir.
type fakeGlobber struct {
	matches map[string][]string
}

func (f fakeGlobber) Glob(_, pattern string) ([]string, error) {
	return f.matches[pattern], nil
}

// fakeStateStore is an in-memory StateStore for tests.
type fakeStateStore struct {
	records map[string]Record
}

func newFakeStateStore() *fakeStateStore {
	return &fakeStateStore{records: map[string]Record{}}
}

func (f *fakeStateStore) Load(_, key string) (Record, bool, error) {
	r, ok := f.records[key]
	return r, ok, nil
}

func (f *fakeStateStore) Save(_, key string, r Record) error {
	f.records[key] = r
	return nil
}

func checksumChangedCondition(t *testing.T) schema.Condition {
	t.Helper()
	cond, err := schema.NewCondition("!cel checksum.changed")
	require.NoError(t, err)
	return cond
}

func timestampChangedCondition(t *testing.T) schema.Condition {
	t.Helper()
	cond, err := schema.NewCondition("!cel timestamp.changed")
	require.NoError(t, err)
	return cond
}

func sourcesRecordsCondition(t *testing.T) schema.Condition {
	t.Helper()
	cond, err := schema.NewCondition("!cel sources.exists(s, artifacts.all(a, s.mtime > a.mtime))")
	require.NoError(t, err)
	return cond
}

// computeFor adapts Checker.Compute's grouped-struct signature (StepDeclarations/StepIdentity,
// kept small to stay within the project's per-function argument limit) back to individual
// arguments, so this file's many scenarios stay terse.
func computeFor(c *Checker, when schema.Condition, inputs *schema.Inputs, artifacts *schema.Artifacts, precondition *schema.Precondition, baseDir, stateDir, scope, stepName string) (Facts, error) {
	declared := StepDeclarations{Inputs: inputs, Artifacts: artifacts, Precondition: precondition}
	id := StepIdentity{BaseDir: baseDir, StateDir: stateDir, Scope: scope, StepName: stepName}
	return c.Compute(when, declared, id)
}

// effectiveWhenFor adapts EffectiveWhen's grouped-struct signature the same way.
func effectiveWhenFor(when schema.Condition, inputs *schema.Inputs, artifacts *schema.Artifacts, precondition *schema.Precondition) schema.Condition {
	return EffectiveWhen(when, StepDeclarations{Inputs: inputs, Artifacts: artifacts, Precondition: precondition})
}

func TestChecker_ChecksumUnchangedSkips(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(srcFile, []byte("package main"), 0o644))
	artifactFile := filepath.Join(tmpDir, "bin", "app")
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactFile), 0o755))
	require.NoError(t, os.WriteFile(artifactFile, []byte("binary"), 0o644))

	store := newFakeStateStore()
	checker := NewChecker(
		WithGlobber(fakeGlobber{matches: map[string][]string{
			"*.go":    {srcFile},
			"bin/app": {artifactFile},
		}}),
		WithStateStore(store),
	)

	inputs := &schema.Inputs{Sources: []string{"*.go"}}
	artifacts := &schema.Artifacts{Paths: []string{"bin/app"}}
	when := checksumChangedCondition(t)

	// First run: no recorded state -> changed.
	facts, err := computeFor(checker, when, inputs, artifacts, nil, tmpDir, tmpDir, "cmd:build", "compile")
	require.NoError(t, err)
	assert.True(t, facts.ChecksumChanged, "first run must report changed (no prior state)")

	require.NoError(t, checker.RecordSuccess(inputs, tmpDir, tmpDir, "cmd:build", "compile"))

	// Second run, unchanged content -> not changed.
	facts, err = computeFor(checker, when, inputs, artifacts, nil, tmpDir, tmpDir, "cmd:build", "compile")
	require.NoError(t, err)
	assert.False(t, facts.ChecksumChanged, "unchanged sources after a recorded success must report unchanged")
}

func TestChecker_ChecksumChangedReruns(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(srcFile, []byte("package main"), 0o644))
	artifactFile := filepath.Join(tmpDir, "app")
	require.NoError(t, os.WriteFile(artifactFile, []byte("binary"), 0o644))

	store := newFakeStateStore()
	globber := fakeGlobber{matches: map[string][]string{"*.go": {srcFile}, "app": {artifactFile}}}
	checker := NewChecker(WithGlobber(globber), WithStateStore(store))

	inputs := &schema.Inputs{Sources: []string{"*.go"}}
	artifacts := &schema.Artifacts{Paths: []string{"app"}}
	when := checksumChangedCondition(t)

	require.NoError(t, checker.RecordSuccess(inputs, tmpDir, tmpDir, "cmd:build", "compile"))

	facts, err := computeFor(checker, when, inputs, artifacts, nil, tmpDir, tmpDir, "cmd:build", "compile")
	require.NoError(t, err)
	assert.False(t, facts.ChecksumChanged)

	// Modify the source file's content.
	require.NoError(t, os.WriteFile(srcFile, []byte("package main // changed"), 0o644))

	facts, err = computeFor(checker, when, inputs, artifacts, nil, tmpDir, tmpDir, "cmd:build", "compile")
	require.NoError(t, err)
	assert.True(t, facts.ChecksumChanged, "changed source content must report changed")
}

func TestChecker_ArtifactsMissingForcesRerunEvenIfSourcesUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(srcFile, []byte("package main"), 0o644))

	store := newFakeStateStore()
	// artifacts: pattern matches nothing -- the output doesn't exist.
	globber := fakeGlobber{matches: map[string][]string{"*.go": {srcFile}, "app": {}}}
	checker := NewChecker(WithGlobber(globber), WithStateStore(store))

	inputs := &schema.Inputs{Sources: []string{"*.go"}}
	artifacts := &schema.Artifacts{Paths: []string{"app"}}
	when := checksumChangedCondition(t)

	require.NoError(t, checker.RecordSuccess(inputs, tmpDir, tmpDir, "cmd:build", "compile"))

	facts, err := computeFor(checker, when, inputs, artifacts, nil, tmpDir, tmpDir, "cmd:build", "compile")
	require.NoError(t, err)
	assert.True(t, facts.ChecksumChanged, "a missing artifacts: output must force a rerun even though sources are unchanged")
}

// TestChecker_ArtifactsOnlyStepStabilizesAfterRecordSuccess guards against a real bug: RecordSuccess
// used to early-return when inputs == nil, so an artifacts-only step (no inputs: declared) never
// persisted state and permanently reported ChecksumChanged == true, rerunning forever even once its
// artifact existed and was unchanged.
func TestChecker_ArtifactsOnlyStepStabilizesAfterRecordSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	artifactFile := filepath.Join(tmpDir, "bin", "app")

	store := newFakeStateStore()
	globber := fakeGlobber{matches: map[string][]string{"bin/app": {}}}
	checker := NewChecker(WithGlobber(globber), WithStateStore(store))

	artifacts := &schema.Artifacts{Paths: []string{"bin/app"}}
	when := checksumChangedCondition(t)

	// First run: artifact doesn't exist yet -> must report changed.
	facts, err := computeFor(checker, when, nil, artifacts, nil, tmpDir, tmpDir, "cmd:build", "compile")
	require.NoError(t, err)
	assert.True(t, facts.ChecksumChanged, "first run must report changed (artifact missing)")

	// The step "runs" and produces the artifact, then records success (inputs == nil).
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactFile), 0o755))
	require.NoError(t, os.WriteFile(artifactFile, []byte("binary"), 0o644))
	globber.matches["bin/app"] = []string{artifactFile}
	require.NoError(t, checker.RecordSuccess(nil, tmpDir, tmpDir, "cmd:build", "compile"))

	// Second run: artifact now exists and nothing changed -> must report unchanged (skip).
	facts, err = computeFor(checker, when, nil, artifacts, nil, tmpDir, tmpDir, "cmd:build", "compile")
	require.NoError(t, err)
	assert.False(t, facts.ChecksumChanged, "an artifacts-only step must stabilize to unchanged once RecordSuccess has persisted state")
}

func TestChecker_RecordSuccessNotCalledOnFailureIsCallerResponsibility(t *testing.T) {
	// Compute alone must never mutate state -- only RecordSuccess does, and callers are
	// documented to invoke it only after a successful Execute(). This test asserts Compute is
	// read-only: calling it twice in a row without RecordSuccess yields the same "changed"
	// answer both times (no implicit recording).
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(srcFile, []byte("package main"), 0o644))
	artifactFile := filepath.Join(tmpDir, "app")
	require.NoError(t, os.WriteFile(artifactFile, []byte("binary"), 0o644))

	store := newFakeStateStore()
	globber := fakeGlobber{matches: map[string][]string{"*.go": {srcFile}, "app": {artifactFile}}}
	checker := NewChecker(WithGlobber(globber), WithStateStore(store))
	inputs := &schema.Inputs{Sources: []string{"*.go"}}
	artifacts := &schema.Artifacts{Paths: []string{"app"}}
	when := checksumChangedCondition(t)

	first, err := computeFor(checker, when, inputs, artifacts, nil, tmpDir, tmpDir, "cmd:build", "compile")
	require.NoError(t, err)
	second, err := computeFor(checker, when, inputs, artifacts, nil, tmpDir, tmpDir, "cmd:build", "compile")
	require.NoError(t, err)
	assert.Equal(t, first.ChecksumChanged, second.ChecksumChanged)
	assert.Empty(t, store.records, "Compute must never write state itself")
}

func TestChecker_MethodTimestampStateless(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "main.go")
	artifactFile := filepath.Join(tmpDir, "app")
	require.NoError(t, os.WriteFile(srcFile, []byte("package main"), 0o644))
	require.NoError(t, os.WriteFile(artifactFile, []byte("binary"), 0o644))

	// Set explicit, well-separated mtimes rather than relying on write-order timing, which can
	// tie at low filesystem timestamp resolution.
	now := time.Now()
	require.NoError(t, os.Chtimes(srcFile, now, now))
	require.NoError(t, os.Chtimes(artifactFile, now.Add(1*time.Hour), now.Add(1*time.Hour)))

	store := newFakeStateStore()
	globber := fakeGlobber{matches: map[string][]string{"*.go": {srcFile}, "app": {artifactFile}}}
	checker := NewChecker(WithGlobber(globber), WithStateStore(store))
	inputs := &schema.Inputs{Sources: []string{"*.go"}}
	artifacts := &schema.Artifacts{Paths: []string{"app"}}
	when := timestampChangedCondition(t)

	// artifact is newer than source -> not changed.
	facts, err := computeFor(checker, when, inputs, artifacts, nil, tmpDir, tmpDir, "cmd:build", "compile")
	require.NoError(t, err)
	assert.False(t, facts.TimestampChanged)
	assert.Empty(t, store.records, "timestamp method must never touch persisted state")

	// Touch the source file to be newer than the artifact.
	newer := now.Add(2 * time.Hour)
	require.NoError(t, os.Chtimes(srcFile, newer, newer))

	facts, err = computeFor(checker, when, inputs, artifacts, nil, tmpDir, tmpDir, "cmd:build", "compile")
	require.NoError(t, err)
	assert.True(t, facts.TimestampChanged, "a source newer than the artifact must report changed")
}

func TestChecker_CorruptedStateTreatedAsMiss(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))

	store := NewStateStore()
	checker := NewChecker(WithStateStore(store), WithGlobber(fakeGlobber{matches: map[string][]string{
		"*.go": {}, "app": {},
	}}))
	key := checker.stateKey("cmd:build", "compile", tmpDir, []string{"*.go"})
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, key+".json"), []byte("not-json"), 0o600))

	record, found, err := store.Load(stateDir, key)
	require.NoError(t, err)
	assert.False(t, found, "corrupted state file must be treated as a cache miss, not an error")
	assert.Equal(t, Record{}, record)
}

func TestFileStateStore_LoadOnNonexistentStateDirIsAMissNotAnError(t *testing.T) {
	store := NewStateStore()
	stateDir := filepath.Join(t.TempDir(), "does", "not", "exist", "yet")

	record, found, err := store.Load(stateDir, "some-key")
	require.NoError(t, err, "a state dir that has never been created (no Save yet) must not error")
	assert.False(t, found)
	assert.Equal(t, Record{}, record)
}

// TestFileStateStore_ConcurrentAccessDoesNotCorrupt fires many concurrent Save/Load calls at the
// same key and asserts the file is never corrupted (always valid JSON matching one of the
// attempted values) -- not that every individual call succeeds. On Windows, pkg/cache.FileLock
// is explicitly best-effort (no real mutual exclusion there -- see its own doc comment), so an
// individual Save or Load CAN transiently fail there under genuine concurrency (e.g. a rename
// racing a concurrent reader's open handle); RecordSuccess's real callers already treat that as
// log-and-continue, never fatal, which is what this test exercises instead of a stricter
// zero-transient-errors bar this package's underlying lock doesn't actually provide on Windows.
func TestFileStateStore_ConcurrentAccessDoesNotCorrupt(t *testing.T) {
	stateDir := t.TempDir()
	store := NewStateStore()
	key := "concurrent-key"

	var wg sync.WaitGroup
	var succeeded atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			hash := fmt.Sprintf("hash-%d", n)
			if err := store.Save(stateDir, key, Record{SourcesHash: hash}); err == nil {
				succeeded.Add(1)
			}
			// Load errors are tolerated for the same best-effort-on-Windows reason; only the
			// final post-wg.Wait() Load below must succeed and be uncorrupted.
			_, _, _ = store.Load(stateDir, key)
		}(i)
	}
	wg.Wait()

	require.Positive(t, succeeded.Load(), "at least one of 20 concurrent writers must succeed")

	// The file must be valid, parseable JSON reflecting SOME writer's value -- not corrupted,
	// interleaved bytes from two concurrent writers.
	record, found, err := store.Load(stateDir, key)
	require.NoError(t, err)
	require.True(t, found)
	assert.Regexp(t, `^hash-\d+$`, record.SourcesHash)
}

func TestEffectiveWhen(t *testing.T) {
	explicit := checksumChangedCondition(t)
	inputs := &schema.Inputs{Sources: []string{"*.go"}}
	artifacts := &schema.Artifacts{Paths: []string{"app"}}
	precondition := &schema.Precondition{Tools: []string{"stringer"}}

	cases := []struct {
		name                    string
		inputs                  *schema.Inputs
		artifacts               *schema.Artifacts
		precondition            *schema.Precondition
		wantChecksumMentioned   bool
		wantPreconditionMention bool
		wantZero                bool
	}{
		{name: "inputs only", inputs: inputs, wantChecksumMentioned: true},
		{name: "artifacts only", artifacts: artifacts, wantChecksumMentioned: true},
		{name: "inputs and artifacts", inputs: inputs, artifacts: artifacts, wantChecksumMentioned: true},
		{name: "precondition only", precondition: precondition, wantPreconditionMention: true},
		{name: "inputs and precondition", inputs: inputs, precondition: precondition, wantChecksumMentioned: true, wantPreconditionMention: true},
		{name: "artifacts and precondition", artifacts: artifacts, precondition: precondition, wantChecksumMentioned: true, wantPreconditionMention: true},
		{name: "all three", inputs: inputs, artifacts: artifacts, precondition: precondition, wantChecksumMentioned: true, wantPreconditionMention: true},
		{name: "none", wantZero: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := effectiveWhenFor(schema.Condition{}, tc.inputs, tc.artifacts, tc.precondition)
			if tc.wantZero {
				assert.True(t, result.IsZero())
				return
			}
			assert.Equal(t, tc.wantChecksumMentioned, result.MentionsCELIdentifier("checksum"))
			assert.Equal(t, tc.wantPreconditionMention, result.MentionsCELIdentifier("precondition"))
		})
	}

	t.Run("explicit when is never overridden", func(t *testing.T) {
		result := effectiveWhenFor(explicit, inputs, artifacts, precondition)
		assert.Equal(t, explicit, result)
	})
}

// TestEffectiveWhen_PreconditionPolarityIsNegated is a targeted regression test for the bug
// found while re-deriving this design: the implicit precondition-only default must be
// "!precondition.success" (run when NOT already satisfied), not bare "precondition.success"
// (which would run only when already satisfied -- backwards).
func TestEffectiveWhen_PreconditionPolarityIsNegated(t *testing.T) {
	result := effectiveWhenFor(schema.Condition{}, nil, nil, &schema.Precondition{Tools: []string{"stringer"}})

	ctxAlreadyInstalled := condition.Context{PreconditionSuccess: true}
	runs, err := result.EvaluateE(ctxAlreadyInstalled)
	require.NoError(t, err)
	assert.False(t, runs, "must NOT run when the tool is already on PATH (precondition already satisfied)")

	ctxMissing := condition.Context{PreconditionSuccess: false}
	runs, err = result.EvaluateE(ctxMissing)
	require.NoError(t, err)
	assert.True(t, runs, "must run when the tool is NOT on PATH (precondition unmet)")
}

func TestChecker_PreconditionAllToolsResolveSucceeds(t *testing.T) {
	checker := NewChecker(WithLookupTool(func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}))
	precondition := &schema.Precondition{Tools: []string{"stringer", "protoc"}}

	facts, err := computeFor(checker, schema.MustCondition("!cel !precondition.success"), nil, nil, precondition, ".", ".", "cmd:x", "step")
	require.NoError(t, err)
	assert.True(t, facts.PreconditionSuccess)
}

func TestChecker_PreconditionAnyMissingToolFails(t *testing.T) {
	checker := NewChecker(WithLookupTool(func(name string) (string, error) {
		if name == "stringer" {
			return "/usr/bin/stringer", nil
		}
		return "", errors.New("exec: \"missing-tool\": executable file not found in $PATH")
	}))
	precondition := &schema.Precondition{Tools: []string{"stringer", "missing-tool"}}

	facts, err := computeFor(checker, schema.MustCondition("!cel !precondition.success"), nil, nil, precondition, ".", ".", "cmd:x", "step")
	require.NoError(t, err)
	assert.False(t, facts.PreconditionSuccess)
}

func TestChecker_PreconditionEmptyToolsIsVacuouslyTrue(t *testing.T) {
	checker := NewChecker(WithLookupTool(func(string) (string, error) {
		t.Fatal("lookup must not be called for an empty Tools list")
		return "", nil
	}))
	facts, err := computeFor(checker, schema.Condition{}, nil, nil, &schema.Precondition{}, ".", ".", "cmd:x", "step")
	require.NoError(t, err)
	assert.True(t, facts.PreconditionSuccess)
}

func TestChecker_PreconditionAlwaysComputedRegardlessOfWhenReferences(t *testing.T) {
	// Unlike checksum/timestamp/structured records, precondition resolution is cheap
	// (exec.LookPath, no shell, no hashing) and is always computed whenever Precondition != nil,
	// mirroring how the pre-rename `check:` field always ran regardless of lazy gating.
	calls := 0
	checker := NewChecker(WithLookupTool(func(string) (string, error) {
		calls++
		return "/usr/bin/x", nil
	}))
	_, err := computeFor(checker, schema.Condition{}, nil, nil, &schema.Precondition{Tools: []string{"x"}}, ".", ".", "cmd:x", "step")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestChecker_StructuredRecordsOnlyBuiltWhenReferenced(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "main.go")
	artifactFile := filepath.Join(tmpDir, "app")
	require.NoError(t, os.WriteFile(srcFile, []byte("package main"), 0o644))
	require.NoError(t, os.WriteFile(artifactFile, []byte("binary"), 0o644))

	globber := fakeGlobber{matches: map[string][]string{"*.go": {srcFile}, "app": {artifactFile}}}
	checker := NewChecker(WithGlobber(globber), WithStateStore(newFakeStateStore()))
	inputs := &schema.Inputs{Sources: []string{"*.go"}}
	artifacts := &schema.Artifacts{Paths: []string{"app"}}

	t.Run("checksum.changed alone never builds structured records", func(t *testing.T) {
		facts, err := computeFor(checker, checksumChangedCondition(t), inputs, artifacts, nil, tmpDir, tmpDir, "cmd:build", "compile")
		require.NoError(t, err)
		assert.Empty(t, facts.Sources, "building per-file records is wasted work when when: never references the bare identifier")
		assert.Empty(t, facts.Artifacts)
	})

	t.Run("bare sources/artifacts reference builds structured records", func(t *testing.T) {
		facts, err := computeFor(checker, sourcesRecordsCondition(t), inputs, artifacts, nil, tmpDir, tmpDir, "cmd:build", "compile")
		require.NoError(t, err)
		require.Len(t, facts.Sources, 1)
		require.Len(t, facts.Artifacts, 1)
		assert.Equal(t, srcFile, facts.Sources[0].Path)
		assert.NotZero(t, facts.Sources[0].Mtime)
		assert.NotEmpty(t, facts.Sources[0].Checksum)
		assert.Equal(t, artifactFile, facts.Artifacts[0].Path)
	})
}

func TestChecker_BuildFileFactsSkipsUnstatablePaths(t *testing.T) {
	tmpDir := t.TempDir()
	checker := NewChecker()
	facts, err := checker.buildFileFacts([]string{filepath.Join(tmpDir, "does-not-exist")})
	require.NoError(t, err)
	assert.Empty(t, facts, "a path removed between glob and stat must be skipped, not error")
}

func TestFacts_ZeroValueHasNoFactsSet(t *testing.T) {
	checker := NewChecker()
	facts, err := computeFor(checker, schema.Condition{}, nil, nil, nil, ".", ".", "cmd:x", "step")
	require.NoError(t, err)
	assert.Equal(t, Facts{}, facts)
}

func TestStateDir(t *testing.T) {
	assert.Equal(t, filepath.Join("base", ".atmos", "cache", "freshness"), StateDir("base"))
}

func TestWithHasher(t *testing.T) {
	called := false
	checker := NewChecker(WithHasher(func(paths []string) (string, error) {
		called = true
		return "fixed-hash", nil
	}), WithGlobber(fakeGlobber{matches: map[string][]string{"*.go": {"main.go"}}}), WithStateStore(newFakeStateStore()))

	inputs := &schema.Inputs{Sources: []string{"*.go"}}
	require.NoError(t, checker.RecordSuccess(inputs, ".", ".", "cmd:x", "step"))
	assert.True(t, called, "WithHasher's override must be used instead of the default")
}

func TestRecordSuccess_GlobErrorPropagates(t *testing.T) {
	checker := NewChecker(WithGlobber(erroringGlobber{}))
	err := checker.RecordSuccess(&schema.Inputs{Sources: []string{"*.go"}}, ".", ".", "cmd:x", "step")
	require.Error(t, err)
}

func TestRecordSuccess_HasherErrorPropagates(t *testing.T) {
	checker := NewChecker(
		WithGlobber(fakeGlobber{matches: map[string][]string{"*.go": {"main.go"}}}),
		WithHasher(func([]string) (string, error) { return "", errors.New("hash failed") }),
	)
	err := checker.RecordSuccess(&schema.Inputs{Sources: []string{"*.go"}}, ".", ".", "cmd:x", "step")
	require.Error(t, err)
}

func TestChecksumChanged_HasherErrorPropagates(t *testing.T) {
	checker := NewChecker(
		WithGlobber(fakeGlobber{matches: map[string][]string{"*.go": {"main.go"}, "app": {"app"}}}),
		WithHasher(func([]string) (string, error) { return "", errors.New("hash failed") }),
	)
	_, err := computeFor(checker, checksumChangedCondition(t), &schema.Inputs{Sources: []string{"*.go"}}, &schema.Artifacts{Paths: []string{"app"}}, nil, ".", ".", "cmd:x", "step")
	require.Error(t, err)
}

// erroringGlobber always fails, for exercising error-propagation paths.
type erroringGlobber struct{}

func (erroringGlobber) Glob(_, _ string) ([]string, error) {
	return nil, errors.New("glob failed")
}

func TestDefaultGlobber_ResolvesRealFiles(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("x"), 0o644))

	g := NewGlobber()
	matches, err := g.Glob(tmpDir, "*.go")
	require.NoError(t, err)
	assert.Len(t, matches, 1)
}

func TestDefaultGlobber_MissingBaseDirIsNotAnError(t *testing.T) {
	g := NewGlobber()
	matches, err := g.Glob(filepath.Join(t.TempDir(), "does-not-exist"), "*.go")
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestGlobAll_PropagatesError(t *testing.T) {
	_, err := globAll(erroringGlobber{}, ".", []string{"*.go"})
	require.Error(t, err)
}

func TestGlobAll_DeduplicatesAcrossPatterns(t *testing.T) {
	g := fakeGlobber{matches: map[string][]string{
		"*.go":  {"a.go", "b.go"},
		"a.go*": {"a.go"},
	}}
	matches, err := globAll(g, ".", []string{"*.go", "a.go*"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a.go", "b.go"}, matches)
}

func TestFileStateStore_SaveThenLoadRoundTrips(t *testing.T) {
	store := NewStateStore()
	stateDir := t.TempDir()
	require.NoError(t, store.Save(stateDir, "key", Record{SourcesHash: "abc"}))

	record, found, err := store.Load(stateDir, "key")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "abc", record.SourcesHash)
}

func TestTimestampChanged_ArtifactsMissingForcesChanged(t *testing.T) {
	assert.True(t, timestampChanged([]string{"src.go"}, []string{"app"}, false))
}

func TestTimestampChanged_NoArtifactsDeclaredWithSourcesIsChanged(t *testing.T) {
	assert.True(t, timestampChanged([]string{"src.go"}, nil, true))
}

func TestTimestampChanged_NoArtifactsAndNoSourcesIsUnchanged(t *testing.T) {
	assert.False(t, timestampChanged(nil, nil, true))
}

func TestTimestampChanged_UnstatableSourceIsSkipped(t *testing.T) {
	tmpDir := t.TempDir()
	artifact := filepath.Join(tmpDir, "app")
	require.NoError(t, os.WriteFile(artifact, []byte("x"), 0o644))

	// The only "source" cannot be stat'd -- it must be skipped (treated as epoch zero), not
	// error, leaving maxSource before the artifact's real mtime -> unchanged.
	assert.False(t, timestampChanged([]string{filepath.Join(tmpDir, "missing.go")}, []string{artifact}, true))
}

func TestTimestampChanged_UnstatableArtifactForcesChanged(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))

	assert.True(t, timestampChanged([]string{src}, []string{filepath.Join(tmpDir, "missing-app")}, true))
}

func TestCompute_SourceGlobErrorPropagates(t *testing.T) {
	checker := NewChecker(WithGlobber(erroringGlobber{}))
	inputs := &schema.Inputs{Sources: []string{"*.go"}}
	_, err := computeFor(checker, checksumChangedCondition(t), inputs, nil, nil, ".", ".", "cmd:x", "step")
	require.Error(t, err)
}

func TestCompute_ArtifactGlobErrorPropagates(t *testing.T) {
	checker := NewChecker(WithGlobber(erroringGlobber{}))
	artifacts := &schema.Artifacts{Paths: []string{"app"}}
	_, err := computeFor(checker, checksumChangedCondition(t), nil, artifacts, nil, ".", ".", "cmd:x", "step")
	require.Error(t, err)
}

func TestCompute_SourceRecordsBuildErrorPropagates(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(srcFile, []byte("x"), 0o644))

	checker := NewChecker(
		WithGlobber(fakeGlobber{matches: map[string][]string{"*.go": {srcFile}}}),
		WithHasher(func([]string) (string, error) { return "", errors.New("hash failed") }),
	)
	inputs := &schema.Inputs{Sources: []string{"*.go"}}
	_, err := computeFor(checker, sourcesRecordsCondition(t), inputs, nil, nil, tmpDir, tmpDir, "cmd:x", "step")
	require.Error(t, err)
}

func TestBuildFileFacts_HasherErrorPropagates(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))

	checker := NewChecker(WithHasher(func([]string) (string, error) { return "", errors.New("hash failed") }))
	_, err := checker.buildFileFacts([]string{f})
	require.Error(t, err)
}

// artifactsRecordsCondition returns a condition referencing the bare `artifacts` identifier only
// (not `sources`), used to isolate the artifacts-side of structured-record building from the
// sources-side (already isolated by sourcesRecordsCondition, which mentions both).
func artifactsRecordsCondition(t *testing.T) schema.Condition {
	t.Helper()
	cond, err := schema.NewCondition("!cel artifacts.exists(a, a.mtime > 0)")
	require.NoError(t, err)
	return cond
}

// TestMentionsAnyFreshnessFact exercises both the early-return-true branch (a freshness
// identifier is referenced) and the loop-exhaustion return-false branch (none are), including
// the real zero-value Condition callers hit before a freshness.Checker exists yet (see the
// function's own doc comment about deferring to Compute's actual facts).
func TestMentionsAnyFreshnessFact(t *testing.T) {
	cases := []struct {
		name string
		cond schema.Condition
		want bool
	}{
		{name: "zero-value condition mentions nothing", cond: schema.Condition{}, want: false},
		{name: "unrelated CEL expression mentions nothing", cond: schema.MustCondition("!cel 1 == 1"), want: false},
		{name: "mentions checksum", cond: checksumChangedCondition(t), want: true},
		{name: "mentions timestamp", cond: timestampChangedCondition(t), want: true},
		{name: "mentions precondition", cond: schema.MustCondition("!cel !precondition.success"), want: true},
		{name: "mentions sources", cond: sourcesRecordsCondition(t), want: true},
		{name: "mentions artifacts", cond: artifactsRecordsCondition(t), want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, MentionsAnyFreshnessFact(tc.cond))
		})
	}
}

// TestCompute_ArtifactRecordsBuildErrorPropagates targets the artifacts-side branch of
// buildStructuredRecords in isolation from the sources-side (already covered by
// TestCompute_SourceRecordsBuildErrorPropagates): a `when:` mentioning bare `artifacts` only
// must still surface a hasher failure while building the per-file artifact records.
func TestCompute_ArtifactRecordsBuildErrorPropagates(t *testing.T) {
	tmpDir := t.TempDir()
	artifactFile := filepath.Join(tmpDir, "app")
	require.NoError(t, os.WriteFile(artifactFile, []byte("binary"), 0o644))

	checker := NewChecker(
		WithGlobber(fakeGlobber{matches: map[string][]string{"app": {artifactFile}}}),
		WithHasher(func([]string) (string, error) { return "", errors.New("hash failed") }),
	)
	artifacts := &schema.Artifacts{Paths: []string{"app"}}

	_, err := computeFor(checker, artifactsRecordsCondition(t), nil, artifacts, nil, tmpDir, tmpDir, "cmd:x", "step")
	require.Error(t, err)
}

// erroringStateStore always fails Load, exercising checksumChanged's store-error propagation
// path distinctly from a hasher failure (already covered by TestChecksumChanged_HasherErrorPropagates).
type erroringStateStore struct{}

func (erroringStateStore) Load(_, _ string) (Record, bool, error) {
	return Record{}, false, errors.New("state load failed")
}

func (erroringStateStore) Save(_, _ string, _ Record) error {
	return nil
}

func TestChecksumChanged_StateStoreLoadErrorPropagates(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(srcFile, []byte("package main"), 0o644))
	artifactFile := filepath.Join(tmpDir, "app")
	require.NoError(t, os.WriteFile(artifactFile, []byte("binary"), 0o644))

	checker := NewChecker(
		WithGlobber(fakeGlobber{matches: map[string][]string{"*.go": {srcFile}, "app": {artifactFile}}}),
		WithStateStore(erroringStateStore{}),
	)
	inputs := &schema.Inputs{Sources: []string{"*.go"}}
	artifacts := &schema.Artifacts{Paths: []string{"app"}}

	_, err := computeFor(checker, checksumChangedCondition(t), inputs, artifacts, nil, tmpDir, tmpDir, "cmd:build", "compile")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "state load failed")
}

func TestDefaultGlobber_InvalidPatternPropagatesUnderlyingError(t *testing.T) {
	// A malformed glob pattern (unmatched bracket) is a real doublestar error, distinct from the
	// "base directory doesn't exist yet" case that Glob deliberately treats as "no matches."
	tmpDir := t.TempDir()
	g := NewGlobber()
	_, err := g.Glob(tmpDir, "[")
	require.Error(t, err)
	assert.False(t, errors.Is(err, errUtils.ErrFailedToFindImport),
		"a malformed pattern must propagate as a real error, not the missing-base-dir sentinel")
}

func TestFileStateStore_LoadMissingKeyInExistingStateDirIsAMissNotAnError(t *testing.T) {
	// Distinct from TestFileStateStore_LoadOnNonexistentStateDirIsAMissNotAnError: here stateDir
	// itself already exists (e.g. another step already recorded state there), but no record has
	// ever been saved for THIS key -- exercising os.ReadFile's os.IsNotExist(readErr) branch
	// rather than the earlier os.Stat(stateDir) short-circuit.
	store := NewStateStore()
	stateDir := t.TempDir()

	record, found, err := store.Load(stateDir, "never-saved-key")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, Record{}, record)
}

func TestFileStateStore_LoadNonNotExistReadErrorPropagates(t *testing.T) {
	// Replace the record file with a directory of the same name: os.ReadFile on a directory
	// fails with a real (non-ENOENT) error on every supported platform, exercising Load's
	// readErr-that-isn't-IsNotExist branch, distinct from both the corrupted-JSON case (a cache
	// miss) and the missing-file case (also a cache miss).
	stateDir := t.TempDir()
	key := "a-key"
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, key+".json"), 0o755))

	store := NewStateStore()
	_, _, err := store.Load(stateDir, key)
	require.Error(t, err)
}

func TestFileStateStore_SaveMkdirAllErrorPropagates(t *testing.T) {
	// stateDir's parent path component is a regular file, not a directory -- os.MkdirAll must
	// fail, and Save must propagate that failure rather than silently swallowing it.
	tmpDir := t.TempDir()
	blocker := filepath.Join(tmpDir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	stateDir := filepath.Join(blocker, "state")

	store := NewStateStore()
	err := store.Save(stateDir, "key", Record{SourcesHash: "abc"})
	require.Error(t, err)
}

func TestFileStateStore_SaveRenameErrorPropagates(t *testing.T) {
	// Pre-create the final record path as a non-empty directory: the closing os.Rename(tmp,
	// path) must fail (a file can never atomically replace a non-empty directory), and Save must
	// propagate that failure instead of silently reporting success while leaving a stale temp
	// file behind.
	stateDir := t.TempDir()
	key := "a-key"
	recordAsDir := filepath.Join(stateDir, key+".json")
	require.NoError(t, os.MkdirAll(recordAsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(recordAsDir, "inner"), []byte("x"), 0o644))

	store := NewStateStore()
	err := store.Save(stateDir, key, Record{SourcesHash: "abc"})
	require.Error(t, err)
}
