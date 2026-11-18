// Package wiretest is test support for the entry points that run artifact operations: the CLI,
// workflow and custom-command steps, script step calls, and kind: step hooks.
//
// A [Fixture] builds a real service over a real local directory repository in a temporary
// directory, pins the clock and the operation IDs, and installs the service through the artifact
// step's service seam. Every entry point then reaches the same wiring, and a test can compare what
// each one asked for and received against a baseline run of the shared implementation.
package wiretest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/compose"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/perf"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Pinned values every fixture uses, so independent runs produce byte-identical records.
const (
	// OperationID is the operation ID every fixture service and artifact step generates.
	OperationID = "op-fixed"

	// Alias is the repository alias of the fixture's local directory repository.
	Alias = "dir"

	// Name and Version are the artifact the fixture source publishes.
	Name    = "team/app"
	Version = "1.0.0"
)

// File modes of the fixture source.
const (
	dirMode  = 0o755
	fileMode = 0o600
)

// PreparedAt is the pinned UTC preparation time.
var PreparedAt = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

// Fixture is a pinned artifact environment.
type Fixture struct {
	// Root is the repository root of the entry point under test.
	Root string

	// Source is a directory with the content to publish.
	Source string

	// Config is the Atmos configuration that declares the repository.
	Config *schema.AtmosConfiguration

	// Recorder records the publications that pass through the service seam.
	Recorder *artifacttest.Recorder

	baselineRoot string
}

// New creates the fixture and installs its service for artifact steps. The seams are restored when
// the test ends.
func New(t *testing.T) *Fixture {
	defer perf.Track(nil, "wiretest.New")()

	t.Helper()
	base := t.TempDir()
	f := &Fixture{
		Root:         filepath.Join(base, "repository"),
		Source:       filepath.Join(base, "dist"),
		baselineRoot: filepath.Join(base, "baseline"),
	}
	require.NoError(t, os.MkdirAll(f.Source, dirMode))
	require.NoError(t, os.WriteFile(filepath.Join(f.Source, "app.txt"), []byte("hello"), fileMode))
	require.NoError(t, os.WriteFile(filepath.Join(f.Source, "README.md"), []byte("# App\n"), fileMode))

	f.Config = Config(f.Root)
	f.Recorder = artifacttest.NewRecorder(f.service(t, f.Config))
	restoreFactory := runnerstep.UseArtifactServiceFactory(func(*schema.AtmosConfiguration, invoke.ServiceOptions) (artifact.Service, error) {
		return f.Recorder, nil
	})
	t.Cleanup(restoreFactory)
	restoreIdentity := runnerstep.UseArtifactIdentity(func() string { return OperationID }, func() time.Time { return PreparedAt })
	t.Cleanup(restoreIdentity)
	return f
}

// Config returns a configuration with one local directory repository rooted at root.
func Config(root string) *schema.AtmosConfiguration {
	defer perf.Track(nil, "wiretest.Config")()

	return &schema.AtmosConfiguration{Artifacts: schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		Alias: {Kind: "local/dir", Options: map[string]any{"path": root}},
	}}}
}

// service builds a real service with the clock and the operation IDs pinned and no retry delay.
func (f *Fixture) service(t *testing.T, config *schema.AtmosConfiguration) artifact.Service {
	t.Helper()
	composition, err := compose.New(config, compose.WithServiceOptions(
		service.WithClock(func() time.Time { return PreparedAt }),
		service.WithIDGenerator(func() string { return OperationID }),
		service.WithSleep(func(context.Context, time.Duration) error { return nil }),
	))
	require.NoError(t, err)
	return composition.Service
}

// Params returns the parameters of the fixture publication. Every entry point is configured to
// publish exactly this.
func (f *Fixture) Params() *invoke.Params {
	defer perf.Track(nil, "wiretest.Fixture.Params")()

	return &invoke.Params{
		Action: artifact.ActionPublish, Repository: Alias, Source: f.Source, Name: Name, Version: Version,
		Labels: map[string]string{"team": "platform"},
	}
}

// Baseline runs the fixture publication through the shared implementation, which is what the CLI
// runs, in a sibling repository. The result is normalized with [Fixture.Normalize] as if it came
// from the fixture's own repository.
func (f *Fixture) Baseline(t *testing.T) *invoke.Result {
	defer perf.Track(nil, "wiretest.Fixture.Baseline")()

	t.Helper()
	config := Config(f.baselineRoot)
	params := f.Params()
	outcome, err := invoke.Run(context.Background(), f.service(t, config), params)
	require.NoError(t, err)
	result := *outcome.Result
	result.OperationResult = relocate(&result.OperationResult, f.baselineRoot)
	return &result
}

// Normalize rewrites the repository root of the fixture to a placeholder, so results from
// repositories rooted in different directories compare equal.
func (f *Fixture) Normalize(op *artifact.OperationResult) artifact.OperationResult {
	defer perf.Track(nil, "wiretest.Fixture.Normalize")()

	return relocate(op, f.Root)
}

func relocate(source *artifact.OperationResult, root string) artifact.OperationResult {
	op := *source
	op.Location = strings.ReplaceAll(op.Location, root, "<root>")
	if len(op.Aliases) > 0 {
		aliases := make([]string, 0, len(op.Aliases))
		for _, alias := range op.Aliases {
			aliases = append(aliases, strings.ReplaceAll(alias, root, "<root>"))
		}
		op.Aliases = aliases
	}
	return op
}

// StepFields returns the artifact step fields of the fixture publication, as authored in YAML.
func (f *Fixture) StepFields() map[string]any {
	defer perf.Track(nil, "wiretest.Fixture.StepFields")()

	return map[string]any{
		"action": "publish", "source": f.Source, "repository": Alias, "artifact_name": Name, "version": Version,
		"metadata": map[string]any{"labels": map[string]any{"team": "platform"}},
	}
}
