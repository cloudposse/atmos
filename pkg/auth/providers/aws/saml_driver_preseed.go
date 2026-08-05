package aws

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// playwright-go's built-in driver download is broken: it fetches the driver zip
// from the retired azureedge.net CDN hosts, and the matching build has been
// purged from the replacement CDN. The driver zip was only ever a bundle of the
// playwright-core npm package plus a Node.js binary, so Atmos pre-seeds the
// driver directory from the official npm registry and nodejs.org instead
// (mirroring what newer playwright-go releases do). With the directory seeded,
// playwright-go's isUpToDateDriver() check passes and its dead download path
// never runs — for Atmos's own install call and for saml2aws's internal one.
const (
	// Node.js runtime version downloaded alongside the playwright-core package
	// when no PLAYWRIGHT_NODEJS_PATH is provided.
	playwrightNodeVersion = "24.18.0"

	// Timeout for each artifact download.
	preseedDownloadTimeout = 5 * time.Minute

	// File mode for seeded directories and executables.
	preseedFileMode = 0o755

	windowsOS = "windows"
)

// Download hosts are variables so tests can point them at local servers.
var (
	npmRegistryBase = "https://registry.npmjs.org"
	nodejsDistBase  = "https://nodejs.org/dist"

	preseedHTTPClient = &http.Client{Timeout: preseedDownloadTimeout}
)

// ensurePlaywrightDriver pre-seeds playwright-go's default driver directory
// (<cache>/ms-playwright-go/<version>, or PLAYWRIGHT_DRIVER_PATH) with
// package/cli.js from the playwright-core npm package and a Node.js binary.
// It is a fast no-op when the driver is already present.
func ensurePlaywrightDriver() error {
	defer perf.Track(nil, "aws.ensurePlaywrightDriver")()

	opts := &playwright.RunOptions{}
	driver, err := playwright.NewDriver(opts)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrPlaywrightDriverSeed, err)
	}
	driverDir := opts.DriverDirectory

	if _, err := os.Stat(filepath.Join(driverDir, "package", "cli.js")); err != nil {
		if err := seedPlaywrightCore(driverDir, driver.Version); err != nil {
			return err
		}
	}

	return seedNodeBinary(driverDir)
}

// seedPlaywrightCore downloads playwright-core from the npm registry, verifies
// the registry-published integrity hash, and extracts the tarball's package/
// tree into the driver directory (yielding <driverDir>/package/cli.js).
func seedPlaywrightCore(driverDir, version string) error {
	tarballURL, integrity, err := npmPackageDist("playwright-core", version)
	if err != nil {
		return err
	}

	body, err := preseedDownload(tarballURL)
	if err != nil {
		return err
	}
	if err := verifyNpmIntegrity(body, integrity); err != nil {
		return err
	}

	log.Debug("Seeding Playwright driver package", "url", tarballURL, "dir", driverDir)
	return extractNpmPackage(body, driverDir)
}

// npmPackageDist fetches npm registry metadata for a package version and
// returns its tarball URL and integrity hash.
func npmPackageDist(name, version string) (tarballURL, integrity string, err error) {
	metaBody, err := preseedDownload(fmt.Sprintf("%s/%s/%s", npmRegistryBase, name, version))
	if err != nil {
		return "", "", err
	}
	var meta struct {
		Dist struct {
			Tarball   string `json:"tarball"`
			Integrity string `json:"integrity"`
		} `json:"dist"`
	}
	if err := json.Unmarshal(metaBody, &meta); err != nil {
		return "", "", fmt.Errorf("could not parse npm metadata for %s@%s: %w", name, version, err)
	}
	if meta.Dist.Tarball == "" {
		return "", "", fmt.Errorf("%w: npm metadata for %s@%s has no tarball URL", errUtils.ErrPlaywrightDriverSeed, name, version)
	}
	return meta.Dist.Tarball, meta.Dist.Integrity, nil
}

// verifyNpmIntegrity checks the downloaded tarball against the registry's
// SRI-style integrity value (sha512-<base64>). An absent integrity value fails
// closed: the registry always publishes one.
func verifyNpmIntegrity(body []byte, integrity string) error {
	digest, ok := strings.CutPrefix(integrity, "sha512-")
	if !ok {
		return fmt.Errorf("%w: unsupported npm integrity value %q", errUtils.ErrPlaywrightDriverSeed, integrity)
	}
	sum := sha512.Sum512(body)
	if base64.StdEncoding.EncodeToString(sum[:]) != digest {
		return fmt.Errorf("%w: npm tarball integrity mismatch (expected %s)", errUtils.ErrPlaywrightDriverSeed, integrity)
	}
	return nil
}

// extractNpmPackage extracts the regular files under the tarball's top-level
// package/ directory into driverDir, preserving file modes.
func extractNpmPackage(tgz []byte, driverDir string) error {
	gzReader, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return fmt.Errorf("could not read playwright-core archive: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	extracted := false
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("could not read playwright-core archive: %w", err)
		}
		// npm tarballs nest everything under a top-level "package/" directory,
		// which is exactly the layout playwright-go expects on disk.
		if header.Typeflag != tar.TypeReg || !strings.HasPrefix(header.Name, "package/") {
			continue
		}
		diskPath, err := safeDriverJoin(driverDir, header.Name)
		if err != nil {
			return err
		}
		if err := writePreseedFile(diskPath, tarReader, header.FileInfo().Mode()); err != nil {
			return err
		}
		extracted = true
	}
	if !extracted {
		return fmt.Errorf("%w: no files extracted from playwright-core archive", errUtils.ErrPlaywrightDriverSeed)
	}
	return nil
}

// seedNodeBinary places a Node.js binary at <driverDir>/node[.exe], verifying
// it against nodejs.org's published SHASUMS256. It is a no-op when
// PLAYWRIGHT_NODEJS_PATH points at a preinstalled Node.js (which also covers
// platforms without compatible prebuilt nodejs.org binaries) or the binary
// already exists.
func seedNodeBinary(driverDir string) error {
	if os.Getenv("PLAYWRIGHT_NODEJS_PATH") != "" { //nolint:forbidigo // Third-party (playwright-go) env var, read the same way the library reads it.
		return nil
	}
	nodeName := "node"
	if runtime.GOOS == windowsOS {
		nodeName = "node.exe"
	}
	nodePath := filepath.Join(driverDir, nodeName)
	if _, err := os.Stat(nodePath); err == nil {
		return nil
	}

	suffix, err := nodePlatformSuffix()
	if err != nil {
		return err
	}
	archiveDir := fmt.Sprintf("node-v%s-%s", playwrightNodeVersion, suffix)
	ext := "tar.gz"
	if runtime.GOOS == windowsOS {
		ext = "zip"
	}
	archiveName := archiveDir + "." + ext

	body, err := preseedDownload(fmt.Sprintf("%s/v%s/%s", nodejsDistBase, playwrightNodeVersion, archiveName))
	if err != nil {
		return err
	}
	if err := verifyNodeChecksum(body, archiveName); err != nil {
		return err
	}

	log.Debug("Seeding Playwright driver Node.js runtime", "archive", archiveName, "dir", driverDir)
	if runtime.GOOS == windowsOS {
		// The Windows archive is a zip with node.exe at "<archiveDir>/node.exe".
		return extractZipSingleEntry(body, archiveDir+"/node.exe", nodePath)
	}
	// Unix archives are gzipped tars with the binary at "<archiveDir>/bin/node".
	return extractTarGzSingleEntry(body, archiveDir+"/bin/node", nodePath)
}

// verifyNodeChecksum checks a nodejs.org archive against the release's
// published SHASUMS256.txt.
func verifyNodeChecksum(body []byte, archiveName string) error {
	sums, err := preseedDownload(fmt.Sprintf("%s/v%s/SHASUMS256.txt", nodejsDistBase, playwrightNodeVersion))
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == archiveName {
			sum := sha256.Sum256(body)
			if hex.EncodeToString(sum[:]) != fields[0] {
				return fmt.Errorf("%w: checksum mismatch for %s", errUtils.ErrPlaywrightDriverSeed, archiveName)
			}
			return nil
		}
	}
	return fmt.Errorf("%w: no published checksum found for %s", errUtils.ErrPlaywrightDriverSeed, archiveName)
}

// nodePlatformSuffix maps GOOS/GOARCH to nodejs.org's archive naming.
func nodePlatformSuffix() (string, error) {
	return nodePlatformSuffixFor(runtime.GOOS, runtime.GOARCH, runtime.GOOS == "linux" && hostUsesMuslLibc())
}

// nodePlatformSuffixFor maps a platform to nodejs.org's archive naming. It
// refuses Linux/musl because nodejs.org's Linux binaries are glibc-linked.
func nodePlatformSuffixFor(goos, goarch string, linuxMusl bool) (string, error) {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[goarch]
	if arch == "" {
		return "", errNoPrebuiltNode(goos, goarch)
	}
	switch goos {
	case "linux":
		if linuxMusl {
			return "", fmt.Errorf("%w: nodejs.org does not publish musl-linked Node.js for %s/%s; set PLAYWRIGHT_NODEJS_PATH to a preinstalled musl-compatible Node.js", errUtils.ErrPlaywrightDriverSeed, goos, goarch)
		}
		return goos + "-" + arch, nil
	case "darwin":
		return goos + "-" + arch, nil
	case windowsOS:
		return "win-" + arch, nil
	}
	return "", errNoPrebuiltNode(goos, goarch)
}

func errNoPrebuiltNode(goos, goarch string) error {
	return fmt.Errorf("%w: no prebuilt Node.js for %s/%s; set PLAYWRIGHT_NODEJS_PATH to a preinstalled Node.js", errUtils.ErrPlaywrightDriverSeed, goos, goarch)
}

func hostUsesMuslLibc() bool {
	for _, pattern := range []string{
		"/lib/ld-musl-*.so.*",
		"/usr/lib/ld-musl-*.so.*",
		"/lib/libc.musl-*.so.*",
		"/usr/lib/libc.musl-*.so.*",
	} {
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			return true
		}
	}

	ldd, err := os.ReadFile("/usr/bin/ldd")
	if err == nil && bytes.Contains(bytes.ToLower(ldd), []byte("musl")) {
		return true
	}

	return false
}

// extractTarGzSingleEntry extracts one named entry from a gzipped tar into
// destPath as an executable file.
func extractTarGzSingleEntry(archive []byte, entryName, destPath string) error {
	gzReader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("could not read Node.js archive: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("could not read Node.js archive: %w", err)
		}
		if header.Typeflag == tar.TypeReg && header.Name == entryName {
			return writePreseedFile(destPath, tarReader, preseedFileMode)
		}
	}
	return fmt.Errorf("%w: entry %s not found in Node.js archive", errUtils.ErrPlaywrightDriverSeed, entryName)
}

// extractZipSingleEntry extracts one named entry from a zip into destPath as an
// executable file.
func extractZipSingleEntry(archive []byte, entryName, destPath string) error {
	zipReader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return fmt.Errorf("could not read Node.js archive: %w", err)
	}
	for _, zipFile := range zipReader.File {
		if zipFile.Name != entryName {
			continue
		}
		file, err := zipFile.Open()
		if err != nil {
			return fmt.Errorf("could not open Node.js archive entry: %w", err)
		}
		defer file.Close()
		return writePreseedFile(destPath, file, preseedFileMode)
	}
	return fmt.Errorf("%w: entry %s not found in Node.js archive", errUtils.ErrPlaywrightDriverSeed, entryName)
}

// safeDriverJoin joins an archive entry name onto the driver directory,
// rejecting entries that would escape it.
func safeDriverJoin(driverDir, entryName string) (string, error) {
	diskPath := filepath.Join(driverDir, filepath.FromSlash(entryName))
	if !strings.HasPrefix(diskPath, filepath.Clean(driverDir)+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: archive entry %q escapes the driver directory", errUtils.ErrPlaywrightDriverSeed, entryName)
	}
	return diskPath, nil
}

// writePreseedFile writes reader contents to path, creating parent directories.
func writePreseedFile(path string, reader io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), preseedFileMode); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}
	outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return fmt.Errorf("could not create file: %w", err)
	}
	if _, err := io.Copy(outFile, reader); err != nil {
		outFile.Close()
		return fmt.Errorf("could not write file: %w", err)
	}
	return outFile.Close()
}

// preseedDownload fetches a URL, returning the body on HTTP 200.
func preseedDownload(url string) ([]byte, error) {
	resp, err := preseedHTTPClient.Get(url) //nolint:noctx // No request-scoped context is available on this pre-seed path; the client carries a timeout.
	if err != nil {
		return nil, fmt.Errorf("could not download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: download of %s returned status %d", errUtils.ErrPlaywrightDriverSeed, url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", url, err)
	}
	return body, nil
}
