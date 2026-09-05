package proexec

import (
	"encoding/json"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	git "github.com/cloudposse/atmos/pkg/git"
	io "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/metrics/process"
)

// fakeGitRepo is a minimal GitRepoInterface implementation for tests.
type fakeGitRepo struct {
	info    *git.RepoInfo
	infoErr error
	sha     string
	shaErr  error
}

func (f *fakeGitRepo) GetLocalRepoInfo() (*git.RepoInfo, error) {
	if f.infoErr != nil {
		return nil, f.infoErr
	}
	return f.info, nil
}

func (f *fakeGitRepo) GetRepoInfo(_ *gogit.Repository) (git.RepoInfo, error) {
	if f.info == nil {
		return git.RepoInfo{}, nil
	}
	return *f.info, nil
}

func (f *fakeGitRepo) GetCurrentCommitSHA() (string, error) {
	if f.shaErr != nil {
		return "", f.shaErr
	}
	return f.sha, nil
}

func testMetrics() *process.ProcessMetrics {
	return &process.ProcessMetrics{
		WallTime:      100 * time.Millisecond,
		UserCPUTime:   50 * time.Millisecond,
		SystemCPUTime: 10 * time.Millisecond,
	}
}

func TestBuildRecord_FieldPopulation(t *testing.T) {
	repo := &fakeGitRepo{
		info: &git.RepoInfo{
			RepoUrl:   "https://github.com/acme/infra",
			RepoName:  "infra",
			RepoOwner: "acme",
			RepoHost:  "github.com",
		},
		sha: "deadbeef",
	}

	req, err := buildRecord(&ExecRecordInput{Command: "terraform plan"}, testMetrics(), repo)
	require.NoError(t, err)
	require.NotNil(t, req)

	assert.Equal(t, "terraform plan", req.Command)
	assert.Equal(t, 0, req.ExitCode)
	assert.Equal(t, "deadbeef", req.GitSHA)
	assert.Equal(t, "https://github.com/acme/infra", req.RepoURL)
	assert.Equal(t, "infra", req.RepoName)
	assert.Equal(t, "acme", req.RepoOwner)
	assert.Equal(t, "github.com", req.RepoHost)
	assert.Equal(t, int64(100), req.Metrics.WallTimeMS)
	assert.Equal(t, int64(50), req.Metrics.UserCPUTimeMS)
	assert.Equal(t, int64(10), req.Metrics.SystemCPUTimeMS)
	assert.NotNil(t, req.Args)
	assert.Empty(t, req.Args)
	assert.NotNil(t, req.Flags)
	assert.Empty(t, req.Flags)
	assert.Nil(t, req.Data)
}

// TestBuildRecord_ExecutionIDIsFreshUUIDPerCall verifies FR-003c: every
// buildRecord call generates a fresh, valid UUID v4 ExecutionID, distinct
// from AtmosProRunID and distinct across separate calls.
func TestBuildRecord_ExecutionIDIsFreshUUIDPerCall(t *testing.T) {
	repo := &fakeGitRepo{info: &git.RepoInfo{}}

	req1, err := buildRecord(&ExecRecordInput{Command: "terraform plan"}, testMetrics(), repo)
	require.NoError(t, err)
	req2, err := buildRecord(&ExecRecordInput{Command: "terraform plan"}, testMetrics(), repo)
	require.NoError(t, err)

	require.NotEmpty(t, req1.ExecutionID)
	require.NotEmpty(t, req2.ExecutionID)

	parsed1, parseErr := uuid.Parse(req1.ExecutionID)
	require.NoError(t, parseErr, "ExecutionID must be a valid UUID")
	assert.Equal(t, uuid.Version(4), parsed1.Version(), "ExecutionID must be a UUID v4")

	assert.NotEqual(t, req1.ExecutionID, req2.ExecutionID, "ExecutionID must be fresh per call, never reused")
}

// TestBuildRecord_ArgsAndFlagsShape verifies FR-003b's Command/Args/Flags
// contract: Command has no "atmos" root, Args holds only the positional
// component, Flags holds only the CLI flags actually passed, and the two are
// never combined into one array — the regression test for the
// previously-always-empty Args bug (maskArgs(nil) in the pre-fix envelope.go).
func TestBuildRecord_ArgsAndFlagsShape(t *testing.T) {
	repo := &fakeGitRepo{info: &git.RepoInfo{}}

	req, err := buildRecord(&ExecRecordInput{
		Command: "terraform plan",
		Args:    []string{"cdn"},
		Flags:   []string{"-s", "plat-use2-dev", "--upload-status"},
	}, testMetrics(), repo)
	require.NoError(t, err)
	require.NotNil(t, req)

	assert.Equal(t, "terraform plan", req.Command)
	assert.Equal(t, []string{"cdn"}, req.Args)
	assert.Equal(t, []string{"-s", "plat-use2-dev", "--upload-status"}, req.Flags)
	assert.NotContains(t, req.Args, "-s", "positional Args must never contain flags")
	assert.NotContains(t, req.Flags, "cdn", "Flags must never contain the positional component")
}

func TestBuildRecord_NilDataOmittedFromJSON(t *testing.T) {
	repo := &fakeGitRepo{info: &git.RepoInfo{}}

	req, err := buildRecord(&ExecRecordInput{Command: "atmos list components"}, testMetrics(), repo)
	require.NoError(t, err)

	b, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(b, &decoded))

	_, hasData := decoded["data"]
	assert.False(t, hasData, "Data must be entirely absent from the marshaled JSON when nil")
}

func TestBuildRecord_DataPresentWhenGiven(t *testing.T) {
	repo := &fakeGitRepo{info: &git.RepoInfo{}}

	type sample struct {
		Foo string `json:"foo"`
	}

	req, err := buildRecord(&ExecRecordInput{Command: "atmos terraform plan", Data: sample{Foo: "bar"}}, testMetrics(), repo)
	require.NoError(t, err)

	b, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(b, &decoded))

	dataVal, hasData := decoded["data"]
	require.True(t, hasData)
	dataMap, ok := dataVal.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "bar", dataMap["foo"])
}

// TestBuildRecord_DataAsArrayWhenGiven verifies the single Data field can
// carry a bulk, array-shaped payload (e.g. one entry per resource change) —
// the old dedicated DataItems field is retired (research.md Decision 16);
// bulk data is now just another Data shape, sized by pro.UploadExecMetadata.
func TestBuildRecord_DataAsArrayWhenGiven(t *testing.T) {
	repo := &fakeGitRepo{info: &git.RepoInfo{}}

	type resourceChange struct {
		Action  string `json:"action"`
		Address string `json:"address"`
	}

	items := []resourceChange{
		{Action: "created", Address: "aws_s3_bucket.example"},
		{Action: "updated", Address: "aws_iam_role.example"},
	}

	req, err := buildRecord(&ExecRecordInput{Command: "atmos terraform plan", Data: items}, testMetrics(), repo)
	require.NoError(t, err)
	require.NotNil(t, req.Data)

	var decoded []resourceChange
	require.NoError(t, json.Unmarshal(req.Data, &decoded))
	require.Len(t, decoded, 2)
	assert.Equal(t, "created", decoded[0].Action)
	assert.Equal(t, "aws_s3_bucket.example", decoded[0].Address)
}

func TestBuildRecord_SecretMaskingAppliedToData(t *testing.T) {
	repo := &fakeGitRepo{info: &git.RepoInfo{}}

	type sample struct {
		AWSKey string `json:"aws_key"`
	}

	// A recognizable AWS access key pattern the Gitleaks-based masker detects.
	// io.GetContext() lazily initializes the global masking context on first
	// use (io/global.go's Initialize, via sync.Once), so this pattern is
	// actually masked in-process, not merely passed through unmodified.
	req, err := buildRecord(&ExecRecordInput{
		Command: "atmos terraform plan",
		Data:    sample{AWSKey: "AKIAIOSFODNN7EXAMPLE"},
	}, testMetrics(), repo)
	require.NoError(t, err)
	require.NotNil(t, req.Data)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(req.Data, &decoded))
	assert.Contains(t, decoded, "aws_key")
	assert.Equal(t, io.MaskReplacement, decoded["aws_key"], "Gitleaks-pattern masking must replace the AWS-shaped key")
}

// TestBuildRecord_BothMaskingLayersRunIndependently verifies FR-010a
// (research.md Decisions 19/24): a Terraform-sensitive output already masked
// by cmd/terraform's maskSensitiveOutputs (layer 1, upstream of buildRecord)
// stays masked after the Gitleaks maskedDataJSON pass (layer 2) here; a
// separate, non-sensitive output containing a Gitleaks-pattern-matching
// literal is still caught by layer 2 even though layer 1 never touched it
// (buildRecord has no knowledge of per-output "sensitive" flags — it only
// ever sees the already-shaped Data map); and a `version` field (FR-005a)
// survives both passes unchanged, since it never matches any secret pattern.
func TestBuildRecord_BothMaskingLayersRunIndependently(t *testing.T) {
	repo := &fakeGitRepo{info: &git.RepoInfo{}}

	// Mirrors cmd/terraform's buildTerraformExecData output shape: outputs
	// already ran through maskSensitiveOutputs (layer 1) before this Data
	// value is ever built — "already_masked" simulates a Sensitive:true
	// output layer 1 already replaced; "leaked_aws_key" simulates a
	// non-sensitive output whose real value happens to match a Gitleaks
	// pattern, which layer 1 (Terraform-sensitive-flag-only) would not catch.
	data := map[string]any{
		"version": 1,
		"outputs": map[string]any{
			"already_masked": map[string]any{"value": io.MaskReplacement, "type": "string", "sensitive": true},
			"leaked_aws_key": map[string]any{"value": "AKIAIOSFODNN7EXAMPLE", "type": "string", "sensitive": false},
		},
	}

	req, err := buildRecord(&ExecRecordInput{
		Command: "atmos terraform apply",
		Data:    data,
	}, testMetrics(), repo)
	require.NoError(t, err)
	require.NotNil(t, req.Data)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(req.Data, &decoded))

	outputs, ok := decoded["outputs"].(map[string]any)
	require.True(t, ok)

	alreadyMasked, ok := outputs["already_masked"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, io.MaskReplacement, alreadyMasked["value"], "layer 1's masked placeholder must survive layer 2 unchanged")

	leaked, ok := outputs["leaked_aws_key"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, io.MaskReplacement, leaked["value"], "layer 2 (Gitleaks) must still catch a pattern-matching value layer 1 left untouched")

	assert.InDelta(t, float64(1), decoded["version"], 0, "version must survive both masking passes unchanged")
}

func TestBuildRecord_GitInfoErrorsAreNonFatal(t *testing.T) {
	repo := &fakeGitRepo{
		infoErr: assertError("no repo"),
		shaErr:  assertError("no sha"),
	}

	req, err := buildRecord(&ExecRecordInput{Command: "atmos version"}, testMetrics(), repo)
	require.NoError(t, err)
	assert.Equal(t, "", req.GitSHA)
	assert.Equal(t, "", req.RepoURL)
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

func assertError(msg string) error { return simpleError(msg) }

// TestVersionedData_WrapsPayloadWithVersion is the regression test for
// research.md Decision 24: every command-specific structured Data shape
// carries its own top-level `version` field (FR-005a), a plain integer
// starting at 1, independent per shape. VersionedData is the shared helper
// for the two genuinely identical single-key-wrap shapes (describe
// affected's {stacks}, list instances' {instances}) — TerraformExecData's
// multi-key shape deliberately does not use it (its own map literal gets
// "version" added directly instead).
func TestVersionedData_WrapsPayloadWithVersion(t *testing.T) {
	someSlice := []string{"a", "b"}

	got := VersionedData(1, "stacks", someSlice)

	assert.Equal(t, map[string]any{"version": 1, "stacks": someSlice}, got)
}

// TestVersionedData_NilPayloadStillWraps ensures a nil payload (e.g. list
// instances with a genuinely empty instance list) does not panic and still
// produces a well-formed, versioned wrapper rather than a bare nil.
func TestVersionedData_NilPayloadStillWraps(t *testing.T) {
	got := VersionedData(1, "instances", nil)

	assert.Equal(t, map[string]any{"version": 1, "instances": nil}, got)
}
