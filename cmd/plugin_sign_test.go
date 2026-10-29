package cmd

import (
	"context"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	containerregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/oci/signature"
	"github.com/cloudposse/atmos/pkg/oci/signature/signaturetest"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/plugin/oci/ocitest"
	pluginsign "github.com/cloudposse/atmos/pkg/plugin/sign"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

var _ = schema.PluginRegistry{Catalog: ""}

func TestPluginSigningCommands(t *testing.T) {
	config := pluginCoverageProject(t)
	layout, release := pluginSigningLayout(t, config)
	server := httptest.NewServer(containerregistry.New(containerregistry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(server.Close)
	repository := strings.TrimPrefix(server.URL, "http://") + "/plugins/oidc"
	catalogRef := "oci://" + strings.TrimPrefix(server.URL, "http://") + "/catalog:latest"
	config.Plugins.Registries = map[string]schema.PluginRegistry{"local": {Catalog: catalogRef}}
	config.Toolchain = schema.Toolchain{Install: "never", InstallPath: filepath.Join(config.BasePath, "tools")}
	writePluginCoverageConfig(t, config)
	previous := toolchain.GetAtmosConfig()
	toolchain.SetAtmosConfig(config)
	t.Cleanup(func() { toolchain.SetAtmosConfig(previous) })
	client := atmosoci.NewClient(config)
	reference, err := name.NewDigest(repository + "@" + release.Digest)
	require.NoError(t, err)
	// Publishing succeeds before signing fails; neither operation updates the configured catalog.
	output, err := invokePluginCoverage(t, config, "publish", layout, "--to", repository, "--sign", "--cosign-version=2.0.0")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.Contains(t, err.Error(), reference.Name())
	require.Contains(t, err.Error(), "was published but signing failed")
	require.Empty(t, output)
	published, err := client.Head(t.Context(), reference)
	require.NoError(t, err)
	require.Equal(t, release.Digest, published.Digest.String())
	_, _, err = catalog.Read(t.Context(), client, catalogRef)
	require.True(t, atmosoci.IsNotFound(err), "catalog must remain absent after failed signing: %v", err)
	evidence, err := signature.Fetch(t.Context(), client, reference)
	require.NoError(t, err)
	require.Empty(t, evidence)
	// Standalone sign validates immutable references and reports missing toolchain dependencies.
	_, err = invokePluginCoverage(t, config, "sign", repository+":0.1.0")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	_, err = invokePluginCoverage(t, config, "sign", reference.Name(), "--yes")
	require.ErrorIs(t, err, errUtils.ErrToolNotInstalled)
	authority := signaturetest.New(t)
	rawIndex, err := release.Index.RawManifest()
	require.NoError(t, err)
	signed := authority.Sign(t, "builder@example.com", "https://issuer.example.com", rawIndex)
	installPluginSigningHelper(t, rawIndex, signed)
	output, err = invokePluginCoverage(t, config, "sign", reference.Name(), "--yes")
	require.NoError(t, err)
	require.Contains(t, output, release.Digest)
	evidence, err = signature.Fetch(t.Context(), client, reference)
	require.NoError(t, err)
	require.Equal(t, [][]byte{signed}, evidence)
	verifier := signature.Verifier{LoadRoot: func(context.Context, bool) (root.TrustedMaterial, error) { return authority, nil }}
	_, err = verifier.Verify(t.Context(), evidence[0], release.Digest, []signature.Identity{{Identity: "builder@example.com", Issuer: "https://issuer.example.com"}}, false)
	require.NoError(t, err)
	// An unchanged publication must honor --sign for the current identity, even
	// when another identity already attached valid evidence to this digest.
	replacement := authority.Sign(t, "other-builder@example.com", "https://issuer.example.com", rawIndex)
	bundleFile := filepath.Join(t.TempDir(), "replacement.json")
	require.NoError(t, os.WriteFile(bundleFile, replacement, 0o600))
	t.Setenv("ATMOS_TEST_COSIGN_BUNDLE", bundleFile)
	// Re-publishing preserves the index and leaves catalog maintenance to its owner.
	output, err = invokePluginCoverage(t, config, "publish", layout, "--to", repository, "--sign", "--yes")
	require.NoError(t, err)
	require.Contains(t, output, release.Digest)
	evidence, err = signature.Fetch(t.Context(), client, reference)
	require.NoError(t, err)
	require.ElementsMatch(t, [][]byte{signed, replacement}, evidence)
	_, err = verifier.Verify(t.Context(), replacement, release.Digest, []signature.Identity{{Identity: "other-builder@example.com", Issuer: "https://issuer.example.com"}}, false)
	require.NoError(t, err)
	_, _, err = catalog.Read(t.Context(), client, catalogRef)
	require.True(t, atmosoci.IsNotFound(err), "publishing must not mutate catalog: %v", err)
	_, err = invokePluginCoverage(t, config, "catalog", "add", "local", reference.Name())
	require.NoError(t, err)
	advertised, _, err := catalog.Read(t.Context(), client, catalogRef)
	require.NoError(t, err)
	rawCatalog, err := yaml.Marshal(advertised)
	require.NoError(t, err)
	require.Contains(t, string(rawCatalog), release.Digest)
	// A subprocess failure still propagates and preserves the catalog snapshot.
	t.Setenv("ATMOS_TEST_COSIGN_FAIL", "1")
	_, err = invokePluginCoverage(t, config, "publish", layout, "--to", repository, "--sign", "--yes")
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	after, _, err := catalog.Read(t.Context(), client, catalogRef)
	require.NoError(t, err)
	require.Equal(t, advertised, after)
}

func pluginSigningLayout(t *testing.T, config *schema.AtmosConfiguration) (string, *pluginoci.Release) {
	t.Helper()
	directory := filepath.Join(config.BasePath, "plugin")
	_, err := invokePluginCoverage(t, config, "create", directory, "--defaults", "--sdk-version=v0.0.0")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(directory, "bin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, pluginManifestExecutable(t, directory)), ocitest.Binary(runtime.GOOS, runtime.GOARCH), 0o700))
	layout := filepath.Join(config.BasePath, "layout")
	_, err = invokePluginCoverage(t, config, "pack", directory, "--output", layout)
	require.NoError(t, err)
	index, err := atmosoci.LoadIndex(layout, "")
	require.NoError(t, err)
	release, err := pluginoci.Inspect(index, "")
	require.NoError(t, err)
	return layout, release
}

func installPluginSigningHelper(t *testing.T, index, signed []byte) {
	t.Helper()
	directory := t.TempDir()
	source := filepath.Join(directory, "main.go")
	require.NoError(t, os.WriteFile(source, []byte(pluginCosignHelperSource), 0o600))
	indexFile, bundleFile := filepath.Join(directory, "index.json"), filepath.Join(directory, "bundle.json")
	require.NoError(t, os.WriteFile(indexFile, index, 0o600))
	require.NoError(t, os.WriteFile(bundleFile, signed, 0o600))
	t.Setenv("ATMOS_TEST_COSIGN_INDEX", indexFile)
	t.Setenv("ATMOS_TEST_COSIGN_BUNDLE", bundleFile)
	name := "cosign"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := toolchain.NewInstaller().GetBinaryPath("sigstore", "cosign", pluginsign.DefaultVersion, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(binary), 0o700))
	command := exec.CommandContext(t.Context(), "go", "build", "-o", binary, source)
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	require.NoError(t, err, "build isolated Go-native signer: %s", output)
	require.FileExists(t, binary)
	t.Run("signer_environment_prerequisite", func(t *testing.T) {
		bundleOutput := filepath.Join(t.TempDir(), "output.json")
		command := exec.CommandContext(t.Context(), binary, "sign-blob", "--new-bundle-format=true", "--bundle", bundleOutput, "--yes", indexFile)
		output, err := command.CombinedOutput()
		require.NoError(t, err, "signer helper environment: %s", output)
		actual, err := os.ReadFile(bundleOutput)
		require.NoError(t, err)
		require.Equal(t, signed, actual)
	})
}

const pluginCosignHelperSource = `package main
import("bytes";"os")
func main(){
 if os.Getenv("ATMOS_TEST_COSIGN_FAIL")=="1" {os.Exit(10)}
 args:=os.Args[1:]
 if len(args)!=6||args[0]!="sign-blob"||args[1]!="--new-bundle-format=true"||args[2]!="--bundle"||args[4]!="--yes" {os.Exit(11)}
 actual,err:=os.ReadFile(args[5]);if err!=nil{os.Exit(12)}
 expected,err:=os.ReadFile(os.Getenv("ATMOS_TEST_COSIGN_INDEX"));if err!=nil||!bytes.Equal(actual,expected){os.Exit(13)}
 bundle,err:=os.ReadFile(os.Getenv("ATMOS_TEST_COSIGN_BUNDLE"));if err!=nil{os.Exit(14)}
 if os.WriteFile(args[3],bundle,0600)!=nil{os.Exit(15)}
}
`
