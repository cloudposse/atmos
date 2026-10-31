package catalog_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/catalog"
)

var (
	sha = func(c string) string { return "sha256:" + strings.Repeat(c, 64) }

	// Compile-time guards for the manifest fields these tests configure.
	_ = catalog.Entry{Name: "", Kind: "", Source: "", Description: "", Versions: nil}
	_ = catalog.Version{Version: "", ResolvedDigest: "", RecordDigest: "", ContentDigest: "", Labels: nil, Documents: nil}
	_ = catalog.Document{Role: "", SourcePath: "", MediaType: "", Size: 0, Digest: ""}
)

const location = "ghcr.io/acme/atmos"

func entry(name string, versions ...string) catalog.Entry {
	out := catalog.Entry{Name: name, Kind: "files", Source: catalog.Source(location, name), Description: "about " + name}
	for _, version := range versions {
		out.Versions = append(out.Versions, catalog.Version{Version: version, ResolvedDigest: sha("a"), RecordDigest: sha("b"), ContentDigest: sha("c")})
	}
	return out
}

func file(entries ...catalog.Entry) *catalog.File {
	built := catalog.New("catalog")
	built.Spec.Entries = entries
	return built
}

func marshal(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}

func TestParseValidatesAndSortsLexically(t *testing.T) {
	parsed, err := catalog.Parse(marshal(t, file(entry("zeta", "9", "10", "1.9", "1.10"), entry("alpha", "2"))))
	require.NoError(t, err)
	var names []string
	for _, e := range parsed.Spec.Entries {
		names = append(names, e.Name)
	}
	assert.Equal(t, []string{"alpha", "zeta"}, names)
	var versions []string
	for _, v := range parsed.Spec.Entries[1].Versions {
		versions = append(versions, v.Version)
	}
	assert.Equal(t, []string{"1.10", "1.9", "10", "9"}, versions, "opaque version labels sort lexically; there is no SemVer ordering")
}

func TestParseRejectsInvalidCatalogs(t *testing.T) {
	good := func() *catalog.File { return file(entry("app", "1.0.0")) }
	tests := []struct {
		name   string
		mutate func(*catalog.File)
	}{
		{"missing metadata name", func(f *catalog.File) { f.Metadata.Name = "" }},
		{"invalid entry name", func(f *catalog.File) { f.Spec.Entries[0].Name = "Not Valid" }},
		{"duplicate entry", func(f *catalog.File) { f.Spec.Entries = append(f.Spec.Entries, entry("app", "2.0.0")) }},
		{"invalid kind", func(f *catalog.File) { f.Spec.Entries[0].Kind = "BAD KIND" }},
		{"source without scheme", func(f *catalog.File) { f.Spec.Entries[0].Source = "ghcr.io/acme/atmos/app" }},
		{"source with a digest", func(f *catalog.File) { f.Spec.Entries[0].Source += "@sha256:" + strings.Repeat("a", 64) }},
		{"source with credentials", func(f *catalog.File) { f.Spec.Entries[0].Source = "oci://user:pw@ghcr.io/acme/atmos/app" }},
		{"source with a query", func(f *catalog.File) { f.Spec.Entries[0].Source += "?token=x" }},
		{"nested scheme", func(f *catalog.File) { f.Spec.Entries[0].Source = "oci://https://ghcr.io/acme/atmos/app" }},
		{"no versions", func(f *catalog.File) { f.Spec.Entries[0].Versions = nil }},
		{"duplicate version", func(f *catalog.File) {
			f.Spec.Entries[0].Versions = append(f.Spec.Entries[0].Versions, f.Spec.Entries[0].Versions[0])
		}},
		{"invalid version", func(f *catalog.File) { f.Spec.Entries[0].Versions[0].Version = "-bad" }},
		{"bad resolved digest", func(f *catalog.File) { f.Spec.Entries[0].Versions[0].ResolvedDigest = "sha256:short" }},
		{"missing record digest", func(f *catalog.File) { f.Spec.Entries[0].Versions[0].RecordDigest = "" }},
		{"bad content digest", func(f *catalog.File) { f.Spec.Entries[0].Versions[0].ContentDigest = "nope" }},
		{"empty label key", func(f *catalog.File) { f.Spec.Entries[0].Versions[0].Labels = map[string]string{"": "x"} }},
		{"unknown document role", func(f *catalog.File) {
			f.Spec.Entries[0].Versions[0].Documents = []catalog.Document{{Role: "license", Digest: sha("d")}}
		}},
		{"repeated document role", func(f *catalog.File) {
			doc := catalog.Document{Role: "readme", SourcePath: "README.md", MediaType: "text/markdown", Size: 1, Digest: sha("d")}
			f.Spec.Entries[0].Versions[0].Documents = []catalog.Document{doc, doc}
		}},
		{"bad document digest", func(f *catalog.File) {
			f.Spec.Entries[0].Versions[0].Documents = []catalog.Document{{Role: "readme", Digest: "x"}}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value := good()
			tc.mutate(value)
			_, err := catalog.Parse(marshal(t, value))
			require.ErrorIs(t, err, errUtils.ErrArtifactCatalogInvalid)
		})
	}
	t.Run("wrong kind", func(t *testing.T) {
		value := good()
		value.Kind = "AtmosPluginCatalog"
		_, err := catalog.Parse(marshal(t, value))
		require.ErrorIs(t, err, errUtils.ErrArtifactCatalogInvalid, "the plugin catalog schema is never accepted")
	})
	t.Run("not a manifest", func(t *testing.T) {
		_, err := catalog.Parse([]byte("{"))
		require.ErrorIs(t, err, errUtils.ErrArtifactCatalogInvalid)
	})
	t.Run("oversized", func(t *testing.T) {
		_, err := catalog.Parse([]byte(strings.Repeat(" ", int(artifact.MaxRecordSize)+1)))
		require.ErrorIs(t, err, errUtils.ErrArtifactTooLarge)
	})
}

func TestValidateBindsEntriesToTheDestinationRepositoryPath(t *testing.T) {
	require.NoError(t, catalog.Validate(file(entry("components/vpc", "1.0.0")), location))
	require.NoError(t, catalog.Validate(file(entry("components/vpc", "1.0.0")), strings.ToUpper(location)), "the host compares case-insensitively")
	for name, source := range map[string]string{
		"registry":         "oci://evil.example.com/acme/atmos/components/vpc",
		"prefix":           "oci://ghcr.io/other/components/vpc",
		"name":             "oci://ghcr.io/acme/atmos/components/other",
		"prefix substring": "oci://ghcr.io/acme/atmos-evil/components/vpc",
	} {
		t.Run(name, func(t *testing.T) {
			value := file(entry("components/vpc", "1.0.0"))
			value.Spec.Entries[0].Source = source
			require.ErrorIs(t, catalog.Validate(value, location), errUtils.ErrArtifactCatalogInvalid)
		})
	}
}

func TestUpsertAndUnlist(t *testing.T) {
	value := file()
	e := entry("app", "1.0.0")
	replaced, err := catalog.Upsert(value, &e, &e.Versions[0])
	require.NoError(t, err)
	assert.False(t, replaced)
	assert.Len(t, value.Spec.Entries, 1)

	second := entry("app", "1.1.0")
	replaced, err = catalog.Upsert(value, &second, &second.Versions[0])
	require.NoError(t, err)
	assert.False(t, replaced)
	assert.Len(t, value.Spec.Entries[0].Versions, 2)

	moved := entry("app", "1.0.0")
	moved.Description = "newer description"
	moved.Versions[0].ResolvedDigest = sha("9")
	replaced, err = catalog.Upsert(value, &moved, &moved.Versions[0])
	require.NoError(t, err)
	assert.True(t, replaced, "a version listed at a new digest is reported as replaced")
	assert.Len(t, value.Spec.Entries[0].Versions, 2)
	assert.Equal(t, "newer description", value.Spec.Entries[0].Description)

	same := entry("app", "1.0.0")
	same.Versions[0].ResolvedDigest = sha("9")
	replaced, err = catalog.Upsert(value, &same, &same.Versions[0])
	require.NoError(t, err)
	assert.False(t, replaced, "an identical digest is not a change")

	foreign := entry("app", "2.0.0")
	foreign.Source = "oci://elsewhere.example.com/app"
	_, err = catalog.Upsert(value, &foreign, &foreign.Versions[0])
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogConflict)
	otherKind := entry("app", "2.0.0")
	otherKind.Kind = "scaffold"
	_, err = catalog.Upsert(value, &otherKind, &otherKind.Versions[0])
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogConflict)

	require.NoError(t, catalog.Unlist(value, "app", "1.0.0"))
	assert.Len(t, value.Spec.Entries[0].Versions, 1)
	require.NoError(t, catalog.Unlist(value, "app", "1.1.0"))
	assert.Empty(t, value.Spec.Entries)
	require.ErrorIs(t, catalog.Unlist(value, "app", "1.1.0"), errUtils.ErrArtifactNotFound)
}

func TestListEntriesKeepPinnedDigestsLabelsAndAlias(t *testing.T) {
	e := entry("components/vpc", "1.0.0", "1.1.0")
	e.Versions[0].Labels = map[string]string{"team": "platform"}
	e.Versions[0].Documents = []catalog.Document{{Role: "readme", SourcePath: "README.md", MediaType: "text/markdown", Size: 3, Digest: sha("d")}}
	entries := catalog.ListEntries(file(e), location, "shared")
	require.Len(t, entries, 2)
	first := entries[0]
	assert.Equal(t, "shared", first.Repository)
	assert.Equal(t, artifact.Ref{Name: "components/vpc", Version: "1.0.0"}, first.Pinned.Ref)
	assert.Equal(t, location+"/components/vpc", first.Pinned.Repository)
	assert.Equal(t, digest.Digest(sha("a")), first.Pinned.ResolvedDigest)
	assert.Equal(t, digest.Digest(sha("b")), first.Pinned.RecordDigest)
	assert.Equal(t, digest.Digest(sha("c")), first.Pinned.ContentDigest)
	assert.Equal(t, map[string]string{"team": "platform"}, first.Labels)
	require.Len(t, first.Documents, 1)
	assert.Equal(t, artifact.DocumentReadme, first.Documents[0].Role)
	assert.Equal(t, "files", first.Kind)

	// Isolation in both directions: mutating the result leaves the catalog unchanged and vice versa.
	first.Labels["team"] = "changed"
	assert.Equal(t, "platform", e.Versions[0].Labels["team"])
	e.Versions[0].Labels["extra"] = "x"
	assert.NotContains(t, first.Labels, "extra")
}

func TestFromInspectionRequiresAnAtmosPackage(t *testing.T) {
	_, _, err := catalog.FromInspection(location, &artifact.Inspection{Pinned: artifact.PinnedReference{Ref: artifact.Ref{Name: "img", Version: "1"}}})
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogInvalid, "a foreign image has no Record and cannot be listed")

	record := &artifact.Record{Name: "app", Version: "1.0.0", Kind: "files", ContentDigest: digest.Digest(sha("c")), Metadata: artifact.Metadata{Description: "d"}}
	_, _, err = catalog.FromInspection(location, &artifact.Inspection{Record: record, Pinned: artifact.PinnedReference{ResolvedDigest: digest.Digest(sha("a"))}})
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogInvalid, "a record digest is required")

	entry, version, err := catalog.FromInspection(location, &artifact.Inspection{Record: record, Pinned: artifact.PinnedReference{ResolvedDigest: digest.Digest(sha("a")), RecordDigest: digest.Digest(sha("b"))}})
	require.NoError(t, err)
	assert.Equal(t, catalog.Source(location, "app"), entry.Source)
	assert.Equal(t, sha("c"), version.ContentDigest)
}

func TestParseInventory(t *testing.T) {
	text := func(refs ...string) []byte {
		return marshal(t, map[string]any{"apiVersion": "atmos/v1", "kind": "AtmosArtifactInventory", "metadata": map[string]any{"name": "inv"}, "spec": map[string]any{"refs": refs}})
	}
	_, refs, err := catalog.ParseInventory(text("b/app:1.0.0", "a/app@"+sha("e"), "a/app:2"))
	require.NoError(t, err)
	var got []string
	for _, r := range refs {
		got = append(got, r.String())
	}
	assert.Equal(t, []string{"a/app:2", "a/app@" + sha("e"), "b/app:1.0.0"}, got, "refs are sorted for deterministic builds")

	for name, raw := range map[string][]byte{
		"empty":          text(),
		"no version":     text("app"),
		"duplicate":      text("app:1", "app:1"),
		"invalid name":   text("BAD:1"),
		"wrong kind":     marshal(t, file(entry("app", "1"))),
		"not a manifest": []byte("{"),
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := catalog.ParseInventory(raw)
			require.ErrorIs(t, err, errUtils.ErrArtifactCatalogInvalid)
		})
	}
	_, _, err = catalog.ParseInventory([]byte(strings.Repeat(" ", int(artifact.MaxRecordSize)+1)))
	require.ErrorIs(t, err, errUtils.ErrArtifactTooLarge)
}
