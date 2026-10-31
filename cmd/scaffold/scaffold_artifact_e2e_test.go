package scaffold

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/roundtriptest"
	"github.com/cloudposse/atmos/pkg/oci/ocitest"
	"github.com/cloudposse/atmos/pkg/project/config"
)

const (
	publishedScaffoldName    = "landing-zone"
	publishedScaffoldVersion = "2.0.0"

	publishedScaffoldManifest = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: landing-zone
  description: A landing zone
  version: 2.0.0
spec:
  fields:
    - name: project_name
      type: input
      default: demo
`

	// The root README is ordinary scaffold content, rendered like any other template file.
	publishedScaffoldReadme = "<!-- atmos:template -->\n# {{ .Config.project_name }}\n\nGenerated project.\n"

	packageReadme    = "# Landing zone package\n\nDistribution notes.\n"
	packageChangelog = "# Changelog\n\n- 2.0.0 first release\n"
)

// writePublishedScaffold writes a scaffold that has BOTH README roles: a root README that is part of
// what generation produces, and a package README and CHANGELOG that describe the distribution.
func writePublishedScaffold(t *testing.T, dir, updateContent string) {
	t.Helper()
	fixtures.WriteFile(t, dir, "scaffold.yaml", publishedScaffoldManifest, 0o644)
	fixtures.WriteFile(t, dir, "README.md", publishedScaffoldReadme, 0o644)
	fixtures.WriteFile(t, dir, "static.txt", "static content\n", 0o644)
	fixtures.WriteFile(t, dir, "update.txt", updateContent, 0o644)
	fixtures.WriteFile(t, dir, "nested/deep.txt", "deep content\n", 0o644)
	fixtures.WriteFile(t, dir, fixtures.ReservedDir+"/README.md", packageReadme, 0o644)
	fixtures.WriteFile(t, dir, fixtures.ReservedDir+"/CHANGELOG.md", packageChangelog, 0o644)
}

// generateScaffold drives the real `scaffold generate` command.
func generateScaffold(t *testing.T, src, target string, flags map[string]string) {
	t.Helper()
	cmd := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd)
	require.NoError(t, cmd.Flags().Set("interactive", "false"))
	for name, value := range flags {
		require.NoError(t, cmd.Flags().Set(name, value))
	}
	require.NoError(t, scaffoldGenerateCmd.RunE(cmd, []string{src, target}))
}

// treeSnapshot maps every file below root to its content, skipping the project record, which
// records where each generation came from and so differs by source on purpose.
func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(rel) == ".atmos/scaffold.yaml" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(content)
		return nil
	}))
	require.NotEmpty(t, files, "the generated tree must not be empty")
	return files
}

// TestScaffoldGenerate_PublishedScaffoldMatchesGeneratingFromTheDirectory publishes a scaffold with
// both README roles and generates from it. The output is identical to generating from the original
// directory and contains no distribution files.
func TestScaffoldGenerate_PublishedScaffoldMatchesGeneratingFromTheDirectory(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	reg := roundtriptest.New(t)
	src := t.TempDir()
	writePublishedScaffold(t, src, "v1 content\n")

	published := reg.Publish(t, &artifact.PublishRequest{Source: src, Kind: "scaffold"})
	require.Equal(t, artifact.OutcomeCreated, published.Outcome)
	assert.Equal(t, publishedScaffoldName+":"+publishedScaffoldVersion, published.Ref, "name and version default from the scaffold manifest")

	fromDirectory := filepath.Join(t.TempDir(), "from-directory")
	generateScaffold(t, src, fromDirectory, nil)
	fromArtifact := filepath.Join(t.TempDir(), "from-artifact")
	generateScaffold(t, reg.URI(publishedScaffoldName, publishedScaffoldVersion), fromArtifact, nil)

	want := treeSnapshot(t, fromDirectory)
	got := treeSnapshot(t, fromArtifact)
	assert.Equal(t, want, got, "generating from the published scaffold is identical to generating from the directory")
	assert.Equal(t, "# demo\n\nGenerated project.\n", got["README.md"], "the root README keeps its rendering behavior")
	for _, tree := range []string{fromDirectory, fromArtifact} {
		assert.NoDirExists(t, filepath.Join(tree, ".atmos", "artifact"), "no distribution files leak into generated output")
		assert.NoFileExists(t, filepath.Join(tree, "manifest.json"))
	}

	record, err := config.LoadProjectRecord(fromArtifact)
	require.NoError(t, err)
	require.NotNil(t, record, "the generation still writes its project record")
}

// TestScaffoldGenerate_PublishedScaffoldUpdateFlow runs the existing rendered-strategy update flow
// against published scaffold versions.
func TestScaffoldGenerate_PublishedScaffoldUpdateFlow(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	reg := roundtriptest.New(t)
	v1 := t.TempDir()
	writePublishedScaffold(t, v1, "v1 content\n")
	reg.Publish(t, &artifact.PublishRequest{Source: v1, Kind: "scaffold"})

	v2 := t.TempDir()
	writePublishedScaffold(t, v2, "v2 content\n")
	reg.Publish(t, &artifact.PublishRequest{Source: v2, Kind: "scaffold", Version: "2.1.0"})

	target := filepath.Join(t.TempDir(), "project")
	generateScaffold(t, reg.URI(publishedScaffoldName, publishedScaffoldVersion), target, map[string]string{"update-strategy": "rendered"})
	require.Equal(t, "v1 content\n", readRenderedFile(t, filepath.Join(target, "update.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(target, "static.txt"), []byte("hand-edited content\n"), 0o644))

	generateScaffold(t, reg.URI(publishedScaffoldName, "2.1.0"), target, map[string]string{"update": "true", "update-strategy": "rendered"})

	assert.Equal(t, "v2 content\n", readRenderedFile(t, filepath.Join(target, "update.txt")), "the template's change is applied")
	assert.Equal(t, "hand-edited content\n", readRenderedFile(t, filepath.Join(target, "static.txt")), "the hand edit survives the update")
	assert.NoDirExists(t, filepath.Join(target, ".atmos", "artifact"), "an update never writes distribution files")
}

// TestScaffoldGenerate_LegacyOCIScaffoldStillWorks keeps the pre-existing single-layer scaffold
// image format working next to the Atmos package format.
func TestScaffoldGenerate_LegacyOCIScaffoldStillWorks(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	image := ocitest.NewRegistry(t, "legacy/scaffold:v1", map[string]string{
		"scaffold.yaml": publishedScaffoldManifest,
		"README.md":     publishedScaffoldReadme,
		"static.txt":    "static content\n",
	})
	target := filepath.Join(t.TempDir(), "legacy")

	generateScaffold(t, "oci://"+image, target, nil)

	files := treeSnapshot(t, target)
	assert.Equal(t, "static content\n", files["static.txt"])
	assert.Equal(t, "# demo\n\nGenerated project.\n", files["README.md"])
}

// TestScaffoldGenerate_LocalSourceNeverGeneratesDistributionMetadata covers the local-directory and
// verbatim-copy source strategies: the author's .atmos/artifact files never reach generated output.
func TestScaffoldGenerate_LocalSourceNeverGeneratesDistributionMetadata(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	src := t.TempDir()
	writePublishedScaffold(t, src, "v1 content\n")
	fixtures.WriteFile(t, src, ".atmos/other.txt", "kept\n", 0o644)
	target := filepath.Join(t.TempDir(), "project")

	generateScaffold(t, src, target, nil)

	assert.NoDirExists(t, filepath.Join(target, ".atmos", "artifact"))
	assert.FileExists(t, filepath.Join(target, ".atmos", "other.txt"), "other files under .atmos are ordinary scaffold content")
}
