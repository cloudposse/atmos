package ui

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/generator/source"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/project/config"
)

const renderedProvenanceScaffoldYAML = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: rendered-provenance
spec:
  fields:
    - name: project_name
      type: input
      default: demo
`

func renderedProvenanceConfig(source string) *templates.Configuration {
	return &templates.Configuration{
		Name:   "rendered-provenance",
		Source: source,
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: renderedProvenanceScaffoldYAML, Permissions: 0o644},
			{Path: "file.txt", Content: "hello", Permissions: 0o644},
		},
	}
}

// TestExecuteWithSetup_RenderedStrategy_NonPinnableSourceRecordsMarker is a
// regression test for a data-integrity gap: --update-strategy=rendered
// generations from a source with no immutable ref to pin (local path,
// file://, s3::, plain http(s) archive -- see source.IsPinnableSource) left
// embedsConfig.ResolvedRef empty, and SaveProjectRecord silently drops an
// empty RenderedRef entirely. The resulting project record then looked
// indistinguishable from one that was never generated under rendered at
// all: a later `--update-strategy=rendered` update would wrongly fail with
// "no recorded rendered-strategy history" (ResolveRenderedBase), and a later
// `--update-strategy=tracked` run would wrongly skip
// CheckNotSwitchedFromRendered's strategy-switch guard. This proves the fix:
// a local-path source now records source.UnpinnedRenderedRefMarker instead
// of leaving RenderedRef empty.
func TestExecuteWithSetup_RenderedStrategy_NonPinnableSourceRecordsMarker(t *testing.T) {
	ui := createTestUI(t)
	ui.SetUpdateStrategy(engine.UpdateStrategyRendered)
	targetDir := t.TempDir()

	// A local filesystem path: not git, not oci -- Resolve never populates
	// ResolvedRef for this source kind. The path need not exist; it is only
	// ever stored as provenance text here.
	embedsConfig := renderedProvenanceConfig(filepath.Join(t.TempDir(), "local-template-source"))
	require.False(t, source.IsPinnableSource(embedsConfig.Source), "test setup: source must be non-pinnable")
	require.Empty(t, embedsConfig.ResolvedRef, "test setup: local sources never get a resolved ref")

	err := ui.executeWithSetup(embedsConfig, targetDir, false, false, true, "", nil, []string{"{{", "}}"})
	require.NoError(t, err)

	record, err := config.LoadProjectRecord(targetDir)
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, source.UnpinnedRenderedRefMarker, record.Spec.RenderedRef, "a non-pinnable source must still record a non-empty rendered marker")
	assert.Empty(t, record.Spec.BaseRef, "rendered strategy must never record a baseRef")
}

// TestExecuteWithSetup_RenderedStrategy_PinnableSourceRecordsResolvedRef
// proves the marker fallback does not clobber a real resolved ref: when
// embedsConfig.ResolvedRef is already populated (the normal case for a git::
// or oci:// source), that exact value -- not the marker -- must be recorded.
func TestExecuteWithSetup_RenderedStrategy_PinnableSourceRecordsResolvedRef(t *testing.T) {
	ui := createTestUI(t)
	ui.SetUpdateStrategy(engine.UpdateStrategyRendered)
	targetDir := t.TempDir()

	embedsConfig := renderedProvenanceConfig("git::https://example.com/acme/repo.git")
	embedsConfig.ResolvedRef = "abc123def456"

	err := ui.executeWithSetup(embedsConfig, targetDir, false, false, true, "", nil, []string{"{{", "}}"})
	require.NoError(t, err)

	record, err := config.LoadProjectRecord(targetDir)
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "abc123def456", record.Spec.RenderedRef, "a real resolved ref must win over the unpinned marker")
}

// TestExecuteWithSetup_RenderedStrategy_UnresolvedGitSourceRejectsUpFront is a
// regression test for a second data-integrity gap: generating under
// --update-strategy=rendered from a pinnable (git/oci) source whose ref
// failed to resolve used to succeed silently, leaving spec.renderedRef empty
// -- indistinguishable from a project never generated under rendered at all
// (see TestExecuteWithSetup_RenderedStrategy_NonPinnableSourceRecordsMarker's
// doc comment for the exact two failure modes that produces downstream).
// Unlike a genuinely non-pinnable source, this can't be fixed by recording
// UnpinnedRenderedRefMarker either: replaceRef would corrupt a git source's
// own ref= query parameter with the literal marker string on the next fetch
// (see source.ValidateRenderedSource's doc comment). The fix rejects the
// generation itself up front instead of producing an unreconstructable
// record.
func TestExecuteWithSetup_RenderedStrategy_UnresolvedGitSourceRejectsUpFront(t *testing.T) {
	ui := createTestUI(t)
	ui.SetUpdateStrategy(engine.UpdateStrategyRendered)
	targetDir := t.TempDir()

	embedsConfig := renderedProvenanceConfig("git::https://example.com/acme/repo.git")
	require.True(t, source.IsPinnableSource(embedsConfig.Source), "test setup: source must be pinnable")
	require.Empty(t, embedsConfig.ResolvedRef, "test setup: simulate a resolution failure")

	err := ui.executeWithSetup(embedsConfig, targetDir, false, false, true, "", nil, []string{"{{", "}}"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRenderedStrategyUnsupportedSource)

	record, err := config.LoadProjectRecord(targetDir)
	require.NoError(t, err)
	assert.Nil(t, record, "a rejected rendered generation must not write a project record")
}

// TestExecuteWithSetup_RenderedStrategy_EmbeddedSourceRejectsUpFront proves
// the second unreconstructable case from source.ValidateRenderedSource's doc
// comment: an embedded template (config.SourceEmbedded) has no location
// Hydrate can re-fetch by that literal source string, so a rendered update
// would always fail later regardless of what gets recorded now -- rejected
// up front instead.
func TestExecuteWithSetup_RenderedStrategy_EmbeddedSourceRejectsUpFront(t *testing.T) {
	ui := createTestUI(t)
	ui.SetUpdateStrategy(engine.UpdateStrategyRendered)
	targetDir := t.TempDir()

	embedsConfig := renderedProvenanceConfig(config.SourceEmbedded)

	err := ui.executeWithSetup(embedsConfig, targetDir, false, false, true, "", nil, []string{"{{", "}}"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRenderedStrategyUnsupportedSource)
}

// TestExecuteWithSetup_TrackedStrategy_UnresolvedGitSourceIsNotValidated
// proves source.ValidateRenderedSource is only ever consulted under
// --update-strategy=rendered: the default tracked strategy has no use for a
// resolved/pinnable source at all, and must not reject a generation that
// rendered strategy would.
func TestExecuteWithSetup_TrackedStrategy_UnresolvedGitSourceIsNotValidated(t *testing.T) {
	ui := createTestUI(t)
	// UpdateStrategyTracked is the zero value; no SetUpdateStrategy call needed.
	targetDir := t.TempDir()

	embedsConfig := renderedProvenanceConfig("git::https://example.com/acme/repo.git")

	err := ui.executeWithSetup(embedsConfig, targetDir, false, false, true, "", nil, []string{"{{", "}}"})
	require.NoError(t, err)
}
