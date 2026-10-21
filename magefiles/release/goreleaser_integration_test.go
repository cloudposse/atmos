package release

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// Run explicitly with the pinned GoReleaser executable. This exercises its real
// tag selection, artifact schema, filenames and packaging without publishing.
func TestGoReleaserTagAuthority(t *testing.T) {
	goreleaser := os.Getenv("ATMOS_TEST_GORELEASER")
	if goreleaser == "" {
		t.Skip("set ATMOS_TEST_GORELEASER to run the local release rehearsal")
	}
	tool, err := exec.LookPath(goreleaser)
	require.NoError(t, err)
	tool, err = filepath.Abs(tool)
	require.NoError(t, err)
	version, err := exec.Command(tool, "--version").CombinedOutput()
	require.NoError(t, err, "%s", version)
	require.Contains(t, string(version), "2.18.3", "match the production GoReleaser pin")

	source, _ := binaryFixtureSource(t)
	require.NoError(t, os.WriteFile(filepath.Join(source, ".gitignore"), []byte("dist/\n"), 0o600))
	gitFixture(t, source, "add", ".gitignore")
	gitFixture(t, source, "-c", "user.name=Release Test", "-c", "user.email=release@example.com",
		"-c", "commit.gpgsign=false", "-c", "core.hooksPath="+t.TempDir(), "commit", "--quiet", "-m", "ignore generated artifacts")
	commit := strings.TrimSpace(gitFixture(t, source, "rev-parse", "HEAD"))
	gitFixture(t, source, "remote", "add", "origin", "https://github.com/cloudposse/atmos.git")
	tags := []string{"v1.233.0", "v1.233.1-rc.1", "v1.233.1-test.123"}
	for _, tag := range append(tags, "v1.234.0") {
		gitFixture(t, source, "tag", tag)
	}
	config := fixtureGoReleaserConfig(t)
	for _, tag := range tags {
		t.Run(tag, func(t *testing.T) {
			cmd := exec.Command(tool, "release", "--clean", "--skip=publish", "--config", config)
			cmd.Dir = source
			cmd.Env = append(os.Environ(), "GORELEASER_CURRENT_TAG="+tag, "GOWORK=off", "GOFLAGS=")
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
			// Cryptographic signing/OIDC requires GitHub; these fixtures only
			// exercise artifact staging and identity verification locally.
			sbom := filepath.Join(source, "dist", "atmos-sbom.spdx.json")
			require.NoError(t, os.WriteFile(sbom, []byte(`{"fixture":"unsigned rehearsal"}`), 0o600))
			staged := filepath.Join(t.TempDir(), "upload")
			require.NoError(t, (Release{}).StageArtifacts(source, staged))
			signature := filepath.Join(staged, "atmos_"+tag[1:]+"_SHA256SUMS.sigstore.json")
			require.NoError(t, os.WriteFile(signature, []byte(`{"fixture":"unsigned rehearsal"}`), 0o600))
			require.NoError(t, (Release{}).VerifyArtifacts(staged, tag, commit))
			data, err := os.ReadFile(filepath.Join(staged, "release-manifest.json"))
			require.NoError(t, err)
			var manifest ArtifactManifest
			require.NoError(t, json.Unmarshal(data, &manifest))
			require.Equal(t, tag, manifest.Tag)
			require.Equal(t, commit, manifest.Commit)
			// Real packaging must survive staging alongside all ten binaries.
			for _, extension := range []string{".deb", ".rpm", ".apk"} {
				count := 0
				for name := range manifest.Assets {
					if strings.HasSuffix(name, extension) {
						count++
					}
				}
				require.Equal(t, 4, count, "one %s per Linux architecture", extension)
			}
			_, err = verifyArtifacts(staged, "v1.234.0", commit)
			require.ErrorContains(t, err, "release identity mismatch")
		})
	}
}

func fixtureGoReleaserConfig(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".goreleaser.yml"))
	require.NoError(t, err)
	var config map[string]any
	require.NoError(t, yaml.Unmarshal(data, &config))
	// Use the production matrix, package formats and naming rules. Replace
	// only runner cleanup, external signing/SBOM tools and the tiny fixture's
	// linker target. This never runs the production cleanup hook locally.
	for _, key := range []string{"before", "signs", "sboms"} {
		delete(config, key)
	}
	config["project_name"] = "atmos"
	build := config["builds"].([]any)[0].(map[string]any)
	build["env"] = []string{"CGO_ENABLED=0"}
	build["ldflags"] = []string{"-s -w -X main.identity=atmos-release:{{.Tag}}:{{.Commit}}:end"}
	data, err = yaml.Marshal(config)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "goreleaser.yml")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}
