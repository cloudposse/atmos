//go:build mage

package main

import (
	"context"
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
	s3DeployManifestName        = ".cloudposse-deploy-manifest-v1.json"
	s3DeployManifestVersion     = 1
	s3DeleteBatchSize           = 1000
	s3ManifestContentType       = "application/json; charset=utf-8"
	s3ErrorWithValueFormat      = "%w: %s"
	s3ManifestNotFoundErrorCode = "NoSuchKey"
)

var (
	errS3DeployInvalidLocalDir  = errors.New("mage: s3 deploy local directory does not exist")
	errS3DeployInvalidURI       = errors.New("mage: invalid S3 URI")
	errS3DeployAWSOperation     = errors.New("mage: S3 deploy AWS operation failed")
	errS3DeployInvalidManifest  = errors.New("mage: invalid S3 deployment manifest")
	errS3DeployUnsupportedState = errors.New("mage: unsupported S3 deployment manifest version")
	errS3DeployPartialDelete    = errors.New("mage: S3 failed to delete one or more objects")
	errS3DeployMissingMetadata  = errors.New("mage: S3 deploy manifest is missing file metadata")
	errS3DeployUnsupportedFile  = errors.New("mage: S3 deploy source contains a non-regular file")
)

type s3DeployFile struct {
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	Protected   bool   `json:"protected,omitempty"`
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

func (l s3DeployLocation) objectKey(relative string) string {
	if l.Prefix == "" {
		return relative
	}
	return l.Prefix + "/" + relative
}

func (l s3DeployLocation) listPrefix() string {
	if l.Prefix == "" {
		return ""
	}
	return l.Prefix + "/"
}

func (l s3DeployLocation) relativeKey(key string) (string, bool) {
	prefix := l.listPrefix()
	if prefix == "" {
		return key, true
	}
	if !strings.HasPrefix(key, prefix) {
		return "", false
	}
	return strings.TrimPrefix(key, prefix), true
}

type s3ProtectedPattern struct {
	Raw  string
	Glob glob.Glob
}

type s3Deployer struct {
	client s3DeployClient
}

type s3DeployState struct {
	localDir  string
	location  s3DeployLocation
	manifest  s3DeployManifest
	previous  *s3DeployManifest
	protected []s3ProtectedPattern
}

func newS3Deployer(client s3DeployClient) *s3Deployer {
	return &s3Deployer{client: client}
}

// Deploy publishes a static site with explicit metadata while avoiding writes
// for objects whose content and Content-Type have not changed. Protected paths
// are read from the newline-separated PROTECTED_PATTERNS environment variable.
func (S3) Deploy(ctx context.Context, localDir, s3URI string) error {
	protected, err := compileS3ProtectedPatterns(os.Getenv("PROTECTED_PATTERNS"))
	if err != nil {
		return err
	}
	state, err := prepareS3DeployState(localDir, s3URI, protected)
	if err != nil {
		return err
	}
	client, err := loadS3DeployClient(ctx)
	if err != nil {
		return err
	}
	return newS3Deployer(client).deploy(ctx, state)
}

func (d *s3Deployer) Deploy(ctx context.Context, localDir, s3URI string, protected []s3ProtectedPattern) error {
	state, err := prepareS3DeployState(localDir, s3URI, protected)
	if err != nil {
		return err
	}
	return d.deploy(ctx, state)
}

func (d *s3Deployer) deploy(ctx context.Context, state *s3DeployState) error {
	fmt.Printf("Managed files: %d\n", len(state.manifest.Files))
	previous, err := d.loadManifest(ctx, state.location)
	if err != nil {
		return err
	}
	state.previous = previous

	if state.previous == nil {
		if err := d.bootstrap(ctx, state); err != nil {
			return err
		}
		if err := d.uploadManifest(ctx, state.location, state.manifest); err != nil {
			return err
		}
		fmt.Printf("Bootstrapped %d managed objects.\n", len(state.manifest.Files))
		return nil
	}
	return d.deployIncremental(ctx, state)
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
	return &s3DeployState{
		localDir:  localDir,
		location:  location,
		manifest:  manifest,
		protected: protected,
	}, nil
}

func (d *s3Deployer) deployIncremental(ctx context.Context, state *s3DeployState) error {
	retainProtectedS3DeployFiles(*state.previous, &state.manifest, state.protected)
	changed, deleted := diffS3DeployManifests(*state.previous, state.manifest, state.protected)
	fmt.Printf("Changed/new: %d; deleted: %d\n", len(changed), len(deleted))
	if len(changed) == 0 && len(deleted) == 0 {
		fmt.Println("No content changes; zero S3 writes required.")
		return nil
	}

	if err := d.uploadChanged(ctx, state.localDir, state.location, changed, state.manifest); err != nil {
		return err
	}
	if err := d.deleteRemoved(ctx, state.location, deleted); err != nil {
		return err
	}
	return d.uploadManifest(ctx, state.location, state.manifest)
}

func compileS3ProtectedPatterns(value string) ([]s3ProtectedPattern, error) {
	patterns := make([]s3ProtectedPattern, 0)
	for _, line := range strings.Split(value, "\n") {
		pattern := strings.TrimSpace(line)
		if pattern == "" {
			continue
		}
		// No separator is supplied intentionally, so '*' matches across '/'.
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

func retainProtectedS3DeployFiles(
	previous s3DeployManifest,
	current *s3DeployManifest,
	protected []s3ProtectedPattern,
) {
	for path, metadata := range previous.Files {
		if _, exists := current.Files[path]; !exists && matchesS3ProtectedPath(path, protected) {
			current.Files[path] = metadata
		}
	}
}

func marshalS3DeployManifest(manifest s3DeployManifest) ([]byte, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("mage: encode S3 deploy manifest: %w", err)
	}
	return append(data, '\n'), nil
}
