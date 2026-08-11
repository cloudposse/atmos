package proexec

import (
	"encoding/json"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	git "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/metrics/process"
	"github.com/cloudposse/atmos/pkg/schema"
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

func testMetrics() process.ProcessMetrics {
	return process.ProcessMetrics{
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

	atmosConfig := &schema.AtmosConfiguration{}

	req, err := buildRecord(atmosConfig, "atmos version", 0, testMetrics(), nil, repo)
	require.NoError(t, err)
	require.NotNil(t, req)

	assert.Equal(t, "atmos version", req.Command)
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
	assert.Nil(t, req.Data)
}

func TestBuildRecord_NilDataOmittedFromJSON(t *testing.T) {
	repo := &fakeGitRepo{info: &git.RepoInfo{}}
	atmosConfig := &schema.AtmosConfiguration{}

	req, err := buildRecord(atmosConfig, "atmos list components", 0, testMetrics(), nil, repo)
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
	atmosConfig := &schema.AtmosConfiguration{}

	type sample struct {
		Foo string `json:"foo"`
	}

	req, err := buildRecord(atmosConfig, "atmos terraform plan", 0, testMetrics(), sample{Foo: "bar"}, repo)
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

func TestBuildRecord_SecretMaskingAppliedToData(t *testing.T) {
	repo := &fakeGitRepo{info: &git.RepoInfo{}}
	atmosConfig := &schema.AtmosConfiguration{}

	type sample struct {
		AWSKey string `json:"aws_key"`
	}

	// A recognizable AWS access key pattern the Gitleaks-based masker detects.
	req, err := buildRecord(atmosConfig, "atmos terraform plan", 0, testMetrics(),
		sample{AWSKey: "AKIAIOSFODNN7EXAMPLE"}, repo)
	require.NoError(t, err)
	require.NotNil(t, req.Data)
	// Masking is a no-op without an initialized masking context in this unit
	// test (pkg/io.GetContext() returns nil by default), so this test only
	// asserts the masking call path executes without corrupting the payload.
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(req.Data, &decoded))
	assert.Contains(t, decoded, "aws_key")
}

func TestBuildRecord_GitInfoErrorsAreNonFatal(t *testing.T) {
	repo := &fakeGitRepo{
		infoErr: assertError("no repo"),
		shaErr:  assertError("no sha"),
	}
	atmosConfig := &schema.AtmosConfiguration{}

	req, err := buildRecord(atmosConfig, "atmos version", 0, testMetrics(), nil, repo)
	require.NoError(t, err)
	assert.Equal(t, "", req.GitSHA)
	assert.Equal(t, "", req.RepoURL)
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

func assertError(msg string) error { return simpleError(msg) }
