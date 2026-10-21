package release

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestReleaseMetadataRejectsBothIncidents(t *testing.T) {
	dir := t.TempDir()
	const oldCommit = "b392e54f74a9c436d2c27fc8ba1b82605e95b58e"
	const taggedCommit = "12f83b5248253662d8af06030e3966034a7ef5d4"
	for _, tc := range []struct {
		name, tag, builtTag, builtCommit string
	}{
		{"wrong version", "v1.233.1", "v1.234.0", taggedCommit},
		{"right version wrong commit", "v1.233.0", "v1.233.0", oldCommit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(releaseMetadata{Tag: tc.builtTag, Version: tc.builtTag[1:], Commit: tc.builtCommit})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o600))
			_, err = verifyArtifacts(dir, tc.tag, taggedCommit)
			require.ErrorContains(t, err, "release identity mismatch")
		})
	}
}

func TestChecksumsRejectTamperingAndUnsafePaths(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "asset"), []byte("original"), 0o600))
	digest, err := digestFile(filepath.Join(dir, "asset"))
	require.NoError(t, err)
	for _, tc := range []struct{ name, body, want string }{
		{"empty", "", "empty checksums"},
		{"traversal", strings.Repeat("a", 64) + "  ../asset\n", "invalid checksum"},
		{"duplicate", digest + "  asset\n" + digest + "  asset\n", "duplicate checksum"},
		{"wrong digest", strings.Repeat("a", 64) + "  asset\n", "checksum mismatch"},
		{"missing file", digest + "  missing\n", "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "checksums"), []byte(tc.body), 0o600))
			_, err := verifyChecksums(dir, "checksums")
			require.ErrorContains(t, err, tc.want)
		})
	}
}

// Exercise real stripped ELF, Mach-O and PE files without executing them.
// This catches regressions that fabricated build-info structs cannot expose.
func TestVerifyReleaseBinaries(t *testing.T) {
	source, commit := binaryFixtureSource(t)
	dir := t.TempDir()
	const tag = "v1.233.0"
	for _, platform := range []string{"linux_amd64", "darwin_arm64", "windows_amd64"} {
		t.Run(platform, func(t *testing.T) {
			binary := buildFixture(t, source, dir, tag, commit, platform, true)
			require.NoError(t, verifyBinary(binary, tag, commit, platform))
			require.ErrorContains(t, verifyBinary(binary, "v1.234.0", commit, platform), "identity mismatch")
			require.ErrorContains(t, verifyBinary(binary, tag, strings.Repeat("a", 40), platform), "commit mismatch")
			require.ErrorContains(t, verifyBinary(binary, tag, commit, "wrong_platform"), "platform mismatch")
		})
	}
	missing := buildFixture(t, source, dir, tag, commit, "linux_amd64", false)
	require.ErrorContains(t, verifyBinary(missing, tag, commit, "linux_amd64"), "commit mismatch")
	wrongVersion := buildFixture(t, source, dir, "v1.234.0", commit, "linux_amd64", true)
	require.ErrorContains(t, verifyBinary(wrongVersion, tag, commit, "linux_amd64"), "identity mismatch")
	require.NoError(t, os.WriteFile(filepath.Join(source, "dirty"), []byte("untracked"), 0o600))
	dirty := buildFixture(t, source, dir, tag, commit, "linux_amd64", true)
	require.ErrorContains(t, verifyBinary(dirty, tag, commit, "linux_amd64"), "modified=true")
}

func binaryFixtureSource(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module releasefixture\n\ngo 1.24\n"), 0o600))
	main := "package main\nimport \"fmt\"\nvar identity string\nfunc main() { fmt.Println(identity) }\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0o600))
	gitFixture(t, dir, "init", "--quiet")
	gitFixture(t, dir, "add", "go.mod", "main.go")
	gitFixture(t, dir, "-c", "user.name=Release Test", "-c", "user.email=release@example.com",
		"-c", "commit.gpgsign=false", "-c", "core.hooksPath="+t.TempDir(), "commit", "--quiet", "-m", "fixture")
	return dir, strings.TrimSpace(gitFixture(t, dir, "rev-parse", "HEAD"))
}

func gitFixture(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	return string(output)
}

func buildFixture(t *testing.T, source, dir, tag, commit, platform string, vcs bool) string {
	t.Helper()
	parts := strings.Split(platform, "_")
	path := filepath.Join(dir, platform+"-"+tag)
	identity := "atmos-release:" + tag + ":" + commit + ":end"
	cmd := exec.Command("go", "build", "-trimpath", fmt.Sprintf("-buildvcs=%t", vcs),
		"-ldflags=-s -w -X main.identity="+identity, "-o", path, ".")
	cmd.Dir = source
	cmd.Env = append(os.Environ(), "GOOS="+parts[0], "GOARCH="+parts[1], "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	return path
}

func TestVerifyCompleteArtifactSet(t *testing.T) {
	source, commit := binaryFixtureSource(t)
	dir := t.TempDir()
	const tag = "v1.233.1-rc.0"
	var checksums strings.Builder
	for _, platform := range releasePlatforms {
		built := buildFixture(t, source, t.TempDir(), tag, commit, platform, true)
		name := "atmos_" + tag[1:] + "_" + platform
		if strings.HasPrefix(platform, "windows_") {
			name += ".exe"
		}
		data, err := os.ReadFile(built)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o600))
		digest, err := digestFile(filepath.Join(dir, name))
		require.NoError(t, err)
		fmt.Fprintf(&checksums, "%s  %s\n", digest, name)
	}
	metadata, err := json.Marshal(releaseMetadata{Tag: tag, Version: tag[1:], Commit: commit})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"), metadata, 0o600))
	checksumName := "atmos_" + tag[1:] + "_SHA256SUMS"
	require.NoError(t, os.WriteFile(filepath.Join(dir, checksumName), []byte(checksums.String()), 0o600))
	for _, name := range []string{checksumName + ".sigstore.json", "atmos-sbom.spdx.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(`{"fixture":true}`), 0o600))
	}
	require.NoError(t, (Release{}).VerifyArtifacts(dir, tag, commit))
	data, err := os.ReadFile(filepath.Join(dir, "release-manifest.json"))
	require.NoError(t, err)
	var manifest ArtifactManifest
	require.NoError(t, json.Unmarshal(data, &manifest))
	require.Equal(t, tag, manifest.Tag)
	require.Equal(t, commit, manifest.Commit)
	require.Len(t, manifest.Assets, len(releasePlatforms)+4)
	// Removing a platform from both the files and the checksum list still fails.
	remaining := strings.SplitAfter(checksums.String(), "\n")[1:]
	require.NoError(t, os.WriteFile(filepath.Join(dir, checksumName), []byte(strings.Join(remaining, "")), 0o600))
	_, err = verifyArtifacts(dir, tag, commit)
	require.ErrorContains(t, err, "missing binary")
}

func TestShippingMatrixMatchesGoReleaser(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".goreleaser.yml"))
	require.NoError(t, err)
	var config struct {
		Builds []struct {
			Goos   []string `yaml:"goos"`
			Goarch []string `yaml:"goarch"`
			Ignore []struct {
				Goos   string `yaml:"goos"`
				Goarch string `yaml:"goarch"`
			} `yaml:"ignore"`
		} `yaml:"builds"`
	}
	require.NoError(t, yaml.Unmarshal(data, &config))
	require.Len(t, config.Builds, 1)
	build := config.Builds[0]
	ignored := make(map[string]bool)
	for _, platform := range build.Ignore {
		ignored[platform.Goos+"_"+platform.Goarch] = true
	}
	var expected []string
	for _, goos := range build.Goos {
		for _, goarch := range build.Goarch {
			platform := goos + "_" + goarch
			if !ignored[platform] {
				expected = append(expected, platform)
			}
		}
	}
	require.ElementsMatch(t, expected, releasePlatforms, "update the guard when changing the shipping matrix")
}
