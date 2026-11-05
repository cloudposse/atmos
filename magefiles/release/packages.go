package release

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type releaseRunner func(context.Context, []byte, string, ...string) ([]byte, error)

func runReleaseTool(ctx context.Context, input []byte, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w: %s", name, err, stderr.String())
	}
	return output, nil
}

// VerifyBuild checks the unsigned artifacts before they can be signed. Package
// binaries must be byte-for-byte identical to an already verified Linux binary.
func (Release) VerifyBuild(dir, tag, commit string) error {
	version := strings.TrimPrefix(tag, "v")
	if err := verifyMetadata(dir, tag, version, commit); err != nil {
		return err
	}
	assets, err := verifyChecksums(dir, "atmos_"+version+"_SHA256SUMS")
	if err != nil {
		return err
	}
	if err = verifyBinaries(dir, tag, version, commit, assets); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	return verifyPackages(ctx, dir, version, assets, runReleaseTool)
}

// Package formats share the same payload check; Debian wraps its data tarball
// in an ar archive, while RPM and APK can be inspected directly by bsdtar.
const packageArchiveTool = "bsdtar"

var linuxPackageFormats = []string{".apk", ".deb", ".rpm"}

func verifyPackages(ctx context.Context, dir, version string, assets map[string]string, run releaseRunner) error {
	binaries := verifiedLinuxDigests(version, assets)
	seen := map[string]bool{}
	for name := range assets {
		format := filepath.Ext(name)
		if !slices.Contains(linuxPackageFormats, format) {
			continue
		}
		binary, err := packageExecutable(ctx, filepath.Join(dir, name), run)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(binary)
		platform, ok := binaries[hex.EncodeToString(hash[:])]
		if !ok {
			return fmt.Errorf("%w: package %s embeds an unverified executable", ErrReleaseVerification, name)
		}
		key := format + platform
		if seen[key] {
			return fmt.Errorf("%w: duplicate package for %s", ErrReleaseVerification, key)
		}
		seen[key] = true
	}
	return requirePackageMatrix(binaries, seen)
}

func verifiedLinuxDigests(version string, assets map[string]string) map[string]string {
	binaries := map[string]string{}
	for _, platform := range releasePlatforms {
		if strings.HasPrefix(platform, "linux_") {
			binaries[assets["atmos_"+version+"_"+platform]] = platform
		}
	}
	return binaries
}

func requirePackageMatrix(binaries map[string]string, seen map[string]bool) error {
	for _, format := range linuxPackageFormats {
		for _, platform := range binaries {
			if !seen[format+platform] {
				return fmt.Errorf("%w: missing %s package for %s", ErrReleaseVerification, format, platform)
			}
		}
	}
	return nil
}

// Package contents are inspected on stdout, never extracted onto the runner's
// filesystem. Absolute RPM paths therefore cannot overwrite trusted tooling.
func packageExecutable(ctx context.Context, archive string, run releaseRunner) ([]byte, error) {
	name := filepath.Base(archive)
	var input []byte
	if filepath.Ext(archive) == ".deb" {
		var err error
		input, err = debianPayload(ctx, archive, run)
		if err != nil {
			return nil, err
		}
		archive = "-"
	}
	listing, err := run(ctx, input, packageArchiveTool, "--ignore-zeros", "-tf", archive)
	if err != nil {
		return nil, err
	}
	member, err := executableMember(string(listing), name)
	if err != nil {
		return nil, err
	}
	return run(ctx, input, packageArchiveTool, "--ignore-zeros", "-xOf", archive, member)
}

func debianPayload(ctx context.Context, archive string, run releaseRunner) ([]byte, error) {
	listing, err := run(ctx, nil, packageArchiveTool, "-tf", archive)
	if err != nil {
		return nil, err
	}
	dataMember := ""
	for _, entry := range strings.Fields(string(listing)) {
		if !strings.HasPrefix(entry, "data.tar") {
			continue
		}
		if dataMember != "" {
			return nil, fmt.Errorf("%w: multiple data archives in %s", ErrReleaseVerification, filepath.Base(archive))
		}
		dataMember = entry
	}
	if dataMember == "" {
		return nil, fmt.Errorf("%w: missing Debian data archive", ErrReleaseVerification)
	}
	return run(ctx, nil, packageArchiveTool, "-xOf", archive, dataMember)
}

func executableMember(listing, packageName string) (string, error) {
	member := ""
	for _, entry := range strings.Fields(listing) {
		// nFPM RPM headers use an absolute path; Debian and APK payloads use
		// relative paths. Accept exactly those paths, without traversal cleanup.
		if strings.TrimPrefix(entry, "./") != "usr/bin/atmos" && entry != "/usr/bin/atmos" {
			continue
		}
		if member != "" {
			return "", fmt.Errorf("%w: duplicate executable in %s", ErrReleaseVerification, packageName)
		}
		member = entry
	}
	if member == "" {
		return "", fmt.Errorf("%w: package %s has no Atmos executable", ErrReleaseVerification, packageName)
	}
	return member, nil
}
