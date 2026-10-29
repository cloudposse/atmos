package oci

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	maxBundleEntries = 4096
	manifestFileName = "plugin.yaml"
	schemasDirectory = "schemas"
)

var (
	forbiddenSuffixes = []string{".tfstate", ".tfvars", ".tfvars.json", ".pem", ".key", ".p12", ".pfx"}
	forbiddenFragment = []string{".tfstate.", "credentials", "secret"}
	forbiddenPrefixes = []string{"id_rsa", "id_ed25519", "id_ecdsa", "id_dsa"}
	forbiddenNames    = map[string]bool{"terraform.rc": true, ".terraformrc": true}
)

// BundleFile describes one file inside a published platform payload.
type BundleFile struct {
	Platform string `json:"platform,omitempty" yaml:"platform,omitempty"`
	Path     string `json:"path" yaml:"path"`
	Size     int64  `json:"size" yaml:"size"`
	Mode     string `json:"mode" yaml:"mode"`
}

// CheckBundlePath rejects any path that must never ship inside a plugin release:
// hidden files and directories (.git, .terraform, .atmos, .env, .aws, .ssh, ...),
// credential, key, and state files, and anything that is not a canonical relative
// slash path. The same rule applies when packing, publishing, and extracting.
func CheckBundlePath(path string) error {
	defer perf.Track(nil, "pluginoci.CheckBundlePath")()

	reason := forbiddenPathReason(path)
	if reason == "" {
		return nil
	}
	return errUtils.Build(fmt.Errorf("%w: %q (%s)", errUtils.ErrPluginBundlePathForbidden, path, reason)).
		WithHint("Plugin releases contain only plugin.yaml, schemas/**/*.json, and the compiled executable. Remove or rename the file, or point the manifest at a regular, non-hidden path that is not credential- or state-like.").
		Err()
}

func forbiddenPathReason(path string) string {
	switch {
	case path == "":
		return "empty path"
	case strings.ContainsAny(path, "\\\x00"):
		return "backslash or NUL in path"
	case strings.Contains(path, ":"):
		return "drive-qualified path or alternate data stream"
	case strings.HasPrefix(path, "/"):
		return "absolute path"
	case pathpkg.Clean(path) != path || path == ".":
		return "path is not a canonical relative path"
	}
	for _, segment := range strings.Split(path, "/") {
		if reason := forbiddenSegmentReason(segment); reason != "" {
			return reason
		}
	}
	return ""
}

func forbiddenSegmentReason(segment string) string {
	lower := strings.ToLower(segment)
	if segment == ".." || strings.HasPrefix(segment, ".") {
		return "hidden or parent path segment " + segment
	}
	if forbiddenNames[lower] {
		return "Terraform CLI configuration file"
	}
	for _, suffix := range forbiddenSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return "state, variable, or key material (" + suffix + ")"
		}
	}
	for _, prefix := range forbiddenPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return "SSH private key name"
		}
	}
	for _, fragment := range forbiddenFragment {
		if strings.Contains(lower, fragment) {
			return "credential-like name (" + strings.Trim(fragment, ".") + ")"
		}
	}
	return ""
}

// checkDeclaredPaths guards every executable path a manifest declares.
func checkDeclaredPaths(executables ...string) error {
	for _, executable := range executables {
		if err := CheckBundlePath(executable); err != nil {
			return err
		}
	}
	return nil
}

func forbiddenEntry(path, kind string) error {
	return errUtils.Build(fmt.Errorf("%w: %q is a %s; only regular files are allowed", errUtils.ErrPluginBundlePathForbidden, path, kind)).
		WithHint("Plugin releases may only contain regular files; replace links, directories, and special files with real files.").
		Err()
}

// bundleRules selects the checks scanBundle applies on top of the path guard.
type bundleRules struct {
	// executable is the declared tar path of the plugin executable.
	executable string
	// exactSet restricts entries to plugin.yaml, schemas/**/*.json, and the executable.
	exactSet bool
	// platform and checkFormat verify the executable's native format.
	platform    *v1.Platform
	checkFormat bool
}

// scanBundle streams a bundle tar once, enforcing the path guard, regular-file
// entries, uniqueness, and size bounds. Nothing is written beside an optional
// temporary spool of the executable that is removed before returning.
func scanBundle(reader io.Reader, rules *bundleRules) ([]BundleFile, error) {
	archive := tar.NewReader(reader)
	seen := make(map[string]bool)
	var files []BundleFile
	var total int64
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.Join(errUtils.ErrOCIIntegrity, err)
		}
		// Validate at the archive boundary before exposing names to inventory consumers.
		if !filepath.IsLocal(header.Name) {
			return nil, fmt.Errorf("%w: %q is not a local path", errUtils.ErrPluginBundlePathForbidden, header.Name)
		}
		if err := checkEntry(header, rules, seen); err != nil {
			return nil, err
		}
		total += header.Size
		if len(files) >= maxBundleEntries || total > maxBundleSize {
			return nil, errUtils.ErrPluginPayloadTooLarge
		}
		if err := checkBundleExecutable(archive, header, rules); err != nil {
			return nil, err
		}

		files = append(files, BundleFile{Path: header.Name, Size: header.Size, Mode: fmt.Sprintf("%04o", header.Mode&0o7777)})
	}
	if rules.exactSet {
		if err := requireEntries(seen, rules.executable); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func checkEntry(header *tar.Header, rules *bundleRules, seen map[string]bool) error {
	switch header.Typeflag {
	case tar.TypeReg:
	case tar.TypeSymlink, tar.TypeLink:
		return forbiddenEntry(header.Name, "link")
	case tar.TypeDir:
		return forbiddenEntry(header.Name, "directory")
	default:
		return forbiddenEntry(header.Name, "special entry")
	}
	if err := CheckBundlePath(header.Name); err != nil {
		return err
	}
	key := strings.ToLower(header.Name)
	if seen[key] || header.Size < 0 {
		return errUtils.ErrOCIIntegrity
	}
	seen[key] = true
	if rules.exactSet && !expectedEntry(header.Name, rules.executable) {
		return errUtils.Build(fmt.Errorf("%w: %q is not part of a plugin release", errUtils.ErrPluginBundlePathForbidden, header.Name)).
			WithHint("A plugin release contains exactly plugin.yaml, schemas/**/*.json, and the declared executable. Re-pack the plugin with `atmos plugin pack`.").
			Err()
	}
	return nil
}

func expectedEntry(path, executable string) bool {
	return path == manifestFileName || path == executable || (strings.HasPrefix(path, schemasDirectory+"/") && strings.HasSuffix(path, ".json"))
}

func requireEntries(seen map[string]bool, executable string) error {
	for _, required := range []string{manifestFileName, strings.ToLower(executable)} {
		if !seen[required] {
			return fmt.Errorf("%w: payload is missing %s", errUtils.ErrOCIIntegrity, required)
		}
	}
	return nil
}

// checkSpooledExecutable copies one tar entry to a temporary file so the
// debug/* parsers can seek, without holding the executable in memory.
func checkSpooledExecutable(entry io.Reader, size int64, platform *v1.Platform) error {
	spool, err := os.CreateTemp("", "atmos-plugin-executable-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = spool.Close()
		_ = os.Remove(spool.Name())
	}()
	if written, err := io.Copy(spool, io.LimitReader(entry, size)); err != nil || written != size {
		return errors.Join(errUtils.ErrOCIIntegrity, err)
	}
	return checkExecutableFormat(spool, platform)
}

// ignoreNotExist converts a missing file into a nil error.
func ignoreNotExist(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func checkBundleExecutable(reader io.Reader, header *tar.Header, rules *bundleRules) error {
	if rules.checkFormat && header.Name == rules.executable {
		return checkSpooledExecutable(reader, header.Size, rules.platform)
	}
	return nil
}
