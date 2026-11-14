package report_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/artifact/report"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/schema"
)

// update regenerates the golden files under testdata. Run: go test ./pkg/artifact/report -update.
var update = flag.Bool("update", false, "regenerate the golden files")

// Compile-time sentinels for the schema fields these tests set.
var (
	_ = schema.CIConfig{Enabled: true}
	_ = schema.CISummaryConfig{Enabled: ptr(true)}
	_ = schema.CIOutputConfig{Enabled: ptr(true)}
	_ = schema.CITemplatesConfig{BasePath: "x", Artifacts: map[string]string{"summary": "x.md"}}
)

func ptr[T any](value T) *T { return &value }

// digestOf returns a deterministic sha256-shaped digest for a one-character seed.
func digestOf(seed string) digest.Digest {
	hex := ""
	for len(hex) < 64 {
		hex += seed
	}
	return digest.Digest("sha256:" + hex[:64])
}

// fakeReporter records what the session writes. It embeds the interface so only the methods the
// session uses are implemented; any other call panics, which fails the test loudly.
type fakeReporter struct {
	ci.Reporter

	mu         sync.Mutex
	summaries  []string
	outputs    []output
	summaryErr error
	outputErr  error
}

type output struct{ Key, Value string }

func (f *fakeReporter) Summary(markdown string) (ci.Receipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.summaries = append(f.summaries, markdown)
	return ci.Receipt{}, f.summaryErr
}

func (f *fakeReporter) Output(key, value string) (ci.Receipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outputs = append(f.outputs, output{key, value})
	return ci.Receipt{}, f.outputErr
}

// factory returns a reporter factory that counts how often the session builds a reporter.
func (f *fakeReporter) factory(builds *int) func(*schema.AtmosConfiguration) ci.Reporter {
	return func(*schema.AtmosConfiguration) ci.Reporter {
		if builds != nil {
			*builds++
		}
		return f
	}
}

// enabledConfig enables the CI master switch, which turns on summaries and outputs by default.
func enabledConfig() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}
}

// result wraps an operation result in an outcome.
func result(op *artifact.OperationResult, mutate ...func(*invoke.Result)) *invoke.Outcome {
	res := &invoke.Result{OperationResult: *op}
	for _, apply := range mutate {
		apply(res)
	}
	return &invoke.Outcome{Result: res}
}

// entry builds a recorded attempt.
func entry(step, id string, attempt int, outcome *invoke.Outcome, err error, position ...int) *report.Entry {
	return &report.Entry{Step: step, OperationID: id, Attempt: attempt, Position: position, Outcome: outcome, Err: err}
}

// published is a successful publication.
func published(id, ref string) *invoke.Outcome {
	return result(&artifact.OperationResult{
		OperationID: id, Action: artifact.ActionPublish, Outcome: artifact.OutcomeCreated, Repository: "shared",
		Requested: ref, Ref: ref, Location: "oci://ghcr.io/acme/" + ref,
		ResolvedDigest: digestOf("a"), RecordDigest: digestOf("b"), ContentDigest: digestOf("c"), RepresentationDigest: digestOf("d"),
		Size: 49152, Duration: 1200 * time.Millisecond, LockScope: artifact.LockScopeHost,
	}, func(res *invoke.Result) {
		res.Record = &artifact.Record{
			Name: "components/vpc", Version: "1.2.0", Kind: "files",
			Metadata:  artifact.Metadata{Description: "VPC component", Labels: map[string]string{"team": "net", "tier": "1"}},
			Inventory: []artifact.Entry{{Path: "main.tf", Type: artifact.EntryFile, Size: 1024}, {Path: "variables.tf", Type: artifact.EntryFile, Size: 2048}},
		}
	})
}

// subjectOf returns a record signature subject.
func subjectOf(seed string) artifact.SignatureSubject {
	return artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: digestOf(seed), MediaType: "application/vnd.atmos.artifact.record.v1+json", Size: 512}
}

// signed is a successful sign.
func signed(id string) *invoke.Outcome {
	subject := subjectOf("b")
	return result(&artifact.OperationResult{
		OperationID: id, Action: artifact.ActionSign, Outcome: artifact.OutcomeCreated, Repository: "shared",
		Requested: "components/vpc:1.2.0", Ref: "components/vpc:1.2.0", RecordDigest: digestOf("b"), LockScope: artifact.LockScopeRepository,
		Duration: 300 * time.Millisecond,
	}, func(res *invoke.Result) {
		res.Subject = &subject
		res.Evidence = []artifact.SignatureEvidence{{
			Subject: subject, Format: artifact.EvidenceSigstoreBundle,
			Descriptor: ocispec.Descriptor{Digest: digestOf("e"), Size: 100},
		}}
	})
}

// verified is a verify operation with the given status.
func verified(id string, status artifact.VerificationStatus) *invoke.Outcome {
	verification := &artifact.VerificationResult{Subject: subjectOf("b"), Status: status, PolicyFingerprint: "sha256:" + string(digestOf("f"))[7:]}
	if status == artifact.VerificationVerified {
		verification.Claims = []artifact.SignerClaim{
			{Identity: "https://github.com/acme/infra/.github/workflows/release.yaml@refs/heads/main", Issuer: "https://token.actions.githubusercontent.com"},
			{KeyID: "SHA256:abcdef"},
		}
	}
	return result(&artifact.OperationResult{
		OperationID: id, Action: artifact.ActionVerify, Outcome: artifact.OutcomeRead, Repository: "shared",
		Requested: "components/vpc:1.2.0", Ref: "components/vpc:1.2.0", RecordDigest: digestOf("b"),
	}, func(res *invoke.Result) {
		res.Verification = verification
		res.Subject = &verification.Subject
	})
}

// golden compares got with testdata/<name>.md, or rewrites it under -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".md")
	// Repository checks keep exactly one final newline in text files, so compare on that form.
	got = strings.TrimRight(got, "\n") + "\n"
	if *update {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden file; run: go test ./pkg/artifact/report -update")
	require.Equal(t, string(want), got)
}

// flushOne records the entries into a session with the fake reporter, flushes, and returns the one summary.
func flushOne(t *testing.T, config *schema.AtmosConfiguration, entries ...*report.Entry) string {
	t.Helper()
	fake := &fakeReporter{}
	session := report.NewSession(config, report.WithReporter(fake.factory(nil)))
	for _, item := range entries {
		session.Record(item)
	}
	require.NoError(t, session.Flush())
	require.Len(t, fake.summaries, 1, "exactly one summary per flush")
	return fake.summaries[0]
}

// newSession builds a session that reports through the fake reporter.
func newSession(config *schema.AtmosConfiguration, fake *fakeReporter, options ...report.Option) *report.Session {
	return report.NewSession(config, append([]report.Option{report.WithReporter(fake.factory(nil))}, options...)...)
}
