package artifact

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Options is a typed, validated view of one repository kind's `options` map. The configuration
// schema keeps `options` a generic map so it never depends on a backend; each kind decodes it into
// its own struct with [DecodeOptions] and checks it with Validate. Validate never performs
// network access, credential resolution, or directory creation.
type Options interface {
	// Validate checks required options and value shapes.
	Validate() error
}

// PackagingValidator is implemented by Options that depend on the effective packaging, for example
// to check that a key_pattern extension agrees with an archive format.
type PackagingValidator interface {
	// ValidateForPackaging checks the options against the effective packaging.
	ValidateForPackaging(packaging Packaging) error
}

// Option keys shared by several kinds.
const (
	// OptionPath is the local destination root of local/dir, local/archive, and oci/layout.
	OptionPath = "path"

	// OptionKeyPattern overrides the path below the stable root for local and S3 payloads.
	OptionKeyPattern = "key_pattern"
)

// S3 object tag limits from the S3 documentation.
const (
	maxS3Tags       = 10
	maxS3TagKeyLen  = 128
	maxS3TagValLen  = 256
	reservedTagHead = "aws:"
)

var (
	// Flags option keys that look like credentials. Credentials never belong
	// in repository options; the `identity` field references an Atmos auth identity instead.
	credentialOptionPattern = regexp.MustCompile(`(?i)secret|password|passwd|token|credential|access[_-]?key|api[_-]?key`)

	// The S3 general-purpose bucket naming rule (3-63 lowercase letters, digits, dots, hyphens).
	s3BucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

	// The shape of a cloud region name such as us-east-2.
	regionPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

	// A registry host[:port] without a scheme, path, or credentials.
	registryHostPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?(:[0-9]{1,5})?$`)

	// Matches one Go template action.
	templateActionPattern = regexp.MustCompile(`(?s)\{\{.*?\}\}`)

	// Matches Go template string literals so field references are not read from them.
	templateStringPattern = regexp.MustCompile("\"(?:[^\"\\\\]|\\\\.)*\"|`[^`]*`")

	// Matches a field reference such as .Name.
	templateFieldPattern = regexp.MustCompile(`\.([A-Za-z_][A-Za-z0-9_]*)`)
)

// Path segments that would escape or empty a path.
const (
	currentDirSegment = "."
	parentDirSegment  = ".."
)

// keyPatternFields are the fields a key_pattern may reference.
var keyPatternFields = map[string]bool{"Name": true, "Version": true, "Kind": true, "CreatedAt": true}

// LocalDirOptions are the options of a local/dir repository.
type LocalDirOptions struct {
	// Path is the destination root directory.
	Path string `mapstructure:"path"`

	// KeyPattern overrides the payload directory path below the root.
	KeyPattern string `mapstructure:"key_pattern"`
}

// Validate implements Options.
func (o *LocalDirOptions) Validate() error {
	defer perf.Track(nil, "artifact.LocalDirOptions.Validate")()

	return errors.Join(validateLocalPath(o.Path), validateKeyPattern(o.KeyPattern, ""))
}

// ValidateForPackaging implements PackagingValidator.
func (o *LocalDirOptions) ValidateForPackaging(packaging Packaging) error {
	defer perf.Track(nil, "artifact.LocalDirOptions.ValidateForPackaging")()

	return validateKeyPattern(o.KeyPattern, packaging)
}

// LocalArchiveOptions are the options of a local/archive repository.
type LocalArchiveOptions struct {
	// Path is the destination root directory.
	Path string `mapstructure:"path"`

	// KeyPattern overrides the archive file path below the root.
	KeyPattern string `mapstructure:"key_pattern"`
}

// Validate implements Options.
func (o *LocalArchiveOptions) Validate() error {
	defer perf.Track(nil, "artifact.LocalArchiveOptions.Validate")()

	return errors.Join(validateLocalPath(o.Path), validateKeyPattern(o.KeyPattern, ""))
}

// ValidateForPackaging implements PackagingValidator.
func (o *LocalArchiveOptions) ValidateForPackaging(packaging Packaging) error {
	defer perf.Track(nil, "artifact.LocalArchiveOptions.ValidateForPackaging")()

	return validateKeyPattern(o.KeyPattern, packaging)
}

// S3Options are the options of an aws/s3 repository.
type S3Options struct {
	// Bucket is the S3 bucket name.
	Bucket string `mapstructure:"bucket"`

	// Prefix is the collection root below the bucket.
	Prefix string `mapstructure:"prefix"`

	// Region is the bucket region.
	Region string `mapstructure:"region"`

	// Tags are S3 object tags applied to every object of a publication. They are not artifact labels.
	Tags map[string]string `mapstructure:"tags"`

	// KeyPattern overrides the object key below the prefix.
	KeyPattern string `mapstructure:"key_pattern"`

	// Endpoint overrides the S3 endpoint URL, for example an S3-compatible emulator such as Floci.
	// It never carries credentials; those come from the repository identity.
	Endpoint string `mapstructure:"endpoint"`

	// PathStyle forces path-style (true) or virtual-hosted-style (false) addressing. Nil keeps the
	// default: path-style whenever an endpoint override applies, virtual-hosted otherwise.
	PathStyle *bool `mapstructure:"path_style"`
}

// Validate implements Options.
func (o *S3Options) Validate() error {
	defer perf.Track(nil, "artifact.S3Options.Validate")()

	var errs []error
	if o.Bucket == "" {
		errs = append(errs, fmt.Errorf("%w: option %q is required", errUtils.ErrArtifactConfigInvalid, "bucket"))
	} else if !s3BucketPattern.MatchString(o.Bucket) {
		errs = append(errs, fmt.Errorf("%w: bucket %q is not a valid S3 bucket name", errUtils.ErrArtifactConfigInvalid, o.Bucket))
	}
	if o.Region != "" && !regionPattern.MatchString(o.Region) {
		errs = append(errs, fmt.Errorf("%w: region %q is not a valid region name", errUtils.ErrArtifactConfigInvalid, o.Region))
	}
	errs = append(errs, validatePrefix(o.Prefix), validateS3Tags(o.Tags), validateKeyPattern(o.KeyPattern, ""), validateEndpoint(o.Endpoint))
	return errors.Join(errs...)
}

// validateEndpoint checks an S3 endpoint override: an absolute http or https URL with a host and
// no embedded credentials. It performs no network access.
func validateEndpoint(endpoint string) error {
	if endpoint == "" {
		return nil
	}
	parsed, err := url.Parse(endpoint)
	switch {
	case err != nil:
		return fmt.Errorf("%w: endpoint %q is not a valid URL: %w", errUtils.ErrArtifactConfigInvalid, endpoint, err)
	case parsed.Scheme != "http" && parsed.Scheme != "https":
		return fmt.Errorf("%w: endpoint %q must use the http or https scheme", errUtils.ErrArtifactConfigInvalid, endpoint)
	case parsed.Host == "":
		return fmt.Errorf("%w: endpoint %q has no host", errUtils.ErrArtifactConfigInvalid, endpoint)
	case parsed.User != nil:
		return fmt.Errorf("%w: endpoint must not embed credentials; reference an auth identity with `identity` instead", errUtils.ErrArtifactConfigInvalid)
	}
	return nil
}

// ValidateForPackaging implements PackagingValidator.
func (o *S3Options) ValidateForPackaging(packaging Packaging) error {
	defer perf.Track(nil, "artifact.S3Options.ValidateForPackaging")()

	return validateKeyPattern(o.KeyPattern, packaging)
}

// OCIRegistryOptions are the options of an oci/registry repository. The prefix is an Atmos
// collection-root convention, not an ORAS namespace field: Atmos maps a collection and artifact
// name onto an OCI repository path below it.
type OCIRegistryOptions struct {
	// Registry is the registry host, with an optional port, for example ghcr.io.
	Registry string `mapstructure:"registry"`

	// Prefix is the collection root path below the registry, for example acme/atmos.
	Prefix string `mapstructure:"prefix"`
}

// Validate implements Options.
func (o *OCIRegistryOptions) Validate() error {
	defer perf.Track(nil, "artifact.OCIRegistryOptions.Validate")()

	var errs []error
	if o.Registry == "" {
		errs = append(errs, fmt.Errorf("%w: option %q is required", errUtils.ErrArtifactConfigInvalid, "registry"))
	} else if !registryHostPattern.MatchString(o.Registry) {
		errs = append(errs, fmt.Errorf("%w: registry %q must be a host with an optional port, without a scheme, path, or credentials", errUtils.ErrArtifactConfigInvalid, o.Registry))
	}
	errs = append(errs, validatePrefix(o.Prefix))
	if o.Prefix != "" {
		for _, segment := range strings.Split(o.Prefix, "/") {
			if err := ValidateName(segment); err != nil {
				errs = append(errs, fmt.Errorf("%w: prefix %q is not a valid OCI repository path: %w", errUtils.ErrArtifactConfigInvalid, o.Prefix, err))
				break
			}
		}
	}
	return errors.Join(errs...)
}

// OCILayoutOptions are the options of an oci/layout repository.
type OCILayoutOptions struct {
	// Path is the OCI image layout directory.
	Path string `mapstructure:"path"`
}

// Validate implements Options.
func (o *OCILayoutOptions) Validate() error {
	defer perf.Track(nil, "artifact.OCILayoutOptions.Validate")()

	return validateLocalPath(o.Path)
}

// DecodeOptions decodes a generic options map into a typed options struct. Decoding is strict:
// unknown keys and mistyped values are errors, so a typo such as `buckt` never silently becomes a
// missing option. Keys that look like credentials get a pointer to the `identity` field.
func DecodeOptions(raw map[string]any, out any) error {
	defer perf.Track(nil, "artifact.DecodeOptions")()

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:      out,
		TagName:     "mapstructure",
		ErrorUnused: true,
	})
	if err != nil {
		return fmt.Errorf("%w: options decoder: %w", errUtils.ErrArtifactConfigInvalid, err)
	}
	if err := decoder.Decode(raw); err != nil {
		return fmt.Errorf("%w: options: %w%s", errUtils.ErrArtifactConfigInvalid, err, credentialHint(raw))
	}
	return nil
}

// credentialHint explains where credentials belong when an option key looks like one.
func credentialHint(raw map[string]any) string {
	keys := make([]string, 0, len(raw))
	for key := range raw {
		if credentialOptionPattern.MatchString(key) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	return fmt.Sprintf(" (option %s looks like a credential; credentials never belong in repository options, reference an auth identity with `identity` instead)", strings.Join(keys, ", "))
}

// validateLocalPath checks a local destination root. A relative path is legitimate (it resolves
// against the declaring configuration file), so only emptiness and NUL bytes are rejected here.
func validateLocalPath(path string) error {
	switch {
	case path == "":
		return fmt.Errorf("%w: option %q is required", errUtils.ErrArtifactConfigInvalid, OptionPath)
	case strings.ContainsRune(path, 0):
		return fmt.Errorf("%w: %w: path contains a NUL byte", errUtils.ErrArtifactConfigInvalid, errUtils.ErrArtifactUnsafePath)
	}
	return nil
}

// ValidatePrefix checks a collection-root prefix such as acme/atmos below a bucket or registry. It
// must be a relative, slash-separated path with no empty, "." or ".." segments, so the prefix
// cannot escape its root, and it must not enter the reserved managed-record namespace.
func ValidatePrefix(prefix string) error {
	defer perf.Track(nil, "artifact.ValidatePrefix")()

	return validatePrefix(prefix)
}

func validatePrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	if strings.HasPrefix(prefix, "/") || strings.Contains(prefix, `\`) || strings.ContainsRune(prefix, 0) {
		return fmt.Errorf("%w: %w: prefix %q must be relative and slash-separated", errUtils.ErrArtifactConfigInvalid, errUtils.ErrArtifactUnsafePath, prefix)
	}
	for _, segment := range strings.Split(prefix, "/") {
		switch segment {
		case "", currentDirSegment, parentDirSegment:
			return fmt.Errorf("%w: %w: prefix %q has an empty, %q, or %q segment", errUtils.ErrArtifactConfigInvalid, errUtils.ErrArtifactUnsafePath, prefix, currentDirSegment, parentDirSegment)
		case ReservedNamespace:
			return fmt.Errorf("%w: %w: prefix %q enters the reserved %s namespace", errUtils.ErrArtifactConfigInvalid, errUtils.ErrArtifactUnsafePath, prefix, ReservedNamespace)
		}
	}
	return nil
}

// ValidateKeyPattern statically checks a key_pattern template against the effective packaging
// without expanding it. Expansion needs the artifact name, version, kind, and creation time, so
// only the literal structure is checked here: balanced template delimiters, referenced fields
// limited to .Name, .Version, .Kind and .CreatedAt, no literal root escape or empty segment, no
// entry into the reserved namespace, and an archive extension that agrees with the packaging.
// The pattern is never rendered during configuration loading.
func ValidateKeyPattern(pattern string, packaging Packaging) error {
	defer perf.Track(nil, "artifact.ValidateKeyPattern")()

	return validateKeyPattern(pattern, packaging)
}

func validateKeyPattern(pattern string, packaging Packaging) error {
	if pattern == "" {
		return nil
	}
	if strings.ContainsRune(pattern, 0) || strings.Contains(pattern, `\`) {
		return fmt.Errorf("%w: %w: key_pattern %q must not contain NUL bytes or backslashes", errUtils.ErrArtifactConfigInvalid, errUtils.ErrArtifactUnsafePath, pattern)
	}
	actions := templateActionPattern.FindAllString(pattern, -1)
	literal := templateActionPattern.ReplaceAllString(pattern, "\x01")
	if strings.Contains(literal, "{{") || strings.Contains(literal, "}}") {
		return fmt.Errorf("%w: key_pattern %q has unbalanced template delimiters", errUtils.ErrArtifactConfigInvalid, pattern)
	}
	if err := validateKeyPatternFields(pattern, actions); err != nil {
		return err
	}
	if err := validateKeyPatternSegments(pattern, literal); err != nil {
		return err
	}
	return validateKeyPatternExtension(pattern, literal, packaging)
}

func validateKeyPatternFields(pattern string, actions []string) error {
	for _, action := range actions {
		stripped := templateStringPattern.ReplaceAllString(action, `""`)
		for _, match := range templateFieldPattern.FindAllStringSubmatch(stripped, -1) {
			if !keyPatternFields[match[1]] {
				return fmt.Errorf("%w: key_pattern %q references unknown field .%s (allowed: .Name, .Version, .Kind, .CreatedAt)", errUtils.ErrArtifactConfigInvalid, pattern, match[1])
			}
		}
	}
	return nil
}

func validateKeyPatternSegments(pattern, literal string) error {
	if strings.HasPrefix(literal, "/") {
		return fmt.Errorf("%w: %w: key_pattern %q must be relative to the repository root", errUtils.ErrArtifactConfigInvalid, errUtils.ErrArtifactUnsafePath, pattern)
	}
	for _, segment := range strings.Split(literal, "/") {
		switch segment {
		case "", currentDirSegment, parentDirSegment:
			return fmt.Errorf("%w: %w: key_pattern %q has an empty, %q, or %q segment", errUtils.ErrArtifactConfigInvalid, errUtils.ErrArtifactUnsafePath, pattern, currentDirSegment, parentDirSegment)
		case ReservedNamespace:
			return fmt.Errorf("%w: %w: key_pattern %q enters the reserved %s namespace", errUtils.ErrArtifactConfigInvalid, errUtils.ErrArtifactUnsafePath, pattern, ReservedNamespace)
		}
	}
	return nil
}

// archiveExtensions maps the literal trailing extension of a key_pattern to the packaging it implies.
var archiveExtensions = []struct {
	suffix    string
	packaging Packaging
}{
	{".tar.gz", PackagingTgz},
	{".tgz", PackagingTgz},
	{".tar", PackagingTar},
	{".zip", PackagingZip},
}

func validateKeyPatternExtension(pattern, literal string, packaging Packaging) error {
	// Only a literal tail (after the last template action) has a known extension.
	tail := literal[strings.LastIndex(literal, "\x01")+1:]
	for _, candidate := range archiveExtensions {
		if !strings.HasSuffix(tail, candidate.suffix) {
			continue
		}
		if packaging != "" && candidate.packaging != packaging {
			return fmt.Errorf("%w: key_pattern %q ends in %q, which does not match packaging %q", errUtils.ErrArtifactConfigInvalid, pattern, candidate.suffix, packaging)
		}
		return nil
	}
	return nil
}

func validateS3Tags(tags map[string]string) error {
	if len(tags) > maxS3Tags {
		return fmt.Errorf("%w: %d S3 tags exceed the S3 limit of %d", errUtils.ErrArtifactConfigInvalid, len(tags), maxS3Tags)
	}
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var errs []error
	for _, key := range keys {
		switch {
		case key == "" || len(key) > maxS3TagKeyLen:
			errs = append(errs, fmt.Errorf("%w: S3 tag key %q must be 1-%d characters", errUtils.ErrArtifactConfigInvalid, key, maxS3TagKeyLen))
		case strings.HasPrefix(strings.ToLower(key), reservedTagHead):
			errs = append(errs, fmt.Errorf("%w: S3 tag key %q uses the reserved %q prefix", errUtils.ErrArtifactConfigInvalid, key, reservedTagHead))
		case len(tags[key]) > maxS3TagValLen:
			errs = append(errs, fmt.Errorf("%w: S3 tag %q value exceeds %d characters", errUtils.ErrArtifactConfigInvalid, key, maxS3TagValLen))
		}
	}
	return errors.Join(errs...)
}

// RegistryHostOf returns the host of an oci:// URI without credentials, query, or path, and
// whether the URI is well formed. It is used to bind a catalog to its repository's registry.
func RegistryHostOf(uri string) (string, bool) {
	defer perf.Track(nil, "artifact.RegistryHostOf")()

	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "oci" || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	return strings.ToLower(parsed.Host), true
}
