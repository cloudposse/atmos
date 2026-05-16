package verification

import (
	"bufio"
	"context"
	"crypto/md5"  //nolint:gosec // md5 is supported for Aqua compatibility only.
	"crypto/sha1" //nolint:gosec // sha1 is supported for Aqua compatibility only.
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

const versionPrefixV = "v"

//nolint:revive // The checksum policy branches are kept together to make required/optional behavior explicit.
func (v *Verifier) verifyChecksum(ctx context.Context, req *Request, result *Result) error {
	checksum := &req.Tool.Checksum
	if !hasChecksum(checksum) {
		if req.Policy.Checksums == PolicyRequired {
			return fmt.Errorf("%w: %s/%s@%s has no checksum metadata", ErrChecksumRequired, req.Tool.RepoOwner, req.Tool.RepoName, req.Version)
		}
		result.SkippedReasons = append(result.SkippedReasons, "checksum metadata unavailable")
		return nil
	}
	if checksum.Enabled != nil && !*checksum.Enabled {
		if req.Policy.Checksums == PolicyRequired {
			return fmt.Errorf("%w: checksum metadata is disabled", ErrChecksumRequired)
		}
		result.SkippedReasons = append(result.SkippedReasons, "checksum disabled by registry metadata")
		return nil
	}

	algorithm := strings.ToLower(checksum.Algorithm)
	if algorithm == "" {
		algorithm = "sha256"
	}
	actual, err := digestFile(req.AssetPath, algorithm)
	if err != nil {
		return err
	}

	sidecar, err := v.downloadChecksumData(ctx, req, checksum, result)
	if err != nil || sidecar == nil {
		return err
	}

	if req.Policy.Signatures != PolicyDisabled && hasCosign(&checksum.Cosign) {
		if err := v.verifyChecksumCosign(ctx, req, sidecar.url, sidecar.data, result); err != nil {
			return err
		}
	}

	assetName := assetNameFromURL(req.AssetURL)
	expected, err := expectedChecksum(sidecar.data, assetName, algorithm, checksum)
	if err != nil {
		return err
	}
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expected, actual)
	}
	result.ChecksumAlgorithm = algorithm
	result.Checksum = actual
	return nil
}

type checksumSidecar struct {
	url  string
	data []byte
}

func (v *Verifier) downloadChecksumData(ctx context.Context, req *Request, checksum *registry.ChecksumConfig, result *Result) (*checksumSidecar, error) {
	checksumURL, err := checksumFileURL(req.Tool, req.Version, req.AssetURL, checksum)
	if err != nil {
		return nil, err
	}
	downloader := req.Downloader
	if downloader == nil {
		downloader = v.Downloader
	}
	if downloader == nil {
		downloader = HTTPDownloader{}
	}
	data, err := downloader.Download(ctx, checksumURL)
	if err != nil {
		if req.Policy.Checksums != PolicyRequired {
			result.SkippedReasons = append(result.SkippedReasons, fmt.Sprintf("checksum sidecar unavailable: %v", err))
			return nil, nil
		}
		return nil, err
	}
	return &checksumSidecar{url: checksumURL, data: data}, nil
}

func (v *Verifier) verifyChecksumCosign(ctx context.Context, req *Request, checksumURL string, data []byte, result *Result) error {
	file, err := os.CreateTemp("", "atmos-checksum-*"+path.Ext(checksumURL))
	if err != nil {
		return err
	}
	defer func() {
		// #nosec G703 -- file is a temporary checksum sidecar created by this process.
		_ = os.Remove(file.Name())
	}()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}

	checksumReq := *req
	checksumReq.AssetURL = checksumURL
	checksumReq.AssetPath = file.Name()
	return v.verifyCosign(ctx, &checksumReq, &req.Tool.Checksum.Cosign, result)
}

func hasChecksum(checksum *registry.ChecksumConfig) bool {
	if checksum == nil {
		return false
	}
	return checksum.Type != "" || checksum.Asset != "" || checksum.URL != "" || checksum.Enabled != nil
}

func checksumFileURL(tool *registry.Tool, version, assetURL string, checksum *registry.ChecksumConfig) (string, error) {
	assetName := assetNameFromURL(assetURL)
	if checksum.URL != "" {
		u, err := renderTemplateString(checksum.URL, tool, version, assetName, checksum.Replacements)
		if err != nil {
			return "", err
		}
		return alignSidecarURLWithAssetURL(u, assetURL, version), nil
	}
	checksumAsset, err := renderTemplateString(checksum.Asset, tool, version, assetName, checksum.Replacements)
	if err != nil {
		return "", err
	}
	if checksum.Type == "http" {
		return alignSidecarURLWithAssetURL(checksumAsset, assetURL, version), nil
	}
	repoOwner := tool.RepoOwner
	repoName := tool.RepoName
	releaseVersion, err := checksumReleaseVersion(tool, version, assetURL, assetName, checksum.Replacements)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", repoOwner, repoName, releaseVersion, checksumAsset), nil
}

func checksumReleaseVersion(tool *registry.Tool, version, assetURL, assetName string, replacements map[string]string) (string, error) {
	if releaseVersion := effectiveReleaseVersionFromAssetURL(assetURL, version); releaseVersion != "" {
		return releaseVersion, nil
	}
	return renderTemplateString("{{.Version}}", tool, version, assetName, replacements)
}

func effectiveReleaseVersionFromAssetURL(assetURL, version string) string {
	if releaseVersion := releaseVersionFromGitHubAssetURL(assetURL); releaseVersion != "" {
		return releaseVersion
	}
	parsed, err := url.Parse(assetURL)
	if err != nil {
		return ""
	}
	target := strings.TrimPrefix(version, versionPrefixV)
	for _, part := range strings.Split(strings.Trim(parsed.Path, "/"), "/") {
		if strings.TrimPrefix(part, versionPrefixV) == target {
			return part
		}
		if prefixed := versionPrefixV + target; strings.Contains(part, prefixed) {
			return prefixed
		}
	}
	return ""
}

func releaseVersionFromGitHubAssetURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host != "github.com" {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i, part := range parts {
		if part == "download" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func replaceVersionSegmentInURL(rawURL, version, effectiveVersion string) string {
	if effectiveVersion == "" || effectiveVersion == version {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return rawURL
	}
	target := strings.TrimPrefix(version, versionPrefixV)
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	changed := false
	for i, part := range parts {
		if strings.TrimPrefix(part, versionPrefixV) == target {
			parts[i] = effectiveVersion
			changed = true
		}
	}
	if !changed {
		return rawURL
	}
	parsed.Path = "/" + strings.Join(parts, "/")
	return parsed.String()
}

func alignSidecarURLWithAssetURL(rawURL, assetURL, version string) string {
	effectiveVersion := effectiveReleaseVersionFromAssetURL(assetURL, version)
	aligned := replaceVersionSegmentInURL(rawURL, version, effectiveVersion)
	if aligned != rawURL || effectiveVersion == "" || effectiveVersion == version {
		return aligned
	}
	return replaceEmbeddedAssetVersionInSidecarURL(rawURL, assetURL, version, effectiveVersion)
}

func replaceEmbeddedAssetVersionInSidecarURL(rawURL, assetURL, version, effectiveVersion string) string {
	parsedSidecar, err := url.Parse(rawURL)
	if err != nil || parsedSidecar.Host == "" {
		return rawURL
	}
	parsedAsset, err := url.Parse(assetURL)
	if err != nil || parsedAsset.Host != parsedSidecar.Host {
		return rawURL
	}
	effectiveAssetBase := path.Base(parsedAsset.Path)
	requestedAssetBase := strings.Replace(effectiveAssetBase, effectiveVersion, version, 1)
	sidecarBase := path.Base(parsedSidecar.Path)
	if requestedAssetBase == effectiveAssetBase || !strings.Contains(sidecarBase, requestedAssetBase) {
		return rawURL
	}
	parts := strings.Split(parsedSidecar.Path, "/")
	parts[len(parts)-1] = strings.Replace(sidecarBase, requestedAssetBase, effectiveAssetBase, 1)
	parsedSidecar.Path = strings.Join(parts, "/")
	return parsedSidecar.String()
}

func digestFile(filePath, algorithm string) (string, error) {
	h, err := newHash(algorithm)
	if err != nil {
		return "", err
	}
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func newHash(algorithm string) (hash.Hash, error) {
	switch strings.ToLower(algorithm) {
	case "md5":
		return md5.New(), nil //nolint:gosec // md5 is supported only for Aqua checksum compatibility.
	case "sha1":
		return sha1.New(), nil //nolint:gosec // sha1 is supported only for Aqua checksum compatibility.
	case "sha256":
		return sha256.New(), nil
	case "sha512":
		return sha512.New(), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, algorithm)
	}
}

func expectedChecksum(data []byte, assetName, algorithm string, checksum *registry.ChecksumConfig) (string, error) {
	if checksum.FileFormat == "raw" {
		return parseRawChecksum(data, algorithm)
	}
	return parseRegexpChecksum(data, assetName, algorithm, checksum.Pattern)
}

func parseRawChecksum(data []byte, algorithm string) (string, error) {
	re := checksumRegex(algorithm)
	match := re.FindString(string(data))
	if match == "" {
		return "", ErrChecksumNotFound
	}
	return match, nil
}

func parseRegexpChecksum(data []byte, assetName, algorithm string, pattern registry.ChecksumPattern) (string, error) {
	checksumPattern := pattern.Checksum
	if checksumPattern == "" {
		checksumPattern = checksumRegex(algorithm).String()
	}
	checksumRE, err := regexp.Compile(checksumPattern)
	if err != nil {
		return "", err
	}

	filePattern := pattern.File
	var fileRE *regexp.Regexp
	if filePattern != "" {
		fileRE, err = regexp.Compile(filePattern)
		if err != nil {
			return "", err
		}
	}

	var singleMatch string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		value, exact, ok := matchChecksumLine(scanner.Text(), assetName, checksumRE, fileRE)
		if !ok {
			continue
		}
		if exact {
			return value, nil
		}
		singleMatch = value
	}
	if scannerErr := scanner.Err(); scannerErr != nil {
		return "", scannerErr
	}
	if singleMatch != "" {
		return singleMatch, nil
	}
	return "", fmt.Errorf("%w: %s", ErrChecksumNotFound, assetName)
}

func matchChecksumLine(line, assetName string, checksumRE, fileRE *regexp.Regexp) (string, bool, bool) {
	match := checksumRE.FindStringSubmatch(line)
	if len(match) == 0 {
		return "", false, false
	}
	value := match[0]
	if len(match) > 1 {
		value = match[1]
	}
	if fileRE != nil {
		return matchChecksumLineWithFilePattern(line, assetName, value, fileRE)
	}
	fields := strings.Fields(line)
	if len(fields) == 0 || (!strings.Contains(line, assetName) && fields[0] != value) {
		return "", false, false
	}
	return value, strings.Contains(line, assetName), true
}

func matchChecksumLineWithFilePattern(line, assetName, value string, fileRE *regexp.Regexp) (string, bool, bool) {
	fileMatch := fileRE.FindStringSubmatch(line)
	if len(fileMatch) == 0 {
		return "", false, false
	}
	fileName := fileMatch[len(fileMatch)-1]
	if fileName == assetName || path.Base(fileName) == assetName {
		return value, true, true
	}
	return "", false, false
}

const (
	md5HexLength    = 32
	sha1HexLength   = 40
	sha256HexLength = 64
	sha512HexLength = 128
)

func checksumRegex(algorithm string) *regexp.Regexp {
	length := map[string]int{
		"md5":    md5HexLength,
		"sha1":   sha1HexLength,
		"sha256": sha256HexLength,
		"sha512": sha512HexLength,
	}[strings.ToLower(algorithm)]
	if length == 0 {
		length = sha256HexLength
	}
	return regexp.MustCompile(fmt.Sprintf(`\b[A-Fa-f0-9]{%d}\b`, length))
}

func assetNameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Path == "" {
		return path.Base(rawURL)
	}
	return path.Base(parsed.Path)
}
