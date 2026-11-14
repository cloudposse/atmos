package s3

import (
	"fmt"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// objectSet names what a payload consists of, independent of where it lives: the payload file
// paths (empty for an archive) and the distribution document file names.
type objectSet struct {
	files []string
	docs  []string
}

// setOfRecord derives the object set from a verified Record.
func setOfRecord(record *artifact.Record) objectSet {
	var set objectSet
	for index := range record.Inventory {
		if record.Inventory[index].Type == artifact.EntryFile {
			set.files = append(set.files, record.Inventory[index].Path)
		}
	}
	for index := range record.Documents {
		set.docs = append(set.docs, layout.DocumentFileName(record.Documents[index].Role))
	}
	return set
}

// setOfDescriptors derives the object set from the descriptors a representation or journal holds.
// Payload descriptors of per-file packaging are titled with their path.
func setOfDescriptors(payload, documents []ocispec.Descriptor) objectSet {
	var set objectSet
	for index := range payload {
		if title := payload[index].Annotations[ocispec.AnnotationTitle]; title != "" {
			set.files = append(set.files, title)
		}
	}
	for index := range documents {
		if title := documents[index].Annotations[ocispec.AnnotationTitle]; title != "" {
			set.docs = append(set.docs, title)
		}
	}
	return set
}

// keys lists the object keys of a payload at a location, including the adjacent metadata. Every
// key is validated, so a tampered journal can never address a key outside its location.
func (s objectSet) keys(packaging artifact.Packaging, location string) ([]string, error) {
	var keys []string
	if isArchive(packaging) {
		keys = append(keys, location, layout.MetadataSidecarPath(location))
		for _, doc := range s.docs {
			keys = append(keys, layout.DocsSidecarDir(location)+"/"+doc)
		}
	} else {
		for _, file := range s.files {
			keys = append(keys, location+"/"+file)
		}
		keys = append(keys, layout.DistributionPath(location, layout.ManifestName))
		for _, doc := range s.docs {
			keys = append(keys, layout.DistributionPath(location, doc))
		}
	}
	for _, key := range keys {
		if err := blob.ValidateKey(key); err != nil || layout.InManagedNamespace(key) {
			return nil, fmt.Errorf("%w: payload object key %q is not a valid location-relative key", errUtils.ErrArtifactUnsafePath, key)
		}
	}
	return keys, nil
}

// claimPaths lists the paths a payload at a location owns, for collision detection. An archive owns
// the object, its metadata sidecar, and its documents directory; per-file payloads own the
// location prefix.
func claimPaths(packaging artifact.Packaging, location string) []string {
	if isArchive(packaging) {
		return []string{location, layout.MetadataSidecarPath(location), layout.DocsSidecarDir(location)}
	}
	return []string{location}
}
