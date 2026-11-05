package produce_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/produce"
)

// Compile-time guard for the producer metadata fields these tests rely on.
var _ = produce.ProducerMetadata{Name: "", Version: "", Description: "", Labels: nil, KindData: nil}

const lockFile = ".terraform.lock.hcl"

// writeComponent writes a Terraform component with an executable helper, a dependency lock file,
// documentation, and every kind of transient Terraform data.
func writeComponent(t *testing.T, dir string) {
	t.Helper()
	fixtures.WriteFile(t, dir, "main.tf", "resource \"null_resource\" \"x\" {}\n", 0o644)
	fixtures.WriteFile(t, dir, lockFile, "# lock\n", 0o644)
	fixtures.WriteFile(t, dir, "README.md", fixtures.RootReadme, 0o644)
	fixtures.WriteFile(t, dir, "scripts/helper.sh", fixtures.HelperScript, 0o755)
	fixtures.WriteFile(t, dir, "modules/inner/main.tf", "# inner\n", 0o644)
	fixtures.WriteFile(t, dir, fixtures.ReservedDir+"/README.md", fixtures.PackageReadme, 0o644)
	fixtures.WriteFile(t, dir, fixtures.ReservedDir+"/CHANGELOG.md", fixtures.PackageChangelog, 0o644)

	fixtures.WriteFile(t, dir, ".terraform/providers/registry/provider.bin", "binary\n", 0o755)
	fixtures.WriteFile(t, dir, "modules/inner/.terraform/modules.json", "{}\n", 0o644)
	fixtures.WriteFile(t, dir, "terraform.tfstate", "{}\n", 0o644)
	fixtures.WriteFile(t, dir, "terraform.tfstate.backup", "{}\n", 0o644)
	fixtures.WriteFile(t, dir, "terraform.tfstate.d/dev/terraform.tfstate", "{}\n", 0o644)
	fixtures.WriteFile(t, dir, "deploy.tfplan", "plan\n", 0o644)
	fixtures.WriteFile(t, dir, "deploy.tfplan.json", "{}\n", 0o644)
}

func prepareKind(t *testing.T, producer *produce.Producer, req *artifact.PrepareRequest) *artifact.PreparedArtifact {
	t.Helper()
	prepared, err := producer.Prepare(context.Background(), *req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = prepared.Close() })
	return prepared
}

func TestTerraformComponent_ExcludesTransientDataAndKeepsLockFileAndScripts(t *testing.T) {
	src := t.TempDir()
	writeComponent(t, src)
	req := request(src, artifact.PackagingTgz)
	req.Kind = produce.KindComponentTerraform

	prepared := prepareKind(t, produce.NewTerraformComponent(produce.WithStagingRoot(t.TempDir())), req)
	record := prepared.Record

	assert.Equal(t, "component/terraform", record.Kind)
	assert.Equal(t, []string{
		lockFile, "README.md", "main.tf", "modules", "modules/inner", "modules/inner/main.tf", "scripts", "scripts/helper.sh",
	}, inventoryPaths(&record), "transient data is excluded; the lock file, scripts, and docs stay")

	wantExclusions := []artifact.Exclusion{
		{Path: ".terraform", Rule: ".terraform/**"},
		{Path: "deploy.tfplan", Rule: "*.tfplan"},
		{Path: "deploy.tfplan.json", Rule: "*.tfplan.*"},
		{Path: "modules/inner/.terraform", Rule: ".terraform/**"},
		{Path: "terraform.tfstate", Rule: "*.tfstate"},
		{Path: "terraform.tfstate.backup", Rule: "*.tfstate.*"},
		{Path: "terraform.tfstate.d", Rule: "*.tfstate.*"},
	}
	assert.Equal(t, wantExclusions, prepared.Exclusions, "each exclusion is reported once, with the rule that excluded it")

	require.Len(t, record.Documents, 2, "package README and CHANGELOG stay distinct from the root README payload")
	var data struct {
		ComponentType string `json:"component_type"`
	}
	require.NoError(t, record.KindData.Decode(&data))
	assert.Equal(t, "terraform", data.ComponentType, "the component subtype is recorded in portable metadata")

	if fixtures.POSIXModes() {
		for _, entry := range record.Inventory {
			if entry.Path == "scripts/helper.sh" {
				assert.Equal(t, uint32(0o755), entry.Mode, "scripts keep their executable bit")
			}
		}
	}
}

func TestFilesProducer_DoesNotInheritComponentExclusions(t *testing.T) {
	src := t.TempDir()
	writeComponent(t, src)

	prepared, _ := prepare(t, request(src, artifact.PackagingNone))

	paths := inventoryPaths(&prepared.Record)
	for _, want := range []string{".terraform", ".terraform/providers/registry/provider.bin", "terraform.tfstate", "terraform.tfstate.backup", "deploy.tfplan", "deploy.tfplan.json", "terraform.tfstate.d/dev/terraform.tfstate"} {
		assert.Contains(t, paths, want, "generic files and backups keep every requested file")
	}
	assert.Empty(t, prepared.Exclusions)
}

func TestTerraformComponent_RuleMatrix(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		excluded bool
	}{
		{name: "working directory file", path: ".terraform/terraform.tfstate.d/x", excluded: true},
		{name: "lock file is kept", path: lockFile, excluded: false},
		{name: "tfstate", path: "a/b/prod.tfstate", excluded: true},
		{name: "tfstate backup", path: "terraform.tfstate.1700000000.backup", excluded: true},
		{name: "tfplan", path: "x.tfplan", excluded: true},
		{name: "tfplan json", path: "x.tfplan.json", excluded: true},
		{name: "tfvars is kept", path: "prod.tfvars", excluded: false},
		{name: "tf is kept", path: "state.tf", excluded: false},
		{name: "name that only contains the word", path: "tfstate.md", excluded: false},
		{name: "a file called .terraform is not a directory rule", path: "docs/.terraformrc", excluded: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := t.TempDir()
			fixtures.WriteFile(t, src, "main.tf", "# keep\n", 0o644)
			fixtures.WriteFile(t, src, tt.path, "content\n", 0o644)
			req := request(src, artifact.PackagingNone)
			req.Kind = produce.KindComponentTerraform

			prepared := prepareKind(t, produce.NewTerraformComponent(produce.WithStagingRoot(t.TempDir())), req)

			exclusionPaths := make([]string, 0, len(prepared.Exclusions))
			for _, exclusion := range prepared.Exclusions {
				exclusionPaths = append(exclusionPaths, exclusion.Path)
			}
			if tt.excluded {
				assert.NotEmpty(t, exclusionPaths)
				assert.NotContains(t, inventoryPaths(&prepared.Record), tt.path)
				return
			}
			assert.Empty(t, exclusionPaths)
			assert.Contains(t, inventoryPaths(&prepared.Record), filepath.ToSlash(tt.path))
		})
	}
}

func TestTerraformComponent_ExclusionsReturnsACopy(t *testing.T) {
	rules := produce.TerraformExclusions()
	require.Equal(t, []string{".terraform/**", "*.tfstate", "*.tfstate.*", "*.tfplan", "*.tfplan.*"}, rules)
	rules[0] = "mutated"
	assert.Equal(t, ".terraform/**", produce.TerraformExclusions()[0])
}

func TestTerraformComponent_IdentityPrecedence(t *testing.T) {
	src := filepath.Join(t.TempDir(), "vpc")
	require.NoError(t, os.MkdirAll(src, 0o755))
	fixtures.WriteFile(t, src, "main.tf", "# vpc\n", 0o644)
	// component.yaml names the vendoring manifest and its upstream version, so neither is reused.
	fixtures.WriteFile(t, src, "component.yaml", "kind: ComponentVendorConfig\nmetadata:\n  name: vpc-vendor-config\nspec:\n  source:\n    version: 9.9.9\n", 0o644)
	producer := produce.NewTerraformComponent(produce.WithStagingRoot(t.TempDir()))

	t.Run("basename is the default name and the version stays explicit", func(t *testing.T) {
		req := request(src, artifact.PackagingNone)
		req.Name = ""
		prepared := prepareKind(t, producer, req)
		assert.Equal(t, "vpc", prepared.Record.Name)
		assert.Equal(t, "1.0.0", prepared.Record.Version)
	})
	t.Run("a missing version fails without a generated one", func(t *testing.T) {
		req := request(src, artifact.PackagingNone)
		req.Version = ""
		_, err := producer.Prepare(context.Background(), *req)
		require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)
	})
	t.Run("an injected metadata source supplies name and version", func(t *testing.T) {
		injected := produce.NewTerraformComponent(produce.WithStagingRoot(t.TempDir()), produce.WithMetadataSource(
			func(context.Context, string) (produce.ProducerMetadata, error) {
				return produce.ProducerMetadata{Name: "network/vpc", Version: "2.3.4", Description: "VPC"}, nil
			},
		))
		req := request(src, artifact.PackagingNone)
		req.Name, req.Version = "", ""
		prepared := prepareKind(t, injected, req)
		assert.Equal(t, "network/vpc", prepared.Record.Name)
		assert.Equal(t, "2.3.4", prepared.Record.Version)
		assert.Equal(t, "VPC", prepared.Record.Metadata.Description)
		assert.NotEmpty(t, prepared.Record.KindData, "the component subtype survives an injected metadata source")
	})
}

func TestKindProducers_RejectMismatchedKindsAndFileSources(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "main.tf", "# x\n", 0o644)
	file := filepath.Join(src, "main.tf")

	t.Run("component rejects another kind", func(t *testing.T) {
		req := request(src, artifact.PackagingNone)
		req.Kind = artifact.KindFiles
		_, err := produce.NewTerraformComponent().Prepare(context.Background(), *req)
		require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	})
	t.Run("files rejects a component kind", func(t *testing.T) {
		req := request(src, artifact.PackagingNone)
		req.Kind = produce.KindComponentTerraform
		_, err := produce.New().Prepare(context.Background(), *req)
		require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	})
	t.Run("component requires a directory", func(t *testing.T) {
		_, err := produce.NewTerraformComponent().Prepare(context.Background(), *request(file, artifact.PackagingNone))
		require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid)
	})
	t.Run("scaffold requires a directory", func(t *testing.T) {
		validator := produce.ScaffoldValidatorFunc(func(context.Context, string) (produce.ProducerMetadata, error) {
			t.Fatal("the validator must not run for a file source")
			return produce.ProducerMetadata{}, nil
		})
		_, err := produce.NewScaffold(produce.WithScaffoldValidator(validator)).Prepare(context.Background(), *request(file, artifact.PackagingNone))
		require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid)
	})
	t.Run("a missing component source fails", func(t *testing.T) {
		_, err := produce.NewTerraformComponent().Prepare(context.Background(), *request(filepath.Join(src, "absent"), artifact.PackagingNone))
		require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid)
	})
}

// scaffoldValidator records the directories it validated and returns canned results.
type scaffoldValidator struct {
	meta  produce.ProducerMetadata
	err   error
	calls []string
}

func (v *scaffoldValidator) ValidateScaffold(_ context.Context, dir string) (produce.ProducerMetadata, error) {
	v.calls = append(v.calls, dir)
	return v.meta, v.err
}

// writeScaffold writes a scaffold with a rendered root README (payload) and distinct package documents.
func writeScaffold(t *testing.T, dir string) {
	t.Helper()
	fixtures.WriteFile(t, dir, "scaffold.yaml", "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: landing-zone\n", 0o644)
	fixtures.WriteFile(t, dir, "README.md", "# {{ .project }}\n", 0o644)
	fixtures.WriteFile(t, dir, "main.tf.tmpl", "# template\n", 0o644)
	fixtures.WriteFile(t, dir, "bin/setup.sh", fixtures.HelperScript, 0o755)
	fixtures.WriteFile(t, dir, fixtures.ReservedDir+"/README.md", fixtures.PackageReadme, 0o644)
	fixtures.WriteFile(t, dir, fixtures.ReservedDir+"/CHANGELOG.md", fixtures.PackageChangelog, 0o644)
}

func TestScaffoldProducer_ValidatesAndSuppliesIdentity(t *testing.T) {
	src := t.TempDir()
	writeScaffold(t, src)
	validator := &scaffoldValidator{meta: produce.ProducerMetadata{Name: "scaffolds/landing-zone", Version: "2.0.0", Description: "A landing zone"}}
	producer := produce.NewScaffold(produce.WithScaffoldValidator(validator), produce.WithStagingRoot(t.TempDir()))

	t.Run("name, version, and description default from the scaffold manifest", func(t *testing.T) {
		req := request(src, artifact.PackagingTgz)
		req.Name, req.Version, req.Kind = "", "", produce.KindScaffold
		prepared := prepareKind(t, producer, req)
		record := prepared.Record

		assert.Equal(t, "scaffold", record.Kind)
		assert.Equal(t, "scaffolds/landing-zone", record.Name)
		assert.Equal(t, "2.0.0", record.Version)
		assert.Equal(t, "A landing zone", record.Metadata.Description)
		assert.Equal(t, []string{src}, validator.calls[len(validator.calls)-1:], "validation reads the source, not a copy")
		assert.Empty(t, prepared.Exclusions, "a scaffold ships exactly what its author selected")
	})
	t.Run("explicit flags win over manifest metadata", func(t *testing.T) {
		req := request(src, artifact.PackagingTgz)
		req.Name, req.Version = "mine/zone", "3.0.0"
		prepared := prepareKind(t, producer, req)
		assert.Equal(t, "mine/zone", prepared.Record.Name)
		assert.Equal(t, "3.0.0", prepared.Record.Version)
	})
	t.Run("root README is payload and the package documents are distinct", func(t *testing.T) {
		prepared := prepareKind(t, producer, request(src, artifact.PackagingNone))
		paths := inventoryPaths(&prepared.Record)
		assert.Contains(t, paths, "README.md")
		assert.Contains(t, paths, "scaffold.yaml")
		for _, path := range paths {
			assert.NotContains(t, path, ".atmos", "reserved distribution metadata is not scaffold payload: %s", path)
		}
		require.Len(t, prepared.Record.Documents, 2)
		assert.Equal(t, ".atmos/artifact/CHANGELOG.md", prepared.Record.Documents[0].SourcePath)
		assert.Equal(t, ".atmos/artifact/README.md", prepared.Record.Documents[1].SourcePath)
	})
}

func TestScaffoldProducer_ValidationFailureStopsPublication(t *testing.T) {
	src := t.TempDir()
	writeScaffold(t, src)
	cause := errors.New("metadata.name is required")
	validator := produce.NewMockScaffoldValidator(gomock.NewController(t))
	validator.EXPECT().ValidateScaffold(gomock.Any(), src).Return(produce.ProducerMetadata{}, errors.Join(errUtils.ErrArtifactScaffoldInvalid, cause))
	staging := t.TempDir()

	_, err := produce.NewScaffold(produce.WithScaffoldValidator(validator), produce.WithStagingRoot(staging)).
		Prepare(context.Background(), *request(src, artifact.PackagingTgz))

	require.ErrorIs(t, err, errUtils.ErrArtifactScaffoldInvalid)
	require.ErrorIs(t, err, cause)
	entries, readErr := os.ReadDir(staging)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "a rejected scaffold leaves no staged data behind")
}

func TestScaffoldProducer_ValidatorSelection(t *testing.T) {
	src := t.TempDir()
	writeScaffold(t, src)

	t.Run("a missing validator fails loudly", func(t *testing.T) {
		produce.RegisterScaffoldValidator(nil)
		_, err := produce.NewScaffold().Prepare(context.Background(), *request(src, artifact.PackagingTgz))
		require.ErrorIs(t, err, errUtils.ErrArtifactProducerUnavailable)
	})
	t.Run("the registered validator is used and an injected one wins", func(t *testing.T) {
		registered := &scaffoldValidator{meta: produce.ProducerMetadata{Version: "1.1.1"}}
		produce.RegisterScaffoldValidator(registered)
		t.Cleanup(func() { produce.RegisterScaffoldValidator(nil) })

		prepared := prepareKind(t, produce.NewScaffold(produce.WithStagingRoot(t.TempDir())), requestWithoutVersion(src))
		assert.Equal(t, "1.1.1", prepared.Record.Version)
		assert.Len(t, registered.calls, 1)

		injected := &scaffoldValidator{meta: produce.ProducerMetadata{Version: "2.2.2"}}
		prepared = prepareKind(t, produce.NewScaffold(produce.WithScaffoldValidator(injected), produce.WithStagingRoot(t.TempDir())), requestWithoutVersion(src))
		assert.Equal(t, "2.2.2", prepared.Record.Version)
		assert.Len(t, registered.calls, 1, "the registered validator is not consulted when one is injected")
	})
}

func requestWithoutVersion(src string) *artifact.PrepareRequest {
	req := request(src, artifact.PackagingTgz)
	req.Version = ""
	return req
}

func TestRegisterDefaults(t *testing.T) {
	registry := artifact.NewProducerRegistry()
	require.NoError(t, produce.RegisterDefaults(registry))
	assert.Equal(t, []string{"component/terraform", "files", "scaffold"}, registry.Kinds())

	for _, kind := range registry.Kinds() {
		producer, ok := registry.Lookup(kind)
		require.True(t, ok)
		assert.Equal(t, kind, producer.Kind())
	}
	require.ErrorIs(t, produce.RegisterDefaults(registry), errUtils.ErrArtifactConflict, "registering twice is an error")
}
