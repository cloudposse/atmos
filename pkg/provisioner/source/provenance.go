package source

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProvenanceFile is the marker the source provisioner writes inside every shared (non-workdir)
// component directory it populates: <component dir>/.atmos/source.json.
//
// `source delete` only removes directories that carry this marker (or, for workdir targets, the
// workdir metadata the provisioner already writes), so it can never delete a hand-written component
// directory that merely shares a name with a source-provisioned instance. Directories vendored by
// an Atmos version that predates the marker have none; re-run `source pull --force` to add it.
const ProvenanceFile = "source.json"

// provenanceFilePermissions is the mode of the marker file (rw-r--r--).
const provenanceFilePermissions = 0o644

// Provenance records which component instance the source provisioner placed into a directory.
type Provenance struct {
	// Component is the resolved component name that names the directory.
	Component string `json:"component"`
	// Stack is the stack the directory was provisioned for.
	Stack string `json:"stack,omitempty"`
	// Source is the credential-free form of the source URI.
	Source string `json:"source"`
	// Version is the source version, when one was declared separately.
	Version        string `json:"version,omitempty"`
	SourceIdentity string `json:"source_identity,omitempty"`
	SourceKind     string `json:"source_kind,omitempty"`
	// ProvisionedAt is the provisioning time.
	ProvisionedAt time.Time `json:"provisioned_at"`
}

// provenancePath returns the marker path inside a provisioned directory.
func provenancePath(dir string) string {
	return filepath.Join(dir, workdir.AtmosDir, ProvenanceFile)
}

// sharedSourceExpired applies remote-source TTLs to directories provisioned by Atmos.
// Unmarked directories may contain authored files; refresh them explicitly with source pull --force.
func sharedSourceExpired(dir string, spec *schema.VendorComponentSource) (bool, string) {
	if spec.TTL == "" || isLocalSource(spec.Uri) {
		return false, ""
	}
	data, err := os.ReadFile(provenancePath(dir))
	if err != nil {
		return false, ""
	}
	var provenance Provenance
	if err := json.Unmarshal(data, &provenance); err != nil || provenance.ProvisionedAt.IsZero() {
		return false, ""
	}
	return isSourceCacheExpired(spec.TTL, provenance.ProvisionedAt)
}

// WriteProvenance writes the provisioner marker into dir.
func WriteProvenance(dir string, p *Provenance) error {
	defer perf.Track(nil, "source.WriteProvenance")()

	if err := os.MkdirAll(filepath.Join(dir, workdir.AtmosDir), DirPermissions); err != nil {
		return errUtils.Build(errUtils.ErrSourceProvision).
			WithCause(err).
			WithExplanation("Failed to create the source provenance directory").
			WithContext("path", dir).
			Err()
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return errUtils.Build(errUtils.ErrSourceProvision).
			WithCause(err).
			WithExplanation("Failed to encode the source provenance marker").
			Err()
	}
	if err := filesystem.NewOSFileSystem().WriteFileAtomic(provenancePath(dir), data, provenanceFilePermissions); err != nil {
		return errUtils.Build(errUtils.ErrSourceProvision).
			WithCause(err).
			WithExplanation("Failed to write the source provenance marker").
			WithContext("path", provenancePath(dir)).
			Err()
	}
	return nil
}

// HasProvenance reports whether the source provisioner populated dir: it carries either the
// provenance marker or, for workdir targets, the workdir metadata the provisioner writes.
func HasProvenance(dir string) bool {
	defer perf.Track(nil, "source.HasProvenance")()

	if info, err := os.Stat(provenancePath(dir)); err == nil && info.Mode().IsRegular() {
		return true
	}
	metadata, err := workdir.ReadMetadata(dir)
	return err == nil && metadata != nil
}

// recordProvenance marks a freshly provisioned target. Workdir targets get the full workdir
// metadata (which also drives re-provisioning decisions); shared directories get the minimal marker.
func recordProvenance(target *Target, stack string, spec *schema.VendorComponentSource, receipts ...*downloader.ResolvedArtifact) error {
	if target.IsWorkdir {
		instance := target.Instance
		if instance == "" {
			instance = target.Component
		}
		return writeWorkdirMetadata(target.Dir, instance, stack, spec, receipts...)
	}
	p := &Provenance{
		Component:     target.Component,
		Stack:         stack,
		Source:        downloader.RedactSource(spec.Uri),
		Version:       spec.Version,
		ProvisionedAt: time.Now().UTC(),
	}
	if len(receipts) > 0 && receipts[0] != nil {
		p.SourceIdentity, p.SourceKind = receipts[0].Identity, receipts[0].Kind
	}
	return WriteProvenance(target.Dir, p)
}

// withProvenance prepares receipts in staging so source files and provenance share
// the same rollback journal. Existing creation time is carried forward explicitly.
func withProvenance(target *Target, stack string, spec *schema.VendorComponentSource, receipt *downloader.ResolvedArtifact) VendorSourceOption {
	return func(opts *vendorSourceOptions) {
		opts.metadata = func(stage string) error {
			if err := validateTargetOwner(target, stack); err != nil {
				return err
			}
			if target.IsWorkdir {
				if previous, err := workdir.ReadMetadata(target.Dir); err == nil && previous != nil {
					if err := workdir.WriteMetadata(stage, previous); err != nil {
						return err
					}
				}
			}
			stagedTarget := *target
			stagedTarget.Dir = stage
			return recordProvenance(&stagedTarget, stack, spec, receipt)
		}
	}
}

// validateTargetOwner also runs under the publication lock before a new receipt is
// staged, so two instances cannot race to claim one explicit working directory.
func validateTargetOwner(target *Target, stack string) error {
	if !target.EnforceOwner {
		return nil
	}
	metadata, err := workdir.ReadMetadata(target.Dir)
	if err != nil || metadata == nil {
		if !isNonEmptyDir(target.Dir) {
			return nil
		}
		return errUtils.Build(errUtils.ErrWorkdirProvision).WithExplanation("Explicit source working directory has files but no verifiable instance owner").WithContext("path", target.Dir).Err()
	}
	if metadata.Component != target.Instance || metadata.Stack != stack {
		return errUtils.Build(errUtils.ErrWorkdirProvision).WithExplanation("Explicit source working directory belongs to another component instance").WithContext("path", target.Dir).WithContext("owner_component", metadata.Component).WithContext("owner_stack", metadata.Stack).Err()
	}
	return nil
}
