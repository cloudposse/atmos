package artifact

import (
	"bytes"
	"maps"
	"slices"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Clone returns a Record that shares no maps, slices, or pointers with r, so a caller can keep
// either copy and edit the other without changing the first.
func (r *Record) Clone() Record {
	defer perf.Track(nil, "artifact.Record.Clone")()

	out := *r
	out.Metadata.Labels = maps.Clone(r.Metadata.Labels)
	if r.Metadata.Provenance != nil {
		provenance := *r.Metadata.Provenance
		if provenance.CI != nil {
			ci := *provenance.CI
			provenance.CI = &ci
		}
		if provenance.Source != nil {
			source := *provenance.Source
			provenance.Source = &source
		}
		out.Metadata.Provenance = &provenance
	}
	out.Inventory = slices.Clone(r.Inventory)
	out.Documents = slices.Clone(r.Documents)
	out.KindData = bytes.Clone(r.KindData)
	return out
}

// Clone returns Capabilities that share no slices with c, so the guarantees a repository
// advertises stay static whatever a caller does with the returned value.
func (c *Capabilities) Clone() Capabilities {
	defer perf.Track(nil, "artifact.Capabilities.Clone")()

	out := *c
	out.Packagings = slices.Clone(c.Packagings)
	out.SubjectKinds = slices.Clone(c.SubjectKinds)
	out.EvidenceFormats = slices.Clone(c.EvidenceFormats)
	out.LockScopes = slices.Clone(c.LockScopes)
	return out
}
