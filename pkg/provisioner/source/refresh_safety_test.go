package source

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"testing"

	cockroacherrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSourceRefreshOwnedFileBecomesDirectory(t *testing.T) {
	for _, nonempty := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty-directory-replaced", true: "runtime-directory-preserved"}[nonempty], func(t *testing.T) {
			src, target := t.TempDir(), filepath.Join(t.TempDir(), "target")
			refreshWrite(t, src, "a.tf", "# old")
			refreshWrite(t, src, "z.data", "original source file")
			spec := &schema.VendorComponentSource{Uri: src}
			require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil)))
			require.NoError(t, os.Remove(filepath.Join(target, "z.data")))
			require.NoError(t, os.Mkdir(filepath.Join(target, "z.data"), 0o755))
			if nonempty {
				refreshWrite(t, target, "z.data/runtime", "caller-owned artifact")
			}
			before, err := downloader.TreeSHA256(target)
			require.NoError(t, err)
			refreshWrite(t, src, "a.tf", "# new")
			refreshWrite(t, src, "z.data", "new source file")
			err = VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil))
			if nonempty {
				require.ErrorIs(t, err, errUtils.ErrSourceCopyFailed)
				require.ErrorContains(t, err, "nonempty directory")
				after, err := downloader.TreeSHA256(target)
				require.NoError(t, err)
				require.Equal(t, before, after, "an earlier source update and all receipts must roll back")
				return
			}
			require.NoError(t, err)
			contents, err := os.ReadFile(filepath.Join(target, "z.data"))
			require.NoError(t, err)
			require.Equal(t, "new source file", string(contents))
		})
	}
}

func TestSourceRefreshBackupFailureRestoresEarlierFiles(t *testing.T) {
	stage, target := t.TempDir(), t.TempDir()
	for _, name := range []string{"a.tf", "b.tf"} {
		refreshWrite(t, stage, name, "# new")
		refreshWrite(t, target, name, "# previous")
	}
	refreshWrite(t, target, "terraform.tfstate", `{"version":4,"lineage":"preserved","serial":3}`)
	before, err := downloader.TreeSHA256(target)
	require.NoError(t, err)
	failure := errors.New("cannot back up source file")
	rename := func(from, to string) error {
		if from == filepath.Join(target, "b.tf") {
			return failure
		}
		return os.Rename(from, to)
	}
	require.ErrorIs(t, publishSource(t.Context(), stage, target, nil, rename), failure)
	after, err := downloader.TreeSHA256(target)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestSourceRefreshRefusesDanglingOwnedArtifact(t *testing.T) {
	src, target := t.TempDir(), filepath.Join(t.TempDir(), "target")
	refreshWrite(t, src, "custom.data", "source")
	spec := &schema.VendorComponentSource{Uri: src}
	require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil)))
	path := filepath.Join(target, "custom.data")
	require.NoError(t, os.Remove(path))
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing-state"), path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	before, err := downloader.TreeSHA256(target)
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(src, "custom.data")))
	require.ErrorContains(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil)), "inspect runtime artifact")
	after, err := downloader.TreeSHA256(target)
	require.NoError(t, err)
	require.Equal(t, before, after, "failed state inspection must not remove the owned path")
}

func TestSourceExplicitWorkdirRefusesUnverifiableOwner(t *testing.T) {
	for _, receipt := range []string{"missing", "corrupt"} {
		t.Run(receipt, func(t *testing.T) {
			base, src := t.TempDir(), t.TempDir()
			refreshWrite(t, src, "main.tf", "# incoming")
			target := filepath.Join(base, "custom")
			refreshWrite(t, target, "terraform.tfstate", `{"version":4,"lineage":"existing","serial":8}`)
			if receipt == "corrupt" {
				refreshWrite(t, target, ".atmos/metadata.json", "invalid receipt")
			}
			section := map[string]any{
				"component": "module", "atmos_component": "instance", "atmos_stack": "dev", "source": src,
				"settings":  map[string]any{"working_directory": target},
				"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
			}
			stateBefore, err := os.ReadFile(filepath.Join(target, "terraform.tfstate"))
			require.NoError(t, err)
			err = AutoProvisionSource(t.Context(), &schema.AtmosConfiguration{BasePath: base}, "terraform", section, nil, provisioner.OutputWriters{})
			require.ErrorIs(t, err, errUtils.ErrWorkdirProvision)
			require.Contains(t, cockroacherrors.GetAllDetails(err), "Explicit source working directory has files but no verifiable instance owner")
			require.NoFileExists(t, filepath.Join(target, "main.tf"))
			stateAfter, err := os.ReadFile(filepath.Join(target, "terraform.tfstate"))
			require.NoError(t, err)
			require.Equal(t, stateBefore, stateAfter)
		})
	}
}

func TestSourceReceiptDirectoryConflictPreservesWorkdir(t *testing.T) {
	src, target := t.TempDir(), t.TempDir()
	refreshWrite(t, src, "main.tf", "# incoming")
	refreshWrite(t, src, ".atmos", "source cannot replace the receipt directory")
	refreshWrite(t, target, "main.tf", "# current")
	require.NoError(t, workdir.WriteMetadata(target, &workdir.WorkdirMetadata{Component: "instance", Stack: "dev"}))
	before, err := downloader.TreeSHA256(target)
	require.NoError(t, err)
	spec := &schema.VendorComponentSource{Uri: src}
	receipt := &downloader.ResolvedArtifact{}
	err = VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil), withProvenance(&Target{Dir: target, IsWorkdir: true, Instance: "instance"}, "dev", spec, receipt))
	var pathErr *os.PathError
	require.ErrorAs(t, err, &pathErr, "receipt staging must fail on the source file occupying .atmos")
	require.Equal(t, ".atmos", filepath.Base(pathErr.Path))
	after, err := downloader.TreeSHA256(target)
	require.NoError(t, err)
	require.Equal(t, before, after, "failed receipt staging must not publish source")
}

func TestSourceRefreshRefusesReceiptPathConflicts(t *testing.T) {
	for _, scenario := range []string{"inventory-is-directory", "stage-metadata-is-file", "metadata-symlink"} {
		t.Run(scenario, func(t *testing.T) {
			src, target := t.TempDir(), t.TempDir()
			refreshWrite(t, src, "main.tf", "# incoming")
			refreshWrite(t, target, "main.tf", "# current")
			refreshWrite(t, target, "terraform.tfstate", `{"version":4,"lineage":"preserved","serial":5}`)
			var external string
			switch scenario {
			case "inventory-is-directory":
				require.NoError(t, os.MkdirAll(filepath.Join(target, sourceFilesManifest), 0o755))
			case "stage-metadata-is-file":
				refreshWrite(t, src, ".atmos", "invalid receipt directory")
			case "metadata-symlink":
				external = t.TempDir()
				refreshWrite(t, external, "preserved", "external metadata")
				if err := os.Symlink(external, filepath.Join(target, ".atmos")); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			before, err := downloader.TreeSHA256(target)
			require.NoError(t, err)
			err = VendorSource(t.Context(), nil, &schema.VendorComponentSource{Uri: src}, target, WithRuntimePreservation(nil))
			if scenario == "metadata-symlink" {
				require.ErrorIs(t, err, errUtils.ErrSourceCopyFailed)
			} else {
				var pathErr *os.PathError
				require.ErrorAs(t, err, &pathErr)
				if scenario == "inventory-is-directory" {
					require.Equal(t, filepath.Join(target, sourceFilesManifest), pathErr.Path)
				} else {
					require.Equal(t, ".atmos", filepath.Base(pathErr.Path))
				}
			}
			after, err := downloader.TreeSHA256(target)
			require.NoError(t, err)
			require.Equal(t, before, after, "receipt path failure must preserve source and state")
			if external != "" {
				entries, err := os.ReadDir(external)
				require.NoError(t, err)
				require.Len(t, entries, 1, "no receipt may be written outside target")
				contents, err := os.ReadFile(filepath.Join(external, "preserved"))
				require.NoError(t, err)
				require.Equal(t, "external metadata", string(contents))
			}
		})
	}
}

func TestSourceRefreshUpdatesOrdinaryZipSource(t *testing.T) {
	src, target := t.TempDir(), filepath.Join(t.TempDir(), "target")
	for _, contents := range []string{"initial configuration", "updated configuration"} {
		file, err := os.Create(filepath.Join(src, "config.zip"))
		require.NoError(t, err)
		archive := zip.NewWriter(file)
		entry, err := archive.Create("configuration.json")
		require.NoError(t, err)
		_, err = entry.Write([]byte(contents))
		require.NoError(t, err)
		require.NoError(t, archive.Close())
		require.NoError(t, file.Close())
		require.NoError(t, VendorSource(t.Context(), nil, &schema.VendorComponentSource{Uri: src}, target, WithRuntimePreservation(nil)), "a ZIP without tfplan is source content, not a saved Terraform plan")
		want, err := os.ReadFile(filepath.Join(src, "config.zip"))
		require.NoError(t, err)
		got, err := os.ReadFile(filepath.Join(target, "config.zip"))
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}
