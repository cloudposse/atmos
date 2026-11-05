package scaffold

import (
	"bytes"
	"context"
	stdio "io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	artifactscmd "github.com/cloudposse/atmos/cmd/artifacts"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Compile-time guard for the schema fields these tests configure.
var _ = schema.ArtifactRepositoryConfig{Kind: "local/dir"}

// bufferedStreams is a minimal io.Streams backed by buffers.
type bufferedStreams struct {
	stdout, stderr *bytes.Buffer
}

func (s *bufferedStreams) Input() stdio.Reader     { return &bytes.Buffer{} }
func (s *bufferedStreams) Output() stdio.Writer    { return s.stdout }
func (s *bufferedStreams) Error() stdio.Writer     { return s.stderr }
func (s *bufferedStreams) RawOutput() stdio.Writer { return s.stdout }
func (s *bufferedStreams) RawError() stdio.Writer  { return s.stderr }

// publishHarness runs a fresh scaffold publish command against an injected service.
type publishHarness struct {
	t      *testing.T
	stdout *bytes.Buffer
}

// newPublishHarness injects the service and a configuration with one repository, clears the
// environment variables the command reads, and captures the output channels.
func newPublishHarness(t *testing.T, service artifact.Service) *publishHarness {
	t.Helper()
	for _, name := range []string{
		"ATMOS_ARTIFACTS_REPOSITORY", "ATMOS_ARTIFACTS_NAME", "ATMOS_ARTIFACTS_VERSION", "ATMOS_ARTIFACTS_KIND",
		"ATMOS_ARTIFACTS_FORCE", "ATMOS_ARTIFACTS_DRY_RUN", "ATMOS_ARTIFACTS_LABELS", "ATMOS_ARTIFACTS_README",
		"ATMOS_ARTIFACTS_CHANGELOG", "ATMOS_ARTIFACTS_FORMAT",
	} {
		t.Setenv(name, "")
	}
	artifactscmd.SetAtmosConfig(&schema.AtmosConfiguration{Artifacts: schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		"shared": {Kind: "local/dir"},
	}}})
	t.Cleanup(func() { artifactscmd.SetAtmosConfig(nil) })
	restore := artifactscmd.UseServiceFactory(func(*schema.AtmosConfiguration, artifactscmd.ServiceOptions) (artifact.Service, error) {
		return service, nil
	})
	t.Cleanup(restore)

	out := &bufferedStreams{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(out))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
	t.Cleanup(data.Reset)
	return &publishHarness{t: t, stdout: out.stdout}
}

func (h *publishHarness) run(args ...string) error {
	h.t.Helper()
	command := artifactscmd.NewKindPublishCommand(&scaffoldPublishSpec)
	command.SilenceUsage, command.SilenceErrors = true, true
	command.SetOut(stdio.Discard)
	command.SetErr(stdio.Discard)
	command.SetArgs(args)
	return command.ExecuteContext(context.Background())
}

func TestScaffoldPublish_IsRegisteredUnderScaffold(t *testing.T) {
	found, _, err := scaffoldCmd.Find([]string{"publish"})
	require.NoError(t, err)
	assert.Same(t, scaffoldPublishCmd, found)
	assert.Equal(t, "scaffold", scaffoldPublishSpec.Kind, "the command supplies the scaffold kind")
	assert.NotEmpty(t, scaffoldPublishCmd.Example, "usage examples are embedded")
	assert.Nil(t, scaffoldPublishCmd.Flags().Lookup("kind"), "a convenience command supplies its kind and has no --kind flag")
	for _, name := range []string{"repository", "name", "version", "labels", "readme", "changelog", "force", "dry-run", "format"} {
		assert.NotNil(t, scaffoldPublishCmd.Flags().Lookup(name), "scaffold publish shares the --%s flag with artifacts publish", name)
	}
}

func TestScaffoldPublish_DelegatesToTheGenericPublisherWithTheScaffoldKind(t *testing.T) {
	service := artifact.NewMockService(gomock.NewController(t))
	var captured artifact.PublishRequest
	service.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, request artifact.PublishRequest) (*artifact.OperationResult, error) {
			captured = request
			return &artifact.OperationResult{Action: artifact.ActionPublish, Outcome: artifact.OutcomeCreated, Repository: request.Repository, Ref: "scaffolds/landing-zone:2.0.0"}, nil
		},
	)
	h := newPublishHarness(t, service)

	err := h.run("./scaffolds/landing-zone", "--repository=shared", "--name=scaffolds/landing-zone", "--version=2.0.0",
		"--labels=team=platform", "--readme=docs/PACKAGE.md", "--changelog=docs/CHANGES.md", "--force", "--format=json")

	require.NoError(t, err)
	assert.Equal(t, "scaffold", captured.Kind)
	assert.Equal(t, "shared", captured.Repository)
	assert.Equal(t, "./scaffolds/landing-zone", captured.Source)
	assert.Equal(t, "scaffolds/landing-zone", captured.Name)
	assert.Equal(t, "2.0.0", captured.Version)
	assert.Equal(t, map[string]string{"team": "platform"}, captured.Metadata.Labels)
	assert.Equal(t, "docs/PACKAGE.md", captured.Readme)
	assert.Equal(t, "docs/CHANGES.md", captured.Changelog)
	assert.True(t, captured.Force)
	assert.False(t, captured.DryRun)
	assert.Contains(t, h.stdout.String(), `"ref": "scaffolds/landing-zone:2.0.0"`, "structured output matches artifacts publish")
}

func TestScaffoldPublish_DryRunAndEnvironmentFlags(t *testing.T) {
	service := artifact.NewMockService(gomock.NewController(t))
	var captured artifact.PublishRequest
	service.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, request artifact.PublishRequest) (*artifact.OperationResult, error) {
			captured = request
			return &artifact.OperationResult{Action: artifact.ActionPublish, Outcome: artifact.OutcomeDryRun, Repository: request.Repository}, nil
		},
	)
	h := newPublishHarness(t, service)
	t.Setenv("ATMOS_ARTIFACTS_REPOSITORY", "shared")
	t.Setenv("ATMOS_ARTIFACTS_VERSION", "3.1.0")

	require.NoError(t, h.run("./scaffolds/landing-zone", "--dry-run", "--format=json"))

	assert.True(t, captured.DryRun)
	assert.Equal(t, "shared", captured.Repository, "the shared ATMOS_ARTIFACTS_REPOSITORY variable selects the repository")
	assert.Equal(t, "3.1.0", captured.Version)
	assert.Equal(t, "scaffold", captured.Kind)
	assert.Empty(t, captured.Name, "the name is left to the scaffold manifest")
}

func TestScaffoldPublish_KindEnvironmentVariableCannotOverrideTheKind(t *testing.T) {
	service := artifact.NewMockService(gomock.NewController(t))
	var captured artifact.PublishRequest
	service.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, request artifact.PublishRequest) (*artifact.OperationResult, error) {
			captured = request
			return &artifact.OperationResult{Action: artifact.ActionPublish, Outcome: artifact.OutcomeCreated}, nil
		},
	)
	h := newPublishHarness(t, service)
	t.Setenv("ATMOS_ARTIFACTS_KIND", "files")

	require.NoError(t, h.run("./s", "--repository=shared", "--version=1.0.0", "--format=json"))

	assert.Equal(t, "scaffold", captured.Kind)
}

func TestScaffoldPublish_RequiresARepositoryAndASource(t *testing.T) {
	h := newPublishHarness(t, artifact.NewMockService(gomock.NewController(t)))

	err := h.run("./scaffolds/landing-zone")
	require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided, "no command picks a default repository")

	err = h.run("--repository=shared")
	require.ErrorIs(t, err, errUtils.ErrInvalidPositionalArgs)
}
