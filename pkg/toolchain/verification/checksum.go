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

	checksumURL, err := checksumFileURL(req.Tool, req.Version, req.AssetURL, checksum)
	if err != nil {
		return err
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
		return err
	}

	assetName := assetNameFromURL(req.AssetURL)
	expected, err := expectedChecksum(data, assetName, algorithm, checksum)
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

func hasChecksum(checksum *registry.ChecksumConfig) bool {
	if checksum == nil {
		return false
	}
	return checksum.Type != "" || checksum.Asset != "" || checksum.URL != "" || checksum.Enabled != nil
}

func checksumFileURL(tool *registry.Tool, version, assetURL string, checksum *registry.ChecksumConfig) (string, error) {
	assetName := assetNameFromURL(assetURL)
	if checksum.URL != "" {
		return renderTemplateString(checksum.URL, tool, version, assetName, checksum.Replacements)
	}
	checksumAsset, err := renderTemplateString(checksum.Asset, tool, version, assetName, checksum.Replacements)
	if err != nil {
		return "", err
	}
	if checksum.Type == "http" {
		return checksumAsset, nil
	}
	repoOwner := tool.RepoOwner
	repoName := tool.RepoName
	releaseVersion, err := renderTemplateString("{{.Version}}", tool, version, assetName, checksum.Replacements)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", repoOwner, repoName, releaseVersion, checksumAsset), nil
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
