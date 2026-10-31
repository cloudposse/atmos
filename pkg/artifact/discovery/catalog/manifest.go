package catalog

import (
	"fmt"
	"sort"
	"strings"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Kind is the manifest kind of an artifact catalog.
const Kind = "AtmosArtifactCatalog"

// sourceScheme prefixes the source of every catalog entry.
const sourceScheme = "oci://"

// File is the standard Atmos manifest envelope of a catalog.
type File = manifest.Manifest[Spec]

// Spec holds the discoverable artifacts. Catalog inclusion never establishes trust.
type Spec struct {
	// Entries are the listed artifact names, sorted lexically.
	Entries []Entry `yaml:"entries" json:"entries"`
}

// Entry lists the versions of one artifact name. It is bound to the repository path the artifact
// lives at, never to credentials.
type Entry struct {
	// Name is the logical artifact name, for example "components/terraform/vpc".
	Name string `yaml:"name" json:"name"`

	// Kind is the artifact kind.
	Kind string `yaml:"kind" json:"kind"`

	// Source is the OCI repository path of the artifact, for example "oci://ghcr.io/acme/atmos/components/terraform/vpc".
	Source string `yaml:"source" json:"source"`

	// Description is the description of the newest listed version.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Versions are the listed versions, sorted lexically. Version labels are opaque.
	Versions []Version `yaml:"versions" json:"versions"`
}

// Version pins a listing to immutable digests and carries the published-version metadata that
// label filters apply to.
type Version struct {
	// Version is the opaque version label.
	Version string `yaml:"version" json:"version"`

	// ResolvedDigest is the OCI manifest digest the version resolved to when it was listed.
	ResolvedDigest string `yaml:"resolved_digest" json:"resolved_digest"`

	// RecordDigest is the digest of the canonical Record.
	RecordDigest string `yaml:"record_digest" json:"record_digest"`

	// ContentDigest is the portable content digest from the Record.
	ContentDigest string `yaml:"content_digest" json:"content_digest"`

	// Labels are the labels published with this version.
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`

	// Documents are the README and CHANGELOG descriptors; catalogs never carry the Markdown.
	Documents []Document `yaml:"documents,omitempty" json:"documents,omitempty"`
}

// Document is a README or CHANGELOG descriptor.
type Document struct {
	// Role is "readme" or "changelog".
	Role string `yaml:"role" json:"role"`

	// SourcePath is the document path relative to the publish source.
	SourcePath string `yaml:"source_path" json:"source_path"`

	// MediaType is the document media type.
	MediaType string `yaml:"media_type" json:"media_type"`

	// Size is the document size in bytes.
	Size int64 `yaml:"size" json:"size"`

	// Digest is the SHA-256 of the document bytes.
	Digest string `yaml:"digest" json:"digest"`
}

func init() { manifest.MustRegister(Kind, manifest.DefaultAPIVersion, &Spec{}) }

// New creates an empty catalog manifest.
func New(name string) *File {
	defer perf.Track(nil, "catalog.New")()

	return &File{APIVersion: manifest.DefaultAPIVersion, Kind: Kind, Metadata: manifest.Metadata{Name: name}, Spec: Spec{Entries: []Entry{}}}
}

// Parse decodes and validates catalog JSON or YAML without contacting any repository. It rejects
// duplicate names or versions, malformed digests, and oversized input, and returns the entries and
// versions in lexical order.
func Parse(raw []byte) (*File, error) {
	defer perf.Track(nil, "catalog.Parse")()

	if int64(len(raw)) > artifact.MaxRecordSize {
		return nil, fmt.Errorf("%w: the catalog is %d bytes; the limit is %d", errUtils.ErrArtifactTooLarge, len(raw), artifact.MaxRecordSize)
	}
	file, err := manifest.Load[Spec](Kind, raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrArtifactCatalogInvalid, err)
	}
	if err := check(file); err != nil {
		return nil, err
	}
	Normalize(file)
	return file, nil
}

// check validates the content of a catalog manifest.
func check(file *File) error {
	if file.Metadata.Name == "" {
		return invalid("the catalog needs a metadata name")
	}
	seen := make(map[string]bool, len(file.Spec.Entries))
	for index := range file.Spec.Entries {
		entry := &file.Spec.Entries[index]
		if err := artifact.ValidateName(entry.Name); err != nil {
			return fmt.Errorf("%w: entry %q: %w", errUtils.ErrArtifactCatalogInvalid, entry.Name, err)
		}
		if seen[entry.Name] {
			return invalid("entry %q is listed twice", entry.Name)
		}
		seen[entry.Name] = true
		if err := checkEntry(entry); err != nil {
			return err
		}
	}
	return nil
}

func checkEntry(entry *Entry) error {
	if err := artifact.ValidateKind(entry.Kind); err != nil {
		return fmt.Errorf("%w: entry %q: %w", errUtils.ErrArtifactCatalogInvalid, entry.Name, err)
	}
	if !strings.HasPrefix(entry.Source, sourceScheme) || strings.ContainsAny(entry.Source, "@?#") || strings.Contains(strings.TrimPrefix(entry.Source, sourceScheme), "://") {
		return invalid("entry %q has source %q; a source is an oci://<registry>/<path> repository path without credentials, tag, or digest", entry.Name, entry.Source)
	}
	if len(entry.Versions) == 0 {
		return invalid("entry %q lists no versions", entry.Name)
	}
	versions := make(map[string]bool, len(entry.Versions))
	for index := range entry.Versions {
		version := &entry.Versions[index]
		if err := checkVersion(entry.Name, version); err != nil {
			return err
		}
		if versions[version.Version] {
			return invalid("entry %q lists version %q twice", entry.Name, version.Version)
		}
		versions[version.Version] = true
	}
	return nil
}

func checkVersion(name string, version *Version) error {
	if err := artifact.ValidateVersion(version.Version); err != nil {
		return fmt.Errorf("%w: entry %q: %w", errUtils.ErrArtifactCatalogInvalid, name, err)
	}
	for _, value := range []string{version.ResolvedDigest, version.RecordDigest, version.ContentDigest} {
		if err := artifact.ValidateDigest(digest.Digest(value)); err != nil {
			return fmt.Errorf("%w: entry %q version %q: %w", errUtils.ErrArtifactCatalogInvalid, name, version.Version, err)
		}
	}
	for key := range version.Labels {
		if key == "" {
			return invalid("entry %q version %q has an empty label key", name, version.Version)
		}
	}
	return checkDocuments(name, version)
}

// checkDocuments validates the README and CHANGELOG descriptors of a version.
func checkDocuments(name string, version *Version) error {
	roles := make(map[string]bool, len(version.Documents))
	for index := range version.Documents {
		doc := &version.Documents[index]
		if doc.Role != string(artifact.DocumentReadme) && doc.Role != string(artifact.DocumentChangelog) || roles[doc.Role] {
			return invalid("entry %q version %q has an invalid or repeated document role %q", name, version.Version, doc.Role)
		}
		roles[doc.Role] = true
		if err := artifact.ValidateDigest(digest.Digest(doc.Digest)); err != nil {
			return fmt.Errorf("%w: entry %q version %q %s: %w", errUtils.ErrArtifactCatalogInvalid, name, version.Version, doc.Role, err)
		}
	}
	return nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errUtils.ErrArtifactCatalogInvalid, fmt.Sprintf(format, args...))
}

// Normalize sorts the entries by name, the versions by version label, and the documents by role.
// Sorting is lexical: version labels are opaque and carry no SemVer ordering.
func Normalize(file *File) {
	defer perf.Track(nil, "catalog.Normalize")()

	sort.Slice(file.Spec.Entries, func(i, j int) bool { return file.Spec.Entries[i].Name < file.Spec.Entries[j].Name })
	for index := range file.Spec.Entries {
		versions := file.Spec.Entries[index].Versions
		sort.Slice(versions, func(i, j int) bool { return versions[i].Version < versions[j].Version })
		for position := range versions {
			documents := versions[position].Documents
			sort.Slice(documents, func(i, j int) bool { return documents[i].Role < documents[j].Role })
		}
	}
}

// Source returns the catalog source of an artifact name below a repository location.
func Source(location, name string) string {
	defer perf.Track(nil, "catalog.Source")()

	return sourceScheme + strings.ToLower(location) + "/" + name
}

// Validate requires every entry to be bound to the repository path of the destination repository:
// its source must be exactly the repository location joined with the entry name. A catalog that
// names another registry, another prefix, or another path is rejected, so catalog data can never
// direct reads, or the credentials bound to the destination repository, anywhere else.
func Validate(file *File, location string) error {
	defer perf.Track(nil, "catalog.Validate")()

	if err := check(file); err != nil {
		return err
	}
	for index := range file.Spec.Entries {
		entry := &file.Spec.Entries[index]
		if want := Source(location, entry.Name); entry.Source != want {
			return invalid("entry %q is bound to %q, not to the destination repository path %q", entry.Name, entry.Source, want)
		}
	}
	return nil
}

// Upsert lists a version, creating its entry when needed. An existing version is replaced, which
// reports whether it differed. The entry keeps the kind recorded first; a different kind for the
// same name is a conflict.
func Upsert(file *File, entry *Entry, version *Version) (replaced bool, err error) {
	defer perf.Track(nil, "catalog.Upsert")()

	for index := range file.Spec.Entries {
		existing := &file.Spec.Entries[index]
		if existing.Name != entry.Name {
			continue
		}
		if existing.Source != entry.Source || existing.Kind != entry.Kind {
			return false, fmt.Errorf("%w: %q is already listed from %s as kind %q", errUtils.ErrArtifactCatalogConflict, entry.Name, existing.Source, existing.Kind)
		}
		existing.Description = entry.Description
		for position := range existing.Versions {
			if existing.Versions[position].Version == version.Version {
				differs := existing.Versions[position].ResolvedDigest != version.ResolvedDigest
				existing.Versions[position] = *version
				return differs, nil
			}
		}
		existing.Versions = append(existing.Versions, *version)
		return false, nil
	}
	added := *entry
	added.Versions = []Version{*version}
	file.Spec.Entries = append(file.Spec.Entries, added)
	return false, nil
}

// Unlist removes one version, and the entry when it was the last. It never touches artifact content.
func Unlist(file *File, name, version string) error {
	defer perf.Track(nil, "catalog.Unlist")()

	for index := range file.Spec.Entries {
		entry := &file.Spec.Entries[index]
		if entry.Name != name {
			continue
		}
		for position := range entry.Versions {
			if entry.Versions[position].Version != version {
				continue
			}
			entry.Versions = append(entry.Versions[:position], entry.Versions[position+1:]...)
			if len(entry.Versions) == 0 {
				file.Spec.Entries = append(file.Spec.Entries[:index], file.Spec.Entries[index+1:]...)
			}
			return nil
		}
	}
	return fmt.Errorf("%w: %s:%s is not listed in the catalog", errUtils.ErrArtifactNotFound, name, version)
}

// ListEntries converts a validated catalog into list entries. Each entry's pinned reference is bound
// to the destination repository path and carries the listed digests; the listing grants no trust,
// and every later read resolves the reference through the repository again.
func ListEntries(file *File, location, alias string) []artifact.ListEntry {
	defer perf.Track(nil, "catalog.ListEntries")()

	var entries []artifact.ListEntry
	for index := range file.Spec.Entries {
		entry := &file.Spec.Entries[index]
		for position := range entry.Versions {
			version := &entry.Versions[position]
			entries = append(entries, artifact.ListEntry{
				Pinned: artifact.PinnedReference{
					Ref:            artifact.Ref{Name: entry.Name, Version: version.Version},
					Repository:     strings.ToLower(location) + "/" + entry.Name,
					ResolvedDigest: digest.Digest(version.ResolvedDigest),
					RecordDigest:   digest.Digest(version.RecordDigest),
					ContentDigest:  digest.Digest(version.ContentDigest),
				},
				Name: entry.Name, Version: version.Version, Kind: entry.Kind, Description: entry.Description,
				Labels: copyLabels(version.Labels), Documents: artifactDocuments(version.Documents), Repository: alias,
			})
		}
	}
	return entries
}

func copyLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		out[key] = value
	}
	return out
}

func artifactDocuments(documents []Document) []artifact.Document {
	var out []artifact.Document
	for index := range documents {
		doc := &documents[index]
		out = append(out, artifact.Document{
			Role: artifact.DocumentRole(doc.Role), SourcePath: doc.SourcePath, MediaType: doc.MediaType,
			Size: doc.Size, Digest: digest.Digest(doc.Digest),
		})
	}
	return out
}

// FromInspection builds the entry and version a published artifact contributes to a catalog. It
// reads only the inspection's Record and pinned digests: metadata, never payload.
func FromInspection(location string, inspection *artifact.Inspection) (Entry, Version, error) {
	defer perf.Track(nil, "catalog.FromInspection")()

	record := inspection.Record
	if record == nil {
		return Entry{}, Version{}, invalid("%s is not an Atmos artifact package and cannot be listed", inspection.Pinned.Ref)
	}
	if inspection.Pinned.RecordDigest == "" {
		return Entry{}, Version{}, invalid("%s has no Record digest", inspection.Pinned.Ref)
	}
	entry := Entry{Name: record.Name, Kind: record.Kind, Source: Source(location, record.Name), Description: record.Metadata.Description}
	version := Version{
		Version:        record.Version,
		ResolvedDigest: inspection.Pinned.ResolvedDigest.String(),
		RecordDigest:   inspection.Pinned.RecordDigest.String(),
		ContentDigest:  record.ContentDigest.String(),
		Labels:         copyLabels(record.Metadata.Labels),
	}
	for index := range record.Documents {
		doc := &record.Documents[index]
		version.Documents = append(version.Documents, Document{
			Role: string(doc.Role), SourcePath: doc.SourcePath, MediaType: doc.MediaType, Size: doc.Size, Digest: doc.Digest.String(),
		})
	}
	return entry, version, nil
}
