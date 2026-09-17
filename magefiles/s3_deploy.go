//go:build mage

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gobwas/glob"
	"github.com/magefile/mage/mg"
)

// S3 groups content-aware S3 deployment targets.
type S3 mg.Namespace

const (
	s3DeployManifestName    = ".cloudposse-deploy-manifest-v1.json"
	s3DeployManifestVersion = 1
	s3DeleteBatchSize       = 1000
	s3FilePermissions       = 0o600
	s3DirectoryPermissions  = 0o755
	s3CommandService        = "s3"
	s3CommandCopy           = "cp"
	s3OnlyShowErrorsFlag    = "--only-show-errors"
	s3ErrorWithValueFormat  = "%w: %s"
)

var (
	errS3DeployInvalidLocalDir  = errors.New("mage: s3 deploy local directory does not exist")
	errS3DeployInvalidURI       = errors.New("mage: invalid S3 URI")
	errS3DeployAWSCommand       = errors.New("mage: S3 deploy AWS command failed")
	errS3DeployInvalidManifest  = errors.New("mage: invalid S3 deployment manifest")
	errS3DeployUnsupportedState = errors.New("mage: unsupported S3 deployment manifest version")
	errS3DeployUnsupportedGlob  = errors.New("mage: protected S3 pattern uses syntax unsupported by the AWS CLI")
	errS3DeployInvalidDelete    = errors.New("mage: invalid S3 delete response")
	errS3DeployPartialDelete    = errors.New("mage: S3 failed to delete one or more objects")
	errS3DeployMissingMetadata  = errors.New("mage: S3 deploy manifest is missing file metadata")
	errS3DeployUnsupportedFile  = errors.New("mage: S3 deploy source contains a non-regular file")
)

type s3DeployFile struct {
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
}

type s3DeployManifest struct {
	Version int                     `json:"version"`
	Files   map[string]s3DeployFile `json:"files"`
}

type s3DeployLocation struct {
	Bucket string
	Prefix string
	URI    string
}

type s3ProtectedPattern struct {
	Raw  string
	Glob glob.Glob
}

type s3DeployCommandRunner interface {
	Run(args ...string) ([]byte, error)
}

type s3DeployAWSCLI struct{}

func (s3DeployAWSCLI) Run(args ...string) ([]byte, error) {
	cmd := exec.Command("aws", args...) // #nosec G204 -- command is fixed and arguments are controlled by the Mage target.
	return cmd.CombinedOutput()
}

type s3Deployer struct {
	runner s3DeployCommandRunner
}

type s3DeployState struct {
	localDir     string
	location     s3DeployLocation
	manifestPath string
	manifest     s3DeployManifest
	previous     *s3DeployManifest
	protected    []s3ProtectedPattern
	tempDir      string
}

func newS3Deployer(runner s3DeployCommandRunner) *s3Deployer {
	return &s3Deployer{runner: runner}
}

// Deploy publishes a static site with explicit metadata while avoiding writes
// for objects whose content and Content-Type have not changed. Protected paths
// are read from the newline-separated PROTECTED_PATTERNS environment variable.
func (S3) Deploy(localDir, s3URI string) error {
	protected, err := compileS3ProtectedPatterns(os.Getenv("PROTECTED_PATTERNS"))
	if err != nil {
		return err
	}
	return newS3Deployer(s3DeployAWSCLI{}).Deploy(localDir, s3URI, protected)
}

func (d *s3Deployer) Deploy(localDir, s3URI string, protected []s3ProtectedPattern) error {
	state, err := prepareS3DeployState(localDir, s3URI, protected)
	if err != nil {
		return err
	}
	defer os.RemoveAll(state.tempDir)

	fmt.Printf("Managed files: %d\n", len(state.manifest.Files))
	state.previous, err = d.loadManifest(state.location.URI, state.manifestPath)
	if err != nil {
		return err
	}
	if err := writeS3DeployManifest(state.manifestPath, state.manifest); err != nil {
		return err
	}

	if state.previous == nil {
		if err := d.bootstrap(state); err != nil {
			return err
		}
		if err := d.uploadManifest(state.manifestPath, state.location.URI); err != nil {
			return err
		}
		fmt.Printf("Bootstrapped %d managed objects.\n", len(state.manifest.Files))
		return nil
	}
	return d.deployIncremental(state)
}

func prepareS3DeployState(localDir, s3URI string, protected []s3ProtectedPattern) (*s3DeployState, error) {
	localDir, err := filepath.Abs(localDir)
	if err != nil {
		return nil, fmt.Errorf("mage: resolve S3 deploy directory: %w", err)
	}
	info, err := os.Stat(localDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf(s3ErrorWithValueFormat, errS3DeployInvalidLocalDir, localDir)
	}

	location, err := parseS3DeployURI(s3URI)
	if err != nil {
		return nil, err
	}
	manifest, err := buildS3DeployManifest(localDir)
	if err != nil {
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "s3-deploy-")
	if err != nil {
		return nil, fmt.Errorf("mage: create S3 deploy temporary directory: %w", err)
	}
	return &s3DeployState{
		localDir:     localDir,
		location:     location,
		manifestPath: filepath.Join(tempDir, s3DeployManifestName),
		manifest:     manifest,
		protected:    protected,
		tempDir:      tempDir,
	}, nil
}

func (d *s3Deployer) deployIncremental(state *s3DeployState) error {
	changed, deleted := diffS3DeployManifests(*state.previous, state.manifest, state.protected)
	fmt.Printf("Changed/new: %d; deleted: %d\n", len(changed), len(deleted))
	if len(changed) == 0 && len(deleted) == 0 {
		fmt.Println("No content changes; zero S3 writes required.")
		return nil
	}

	if len(changed) > 0 {
		if err := d.uploadChanged(state.localDir, state.location.URI, changed, state.manifest, state.tempDir); err != nil {
			return err
		}
	}
	if len(deleted) > 0 {
		if err := d.deleteRemoved(state.location, deleted, state.tempDir); err != nil {
			return err
		}
	}
	return d.uploadManifest(state.manifestPath, state.location.URI)
}

func compileS3ProtectedPatterns(value string) ([]s3ProtectedPattern, error) {
	patterns := make([]s3ProtectedPattern, 0)
	for _, line := range strings.Split(value, "\n") {
		pattern := strings.TrimSpace(line)
		if pattern == "" {
			continue
		}
		// AWS CLI filters support *, ?, and character classes, but not the
		// gobwas brace-alternation or backslash-escape extensions.
		if strings.ContainsAny(pattern, "{}\\") {
			return nil, fmt.Errorf("%w: %q", errS3DeployUnsupportedGlob, pattern)
		}
		// No separator is supplied intentionally: AWS CLI '*' filters span '/'.
		// That also makes '*' and '**' equivalent in both matchers.
		compiled, err := glob.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("mage: compile protected S3 pattern %q: %w", pattern, err)
		}
		patterns = append(patterns, s3ProtectedPattern{Raw: pattern, Glob: compiled})
	}
	return patterns, nil
}

func parseS3DeployURI(value string) (s3DeployLocation, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "s3" || parsed.Host == "" {
		return s3DeployLocation{}, fmt.Errorf(s3ErrorWithValueFormat, errS3DeployInvalidURI, value)
	}
	prefix := strings.Trim(parsed.Path, "/")
	normalized := "s3://" + parsed.Host + "/"
	if prefix != "" {
		normalized += prefix + "/"
	}
	return s3DeployLocation{Bucket: parsed.Host, Prefix: prefix, URI: normalized}, nil
}

func buildS3DeployManifest(localDir string) (s3DeployManifest, error) {
	manifest := s3DeployManifest{Version: s3DeployManifestVersion, Files: map[string]s3DeployFile{}}
	err := filepath.WalkDir(localDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := validateS3DeployFileMode(path, info.Mode()); err != nil {
			return err
		}
		relative, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == s3DeployManifestName {
			return nil
		}
		metadata, err := s3DeployFileMetadata(path, relative)
		if err != nil {
			return err
		}
		manifest.Files[relative] = metadata
		return nil
	})
	if err != nil {
		return s3DeployManifest{}, fmt.Errorf("mage: build S3 deploy manifest: %w", err)
	}
	return manifest, nil
}

func validateS3DeployFileMode(path string, mode fs.FileMode) error {
	if !mode.IsRegular() {
		return fmt.Errorf(s3ErrorWithValueFormat, errS3DeployUnsupportedFile, path)
	}
	return nil
}

func s3DeployFileMetadata(path, relative string) (s3DeployFile, error) {
	file, err := os.Open(path) // #nosec G304 -- path is constrained to the caller-provided deployment directory.
	if err != nil {
		return s3DeployFile{}, err
	}
	defer file.Close()

	detected, err := mimetype.DetectReader(file)
	if err != nil {
		return s3DeployFile{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return s3DeployFile{}, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return s3DeployFile{}, err
	}
	info, err := file.Stat()
	if err != nil {
		return s3DeployFile{}, err
	}
	return s3DeployFile{
		SHA256:      hex.EncodeToString(hash.Sum(nil)),
		Size:        info.Size(),
		ContentType: s3DeployContentType(relative, detected.String()),
	}, nil
}

func s3DeployContentType(path, detected string) string {
	extension := strings.ToLower(filepath.Ext(path))
	contentType := mime.TypeByExtension(extension)
	if contentType == "" {
		contentType = detected
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	mediaType, parameters, err := mime.ParseMediaType(contentType)
	if err != nil || !s3DeployContentTypeUsesUTF8(mediaType) {
		return contentType
	}
	parameters["charset"] = "utf-8"
	return mime.FormatMediaType(mediaType, parameters)
}

func s3DeployContentTypeUsesUTF8(mediaType string) bool {
	return strings.HasPrefix(mediaType, "text/") ||
		strings.HasSuffix(mediaType, "+json") ||
		strings.HasSuffix(mediaType, "+xml") ||
		strings.Contains(mediaType, "javascript") ||
		mediaType == "application/json" ||
		mediaType == "application/toml" ||
		mediaType == "application/xml" ||
		mediaType == "application/yaml"
}

func diffS3DeployManifests(oldManifest, newManifest s3DeployManifest, protected []s3ProtectedPattern) ([]string, []string) {
	changed := make([]string, 0)
	deleted := make([]string, 0)
	for path, metadata := range newManifest.Files {
		if oldMetadata, ok := oldManifest.Files[path]; !ok || oldMetadata != metadata {
			changed = append(changed, path)
		}
	}
	for path := range oldManifest.Files {
		if _, ok := newManifest.Files[path]; !ok && !matchesS3ProtectedPath(path, protected) {
			deleted = append(deleted, path)
		}
	}
	sort.Strings(changed)
	sort.Strings(deleted)
	return changed, deleted
}

func matchesS3ProtectedPath(path string, patterns []s3ProtectedPattern) bool {
	for _, pattern := range patterns {
		if pattern.Glob.Match(path) {
			return true
		}
	}
	return false
}

func writeS3DeployManifest(path string, manifest s3DeployManifest) error {
	data, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("mage: encode S3 deploy manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, s3FilePermissions); err != nil {
		return fmt.Errorf("mage: write S3 deploy manifest: %w", err)
	}
	return nil
}

func (d *s3Deployer) loadManifest(s3URI, destination string) (*s3DeployManifest, error) {
	output, err := d.runner.Run(s3CommandService, s3CommandCopy, s3URI+s3DeployManifestName, destination, s3OnlyShowErrorsFlag)
	if err != nil {
		message := string(output)
		if strings.Contains(message, "404") || strings.Contains(message, "Not Found") || strings.Contains(message, "NoSuchKey") {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: read manifest: %w: %s", errS3DeployAWSCommand, err, strings.TrimSpace(message))
	}
	data, err := os.ReadFile(destination) // #nosec G304 -- destination is a target-owned temporary path.
	if err != nil {
		return nil, fmt.Errorf("mage: read downloaded S3 deploy manifest: %w", err)
	}
	var manifest s3DeployManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("%w: %w", errS3DeployInvalidManifest, err)
	}
	if manifest.Version != s3DeployManifestVersion {
		return nil, fmt.Errorf("%w: %d", errS3DeployUnsupportedState, manifest.Version)
	}
	if manifest.Files == nil {
		return nil, errS3DeployInvalidManifest
	}
	return &manifest, nil
}

func (d *s3Deployer) bootstrap(state *s3DeployState) error {
	fmt.Println("No remote manifest found; performing the one-time full metadata upload.")
	files := make([]string, 0, len(state.manifest.Files))
	for path := range state.manifest.Files {
		files = append(files, path)
	}
	sort.Strings(files)
	if len(files) > 0 {
		if err := d.uploadChanged(state.localDir, state.location.URI, files, state.manifest, state.tempDir); err != nil {
			return err
		}
	}

	// Every managed file was uploaded above. This size-only sync is solely a
	// deletion reconciliation and therefore cannot re-upload those files.
	args := []string{s3CommandService, "sync", state.localDir, state.location.URI, "--delete", "--size-only"}
	for _, pattern := range state.protected {
		args = append(args, "--exclude", pattern.Raw)
	}
	args = append(args, s3OnlyShowErrorsFlag)
	return d.runAWS(args...)
}

func (d *s3Deployer) uploadChanged(localDir, s3URI string, changed []string, manifest s3DeployManifest, tempDir string) error {
	groups := map[string][]string{}
	for _, relative := range changed {
		metadata, ok := manifest.Files[relative]
		if !ok {
			return fmt.Errorf(s3ErrorWithValueFormat, errS3DeployMissingMetadata, relative)
		}
		groups[metadata.ContentType] = append(groups[metadata.ContentType], relative)
	}
	contentTypes := make([]string, 0, len(groups))
	for contentType := range groups {
		contentTypes = append(contentTypes, contentType)
	}
	sort.Strings(contentTypes)

	for index, contentType := range contentTypes {
		groupDir := filepath.Join(tempDir, fmt.Sprintf("group-%d", index))
		for _, relative := range groups[contentType] {
			source := filepath.Join(localDir, filepath.FromSlash(relative))
			destination := filepath.Join(groupDir, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(destination), s3DirectoryPermissions); err != nil {
				return fmt.Errorf("mage: create S3 deploy staging directory: %w", err)
			}
			if err := linkOrCopyS3DeployFile(source, destination); err != nil {
				return err
			}
		}
		if err := d.runAWS(
			s3CommandService, s3CommandCopy, groupDir, s3URI, "--recursive",
			"--content-type", contentType, s3OnlyShowErrorsFlag,
		); err != nil {
			return err
		}
	}
	return nil
}

func linkOrCopyS3DeployFile(source, destination string) error {
	if err := os.Link(source, destination); err == nil {
		return nil
	}
	input, err := os.Open(source) // #nosec G304 -- source is constrained to the deployment directory.
	if err != nil {
		return fmt.Errorf("mage: open S3 deploy source: %w", err)
	}
	defer input.Close()
	output, err := os.Create(destination) // #nosec G304 -- destination is a target-owned temporary path.
	if err != nil {
		return fmt.Errorf("mage: create S3 deploy staging file: %w", err)
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return fmt.Errorf("mage: copy S3 deploy staging file: %w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("mage: close S3 deploy staging file: %w", err)
	}
	return nil
}

func (d *s3Deployer) uploadManifest(manifestPath, s3URI string) error {
	return d.runAWS(
		s3CommandService, s3CommandCopy, manifestPath, s3URI+s3DeployManifestName,
		"--content-type", "application/json; charset=utf-8", s3OnlyShowErrorsFlag,
	)
}

func (d *s3Deployer) runAWS(args ...string) error {
	_, err := d.runAWSOutput(args...)
	return err
}

func (d *s3Deployer) runAWSOutput(args ...string) ([]byte, error) {
	output, err := d.runner.Run(args...)
	if err != nil {
		return nil, fmt.Errorf("%w: aws %s: %w: %s", errS3DeployAWSCommand, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	if len(output) > 0 {
		fmt.Print(string(output))
	}
	return output, nil
}
