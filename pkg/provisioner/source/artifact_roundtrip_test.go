package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/roundtriptest"
	"github.com/cloudposse/atmos/pkg/oci/ocitest"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard for the vendor source fields this test configures.
var _ = schema.VendorComponentSource{Uri: ""}

const (
	componentName    = "components/terraform/vpc"
	componentVersion = "1.2.0"
	lockFileName     = ".terraform.lock.hcl"
)

// writeTerraformComponent writes a Terraform component with an executable helper, the dependency
// lock file, and every kind of transient Terraform data a working directory accumulates.
func writeTerraformComponent(t *testing.T, dir string) {
	t.Helper()
	fixtures.WriteFile(t, dir, "main.tf", "resource \"null_resource\" \"vpc\" {}\n", 0o644)
	fixtures.WriteFile(t, dir, lockFileName, "# provider lock\n", 0o644)
	fixtures.WriteFile(t, dir, "README.md", "# vpc component\n", 0o644)
	fixtures.WriteFile(t, dir, "scripts/helper.sh", fixtures.HelperScript, 0o755)
	fixtures.WriteFile(t, dir, fixtures.ReservedDir+"/README.md", fixtures.PackageReadme, 0o644)
	fixtures.WriteFile(t, dir, fixtures.ReservedDir+"/CHANGELOG.md", fixtures.PackageChangelog, 0o644)

	fixtures.WriteFile(t, dir, ".terraform/providers/provider.bin", "binary\n", 0o755)
	fixtures.WriteFile(t, dir, "terraform.tfstate", "{}\n", 0o644)
	fixtures.WriteFile(t, dir, "terraform.tfstate.backup", "{}\n", 0o644)
	fixtures.WriteFile(t, dir, "deploy.tfplan", "plan\n", 0o644)
}

// assertProvisionedComponent checks what a provisioned component tree must look like.
func assertProvisionedComponent(t *testing.T, target string) {
	t.Helper()
	for _, kept := range []string{"main.tf", lockFileName, "README.md", filepath.Join("scripts", "helper.sh")} {
		assert.FileExists(t, filepath.Join(target, kept), "%s is part of the component", kept)
	}
	for _, transient := range []string{".terraform", "terraform.tfstate", "terraform.tfstate.backup", "deploy.tfplan"} {
		assert.NoFileExists(t, filepath.Join(target, transient), "%s is Terraform transient data", transient)
	}
	assert.NoDirExists(t, filepath.Join(target, ".atmos"), "distribution metadata must not leak into the provisioned component")

	if fixtures.POSIXModes() {
		info, err := os.Stat(filepath.Join(target, "scripts", "helper.sh"))
		require.NoError(t, err)
		assert.NotZero(t, info.Mode().Perm()&0o100, "the helper script is still executable")
	}
	content, err := os.ReadFile(filepath.Join(target, lockFileName))
	require.NoError(t, err)
	assert.Equal(t, "# provider lock\n", string(content))
}

// TestVendorSourceOCI_PublishedComponentRoundTrip publishes a component with `component/terraform`,
// then provisions it through the existing source provisioning path.
func TestVendorSourceOCI_PublishedComponentRoundTrip(t *testing.T) {
	reg := roundtriptest.New(t)
	src := t.TempDir()
	writeTerraformComponent(t, src)

	dryRun, err := reg.Service.Publish(context.Background(), artifact.PublishRequest{
		Repository: roundtriptest.Alias, Source: src, Name: componentName, Version: componentVersion,
		Kind: "component/terraform", DryRun: true,
	})
	require.NoError(t, err)
	var excluded []string
	for _, diagnostic := range dryRun.Diagnostics {
		if diagnostic.Code == "excluded" {
			excluded = append(excluded, diagnostic.Message)
		}
	}
	assert.ElementsMatch(t, []string{
		"excluded .terraform (rule .terraform/**)",
		"excluded deploy.tfplan (rule *.tfplan)",
		"excluded terraform.tfstate (rule *.tfstate)",
		"excluded terraform.tfstate.backup (rule *.tfstate.*)",
	}, excluded, "the dry run reports every default exclusion with its rule")
	require.Empty(t, reg.Requests(), "a dry run never contacts the registry")

	published := reg.Publish(t, &artifact.PublishRequest{Source: src, Name: componentName, Version: componentVersion, Kind: "component/terraform"})
	require.Equal(t, artifact.OutcomeCreated, published.Outcome)

	target := filepath.Join(t.TempDir(), "components", "terraform", "vpc")
	require.NoError(t, VendorSource(context.Background(), nil, &schema.VendorComponentSource{Uri: reg.URI(componentName, componentVersion)}, target))

	assertProvisionedComponent(t, target)
}

// TestVendorSourceOCI_GenericFilesKeepRequestedTransientFiles proves the component exclusions are not
// inherited by generic file publishing: a backup of a working directory retains what was requested.
func TestVendorSourceOCI_GenericFilesKeepRequestedTransientFiles(t *testing.T) {
	reg := roundtriptest.New(t)
	src := t.TempDir()
	writeTerraformComponent(t, src)

	reg.Publish(t, &artifact.PublishRequest{Source: src, Name: "backups/vpc", Version: "1.0.0"})
	target := filepath.Join(t.TempDir(), "restored")
	require.NoError(t, VendorSource(context.Background(), nil, &schema.VendorComponentSource{Uri: reg.URI("backups/vpc", "1.0.0")}, target))

	for _, kept := range []string{".terraform/providers/provider.bin", "terraform.tfstate", "terraform.tfstate.backup", "deploy.tfplan", lockFileName} {
		assert.FileExists(t, filepath.Join(target, filepath.FromSlash(kept)), "%s was requested and must survive a generic backup", kept)
	}
	assert.NoDirExists(t, filepath.Join(target, ".atmos"), "distribution metadata never reaches a provisioned tree")
}

// TestVendorSourceOCI_PublishedComponentDocumentsNeedNoPayload shows that the package README and
// CHANGELOG are read from their own blobs without pulling the payload.
func TestVendorSourceOCI_PublishedComponentDocumentsNeedNoPayload(t *testing.T) {
	reg := roundtriptest.New(t)
	src := t.TempDir()
	writeTerraformComponent(t, src)
	reg.Publish(t, &artifact.PublishRequest{Source: src, Name: componentName, Version: componentVersion, Kind: "component/terraform"})
	ref := artifact.Ref{Name: componentName, Version: componentVersion}
	inspected, err := reg.Service.Inspect(context.Background(), artifact.InspectRequest{Repository: roundtriptest.Alias, Ref: ref})
	require.NoError(t, err)
	require.Len(t, inspected.Inspection.Record.Documents, 2)
	require.Len(t, inspected.Inspection.Representation.Payload, 1)
	payload := inspected.Inspection.Representation.Payload[0].Digest.String()
	reg.ResetRequests()

	readme, err := reg.Service.ShowDocument(context.Background(), artifact.ShowDocumentRequest{Repository: roundtriptest.Alias, Ref: ref, Role: artifact.DocumentReadme})
	require.NoError(t, err)
	assert.Equal(t, fixtures.PackageReadme, string(readme.Content))
	changelog, err := reg.Service.ShowDocument(context.Background(), artifact.ShowDocumentRequest{Repository: roundtriptest.Alias, Ref: ref, Role: artifact.DocumentChangelog})
	require.NoError(t, err)
	assert.Equal(t, fixtures.PackageChangelog, string(changelog.Content))

	requests := reg.Requests()
	require.NotEmpty(t, requests, "the documents were read from the registry")
	for _, request := range requests {
		assert.NotContains(t, request, payload, "documentation reads must not fetch the payload blob")
	}
}

// TestVendorSourceOCI_LegacyComponentLayerStillExtracts keeps the pre-existing single tar+gzip
// component format working next to the new Atmos package format.
func TestVendorSourceOCI_LegacyComponentLayerStillExtracts(t *testing.T) {
	imageRef := ocitest.NewRegistry(t, "test/legacy-component:v1", map[string]string{"main.tf": "# legacy\n"})
	target := filepath.Join(t.TempDir(), "legacy")
	require.NoError(t, VendorSource(context.Background(), nil, &schema.VendorComponentSource{Uri: "oci://" + imageRef}, target))
	assert.FileExists(t, filepath.Join(target, "main.tf"))
	assert.NoDirExists(t, filepath.Join(target, ".atmos"))
}
