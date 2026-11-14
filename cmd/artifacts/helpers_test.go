package artifacts

import (
	"bytes"
	"context"
	stdio "io"
	"regexp"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Compile-time guards for the schema fields these tests configure.
var _ = schema.ArtifactRepositoryConfig{Kind: "local/dir"}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// streams is a minimal io.Streams backed by buffers, so a test can tell stdout (data) from stderr (UI).
type streams struct {
	stdout, stderr *bytes.Buffer
}

func (s *streams) Input() stdio.Reader     { return &bytes.Buffer{} }
func (s *streams) Output() stdio.Writer    { return s.stdout }
func (s *streams) Error() stdio.Writer     { return s.stderr }
func (s *streams) RawOutput() stdio.Writer { return s.stdout }
func (s *streams) RawError() stdio.Writer  { return s.stderr }

// invocation is the captured outcome of one command run.
type invocation struct {
	stdout string
	stderr string
	err    error
}

// harness runs fresh command trees against an injected service.
type harness struct {
	t       *testing.T
	service artifact.Service
	config  *schema.AtmosConfiguration
	options []ServiceOptions
}

// newHarness injects a mock service, a configuration with two repositories, and buffered streams.
func newHarness(t *testing.T) (*harness, *artifact.MockService) {
	t.Helper()
	for _, name := range []string{
		envRepository, envName, envVersion, envKind, envForce, envDryRun, envLabels, envReadme,
		envChangelog, envOutput, envFrom, envTo, envRefresh, envFormat, envYes,
	} {
		t.Setenv(name, "")
	}
	mock := artifact.NewMockService(gomock.NewController(t))
	h := &harness{t: t, service: mock, config: &schema.AtmosConfiguration{Artifacts: schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		"dir":     {Kind: "local/dir"},
		"archive": {Kind: "local/archive"},
	}}}}

	previousConfig, previousFactory := atmosConfigPtr, newService
	atmosConfigPtr = h.config
	newService = func(_ *schema.AtmosConfiguration, options ServiceOptions) (artifact.Service, error) {
		h.options = append(h.options, options)
		return h.service, nil
	}
	t.Cleanup(func() { atmosConfigPtr, newService = previousConfig, previousFactory })
	return h, mock
}

// run executes `atmos artifacts <args...>` on a fresh command tree.
func (h *harness) run(args ...string) invocation {
	h.t.Helper()
	out := &streams{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(out))
	require.NoError(h.t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
	defer data.Reset()

	command := NewCommand()
	command.SilenceUsage, command.SilenceErrors = true, true
	command.SetOut(stdio.Discard)
	command.SetErr(stdio.Discard)
	command.SetArgs(args)
	runErr := command.ExecuteContext(context.Background())
	return invocation{stdout: out.stdout.String(), stderr: ansiPattern.ReplaceAllString(out.stderr.String(), ""), err: runErr}
}

const (
	recordDigest   = digest.Digest("sha256:1111111111111111111111111111111111111111111111111111111111111111")
	manifestDigest = digest.Digest("sha256:2222222222222222222222222222222222222222222222222222222222222222")
	contentDigest  = digest.Digest("sha256:3333333333333333333333333333333333333333333333333333333333333333")
	archiveDigest  = digest.Digest("sha256:4444444444444444444444444444444444444444444444444444444444444444")
)

// operation is a successful operation result with every named output set.
func operation(action artifact.Action, outcome artifact.Outcome) *artifact.OperationResult {
	return &artifact.OperationResult{
		OperationID: "op-1", Action: action, Outcome: outcome, Repository: "dir", Requested: "team/app:1.0.0",
		Ref: "team/app:1.0.0", Location: "/repo/team/app/1.0.0", ResolvedDigest: manifestDigest, RecordDigest: recordDigest,
		ContentDigest: contentDigest, RepresentationDigest: archiveDigest, Size: 42, LockScope: artifact.LockScopeHost,
	}
}
