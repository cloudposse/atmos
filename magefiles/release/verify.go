package release

import (
	"bufio"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/internal/releaseidentity"
)

type releaseMetadata struct {
	Tag     string `json:"tag"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

// ArtifactManifest binds the files uploaded to a release to its tag and commit.
type ArtifactManifest struct {
	Tag    string            `json:"tag"`
	Commit string            `json:"commit"`
	Assets map[string]string `json:"assets"`
}

// ErrReleaseVerification prevents publication when release identity or bytes disagree.
var ErrReleaseVerification = errors.New("release verification failed")

var assetName = regexp.MustCompile(`^[A-Za-z0-9_.+-]+$`)

var checksumLine = regexp.MustCompile(`^([0-9a-f]{64})  ([A-Za-z0-9_.+-]+)$`)

// VerifyArtifacts checks the actual distributable bytes before publication and
// writes the exact upload allowlist. It never runs the binaries being inspected.
func (Release) VerifyArtifacts(dir, tag, commit string) error {
	manifest, err := verifyArtifacts(dir, tag, commit)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "release-manifest.json"), append(data, '\n'), releaseFileMode)
}

func verifyArtifacts(dir, tag, commit string) (*ArtifactManifest, error) {
	version, _, err := releaseidentity.Parse("atmos-release:" + tag + ":" + commit + ":end")
	if err != nil {
		return nil, err
	}
	if err = verifyMetadata(dir, tag, version, commit); err != nil {
		return nil, err
	}
	checksums := "atmos_" + version + "_SHA256SUMS"
	assets, err := verifyChecksums(dir, checksums)
	if err != nil {
		return nil, err
	}
	if err = verifyBinaries(dir, tag, version, commit, assets); err != nil {
		return nil, err
	}
	for _, name := range []string{checksums, checksums + ".sigstore.json", "metadata.json", "atmos-sbom.spdx.json"} {
		digest, hashErr := digestFile(filepath.Join(dir, name))
		if hashErr != nil {
			return nil, hashErr
		}
		assets[name] = digest
	}
	if err = verifyAssetSet(dir, assets); err != nil {
		return nil, err
	}
	return &ArtifactManifest{Tag: tag, Commit: commit, Assets: assets}, nil
}

func verifyBinaries(dir, tag, version, commit string, assets map[string]string) error {
	for _, platform := range releasePlatforms {
		name := "atmos_" + version + "_" + platform
		if strings.HasPrefix(platform, "windows_") {
			name += ".exe"
		}
		if _, ok := assets[name]; !ok {
			return fmt.Errorf("%w: release %s is missing binary %s", ErrReleaseVerification, tag, name)
		}
		if err := verifyBinary(filepath.Join(dir, name), tag, commit, platform); err != nil {
			return err
		}
	}
	return nil
}

func verifyAssetSet(dir string, assets map[string]string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == "release-manifest.json" {
			continue // Re-verification replaces this derived file.
		}
		if _, ok := assets[entry.Name()]; !ok {
			return fmt.Errorf("%w: unexpected staged asset %s", ErrReleaseVerification, entry.Name())
		}
	}
	return nil
}

// Keep the full shipping matrix explicit: removing a missing binary from a
// checksums file must never turn a partial build into a successful release.
var releasePlatforms = []string{
	"darwin_amd64", "darwin_arm64",
	"linux_amd64", "linux_arm64",
	"windows_amd64", "windows_arm64",
}

func verifyMetadata(dir, tag, version, commit string) error {
	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return err
	}
	var metadata releaseMetadata
	if err = json.Unmarshal(data, &metadata); err != nil {
		return err
	}
	if metadata.Tag != tag || metadata.Version != version || metadata.Commit != commit {
		return fmt.Errorf("%w: release identity mismatch: expected %s at %s; built %s (%s) at %s", ErrReleaseVerification,
			tag, commit, metadata.Tag, metadata.Version, metadata.Commit)
	}
	return nil
}

func verifyChecksums(dir, name string) (map[string]string, error) {
	file, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	assets := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		match := checksumLine.FindStringSubmatch(scanner.Text())
		if len(match) != 3 || !validAssetName(match[2]) {
			return nil, fmt.Errorf("%w: invalid checksum entry %q", ErrReleaseVerification, scanner.Text())
		}
		if _, exists := assets[match[2]]; exists {
			return nil, fmt.Errorf("%w: duplicate checksum entry %s", ErrReleaseVerification, match[2])
		}
		digest, hashErr := digestFile(filepath.Join(dir, match[2]))
		if hashErr != nil {
			return nil, hashErr
		}
		if digest != match[1] {
			return nil, fmt.Errorf("%w: checksum mismatch for %s", ErrReleaseVerification, match[2])
		}
		assets[match[2]] = digest
	}
	if err = scanner.Err(); err != nil {
		return nil, err
	}
	if len(assets) == 0 {
		return nil, fmt.Errorf("%w: empty checksums file %s", ErrReleaseVerification, name)
	}
	return assets, nil
}

func digestFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return "", fmt.Errorf("%w: release asset must be a nonempty regular file: %s", ErrReleaseVerification, path)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func verifyBinary(path, tag, commit, platform string) error {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read binary provenance %s: %w", path, err)
	}
	settings := make(map[string]string)
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	if settings["vcs.revision"] != commit || settings["vcs.modified"] != "false" {
		return fmt.Errorf("%w: binary commit mismatch: %s expected %s, got %s (modified=%s)", ErrReleaseVerification,
			path, commit, settings["vcs.revision"], settings["vcs.modified"])
	}
	if settings["GOOS"]+"_"+settings["GOARCH"] != platform {
		return fmt.Errorf("%w: binary platform mismatch: %s expected %s", ErrReleaseVerification, path, platform)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	matches := releaseidentity.Pattern.FindAll(data, -1)
	expected := "atmos-release:" + tag + ":" + commit + ":end"
	if len(matches) != 1 || string(matches[0]) != expected {
		return fmt.Errorf("%w: binary version/commit identity mismatch: %s expected %s at %s", ErrReleaseVerification, path, tag, commit)
	}
	return nil
}
