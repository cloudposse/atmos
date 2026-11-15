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
	out.Metadata = r.Metadata.Clone()
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

// Clone returns Metadata that shares no map or pointer with m.
func (m *Metadata) Clone() Metadata {
	defer perf.Track(nil, "artifact.Metadata.Clone")()

	out := *m
	out.Labels = maps.Clone(m.Labels)
	if m.Provenance != nil {
		provenance := *m.Provenance
		if provenance.CI != nil {
			ci := *provenance.CI
			provenance.CI = &ci
		}
		if provenance.Source != nil {
			source := *provenance.Source
			provenance.Source = &source
		}
		out.Provenance = &provenance
	}
	return out
}

// Clone returns a VerificationPolicy that shares no slice with p, so editing the trust sets of a
// description handed to a caller never reaches the configured policy.
func (p *VerificationPolicy) Clone() VerificationPolicy {
	defer perf.Track(nil, "artifact.VerificationPolicy.Clone")()

	out := *p
	out.TrustSets = slices.Clone(p.TrustSets)
	for index := range out.TrustSets {
		out.TrustSets[index].Identities = slices.Clone(out.TrustSets[index].Identities)
		out.TrustSets[index].Keys = slices.Clone(out.TrustSets[index].Keys)
	}
	return out
}
