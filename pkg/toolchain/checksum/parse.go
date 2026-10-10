package checksum

import (
	"bufio"
	"bytes"
	"path"
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// sha256HexLen is the length of a hex-encoded SHA-256 digest.
const sha256HexLen = 64

// bsdLineRE matches BSD-style `SHA256 (<filename>) = <hash>` lines.
// Tolerates `SHA-256` and other tag variants by relaxing the prefix.
var bsdLineRE = regexp.MustCompile(`^[A-Za-z0-9-]+\s*\(\s*(.+?)\s*\)\s*=\s*([0-9a-fA-F]+)\s*$`)

// ParseChecksumsFile parses a checksum manifest and returns a filename → lowercase-hex-hash map.
// Auto-detects format per line (so mixed manifests round-trip) and supports:
//   - GNU `sha256sum` output: `<hash>  <filename>` or `<hash> *<filename>`
//   - BSD `shasum` output:    `SHA256 (<filename>) = <hash>`
//   - Bare hash (single line): stored under the empty-string key so single-asset manifests work.
//
// Comment lines (leading `#`) and blank lines are skipped. Malformed lines are skipped, not
// errored — registries in the wild sometimes prepend banner text or include signature lines
// alongside the hashes. Returns ErrEmptyChecksumFile if no entries parse.
func ParseChecksumsFile(data []byte) (map[string]string, error) {
	defer perf.Track(nil, "checksum.ParseChecksumsFile")()

	entries := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	// Bump buffer size — some manifests have very long lines (e.g. cosign bundle entries).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		filename, hash, ok := parseLine(line)
		if !ok {
			continue
		}
		entries[filename] = strings.ToLower(hash)
	}

	if err := scanner.Err(); err != nil {
		// Scanner only errors on token-too-large or I/O — both are diagnostic-worthy.
		// Return whatever we parsed so callers can decide whether partial data is useful.
		if len(entries) == 0 {
			return nil, err
		}
	}

	if len(entries) == 0 {
		return nil, ErrEmptyChecksumFile
	}
	return entries, nil
}

// parseLine tries each known format in order and returns (filename, hash, ok).
// Filename is the empty string for bare-hash lines; callers use lookupAsset to handle that case.
func parseLine(line string) (string, string, bool) {
	if filename, hash, ok := parseBSDLine(line); ok {
		return filename, hash, true
	}
	if filename, hash, ok := parseGNULine(line); ok {
		return filename, hash, true
	}
	if hash, ok := parseBareHash(line); ok {
		return "", hash, true
	}
	return "", "", false
}

// parseBSDLine handles `SHA256 (filename) = hash`.
func parseBSDLine(line string) (string, string, bool) {
	m := bsdLineRE.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	hash := m[2]
	if len(hash) != sha256HexLen {
		return "", "", false
	}
	return m[1], hash, true
}

// parseGNULine handles `<hash>  <filename>` or `<hash> *<filename>` (the `*` marks binary mode in
// sha256sum output and must be stripped from the filename).
func parseGNULine(line string) (string, string, bool) {
	// Split into hash + remainder. GNU uses two spaces; some tools use one. Accept either.
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", "", false
	}
	hash := fields[0]
	if len(hash) != sha256HexLen || !isHex(hash) {
		return "", "", false
	}
	// Re-derive the filename by trimming the hash + leading whitespace, so filenames containing
	// spaces survive intact.
	rest := strings.TrimLeft(strings.TrimPrefix(line, hash), " \t")
	rest = strings.TrimPrefix(rest, "*") // Binary-mode marker.
	if rest == "" {
		return "", "", false
	}
	return rest, hash, true
}

// parseBareHash handles a single-line manifest containing only `<hash>` (and optionally a newline).
// Common for tools that distribute a `.sha256` sidecar containing just the digest.
func parseBareHash(line string) (string, bool) {
	if len(line) != sha256HexLen || !isHex(line) {
		return "", false
	}
	return line, true
}

// isHex reports whether s is composed entirely of hex digits.
func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// lookupAsset finds the hash for assetFilename in entries. Tries exact match first, then
// basename match (registries sometimes record paths like `dist/kubectl_linux_amd64.tar.gz` while
// callers pass the bare filename), and finally falls back to a single-entry manifest's bare-hash
// key (the empty string).
func lookupAsset(entries map[string]string, assetFilename string) (string, bool) {
	if hash, ok := entries[assetFilename]; ok {
		return hash, true
	}
	// Try basename match against every entry — handles `./` prefixes and path-qualified entries.
	target := path.Base(assetFilename)
	for name, hash := range entries {
		if path.Base(name) == target {
			return hash, true
		}
	}
	// Single-asset manifest (bare hash) — only safe to return if there's exactly one entry.
	if len(entries) == 1 {
		if hash, ok := entries[""]; ok {
			return hash, true
		}
	}
	return "", false
}
