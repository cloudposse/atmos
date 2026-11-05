package artifact

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Frozen canonical byte vectors. These literals are the contract: they were cross-checked with an
// independent JSON canonicalizer and must never be regenerated to hide an incompatible change.
// A change to any value below changes the identity of every previously published artifact.
const (
	// Vector A: files package with every entry type, documents, labels, provenance, escapes, Unicode.
	vectorACanonical = `{"content_digest":"sha256:9bd2edcda28887b6bc03cba0a811460d0abfb4f35633a7cf1eb7908f43ec4fea","created_at":"2026-10-30T12:34:56Z","documents":[{"digest":"sha256:6665bc2f57c4a7045d347313a1566d91561ff2a3c9d17874a462ae73b7eb8593","media_type":"text/markdown","role":"changelog","size":9,"source_path":".atmos/artifact/CHANGELOG.md"},{"digest":"sha256:711a6108ba2ce6ca93dd47d6817f2361db10d8ab6eec89460b2dfc2c325efabe","media_type":"text/markdown","role":"readme","size":5,"source_path":".atmos/artifact/README.md"}],"inventory":[{"mode":0,"path":"link.tf","size":0,"target":"main.tf","type":"symlink"},{"digest":"sha256:0d6e4079e36703ebd37c00722f5891d28b0e2811dc114b129215123adcce3605","mode":420,"path":"main.tf","size":8,"type":"file"},{"mode":493,"path":"scripts","size":0,"type":"directory"},{"digest":"sha256:acba25512100f80b56fc3ccd14c65be55d94800cda77585c5f41a887e398f9be","mode":493,"path":"scripts/run.sh","size":12,"type":"file"},{"digest":"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","mode":420,"path":"ünï.txt","size":0,"type":"file"}],"kind":"files","metadata":{"description":"VPC <module> & \"friends\"\u0009é","labels":{"env":"prod","team":"platform"},"provenance":{"ci":{"provider":"github","run_id":"42"}}},"name":"components/terraform/vpc","schema_version":1,"version":"1.2.0"}`

	vectorAContent digest.Digest = "sha256:9bd2edcda28887b6bc03cba0a811460d0abfb4f35633a7cf1eb7908f43ec4fea"
	vectorAIntent  digest.Digest = "sha256:c542e50200a6bfd14a5663e89c70e6e5aa97b951e682fb20dac8a71ed1777bee"
	vectorARecord  digest.Digest = "sha256:48797bdf65aa85538095cfa311c57614180796c8200285c10434c32af896e1c3"

	// Vector B: empty package carrying typed kind data (a legacy-planfile-shaped kind).
	vectorBCanonical = `{"content_digest":"sha256:6a5cabe4fc108c5947a6ff9fdb256559d9053edb389ee39cdbbcd97fdf0133cb","created_at":"2026-01-02T03:04:05Z","documents":[],"inventory":[],"kind":"ci/planfile","kind_data":{"component":"vpc","extra":{"a":"","b":0},"key":"k","pr_number":7,"stack":"plat-ue2-dev"},"metadata":{},"name":"ci/planfiles/abc","schema_version":1,"version":"deadbeef"}`

	vectorBContent digest.Digest = "sha256:6a5cabe4fc108c5947a6ff9fdb256559d9053edb389ee39cdbbcd97fdf0133cb"
	vectorBIntent  digest.Digest = "sha256:789b6c268c0f17f8b9744f05c2f309a4ba90a8342be21d4848fb569873fe2702"
	vectorBRecord  digest.Digest = "sha256:56c1a7c24bd4a1c74fc23f6aa0f351edff24e891bd7fa8fba4846593f51c88de"
)

type vectorKindData struct {
	Stack     string `json:"stack"`
	Key       string `json:"key"`
	PRNumber  int    `json:"pr_number"`
	Component string `json:"component"`
	Extra     struct {
		B int    `json:"b"`
		A string `json:"a"`
	} `json:"extra"`
}

// vectorRecordA builds Vector A with inventory in scrambled order and labels inserted in the given order.
func vectorRecordA(t *testing.T, labelOrder []string) Record {
	t.Helper()
	all := map[string]string{"team": "platform", "env": "prod"}
	labels := make(map[string]string, len(all))
	for _, key := range labelOrder {
		labels[key] = all[key]
	}
	record := Record{
		Name:    "components/terraform/vpc",
		Version: "1.2.0",
		Kind:    "files",
		// Sub-second precision must be discarded by the canonical form.
		CreatedAt: NewTimestamp(time.Date(2026, 10, 30, 12, 34, 56, 789000000, time.UTC)),
		Metadata: Metadata{
			Description: "VPC <module> & \"friends\"\té",
			Labels:      labels,
			Provenance:  &Provenance{CI: &CIProvenance{Provider: "github", RunID: "42"}},
		},
		Inventory: []Entry{
			{Path: "scripts/run.sh", Type: EntryFile, Size: 12, Mode: 0o755, Digest: digest.FromString("run")},
			{Path: "main.tf", Type: EntryFile, Size: 8, Mode: 0o644, Digest: digest.FromString("main")},
			{Path: "scripts", Type: EntryDirectory, Mode: 0o755},
			{Path: "link.tf", Type: EntrySymlink, Target: "main.tf"},
			{Path: "ünï.txt", Type: EntryFile, Mode: 0o644, Digest: digest.FromString("")},
		},
		Documents: []Document{
			{Role: DocumentReadme, SourcePath: ".atmos/artifact/README.md", MediaType: DocumentMediaType, Size: 5, Digest: digest.FromString("readme")},
			{Role: DocumentChangelog, SourcePath: ".atmos/artifact/CHANGELOG.md", MediaType: DocumentMediaType, Size: 9, Digest: digest.FromString("changelog")},
		},
	}
	require.NoError(t, record.Seal())
	return record
}

func vectorRecordB(t *testing.T) Record {
	t.Helper()
	data := vectorKindData{Stack: "plat-ue2-dev", Key: "k", PRNumber: 7, Component: "vpc"}
	kind, err := NewKindData(data)
	require.NoError(t, err)
	record := Record{
		Name:      "ci/planfiles/abc",
		Version:   "deadbeef",
		Kind:      "ci/planfile",
		CreatedAt: NewTimestamp(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)),
		KindData:  kind,
	}
	require.NoError(t, record.Seal())
	return record
}

func TestFrozenVectors(t *testing.T) {
	tests := []struct {
		name      string
		build     func(t *testing.T) Record
		canonical string
		content   digest.Digest
		intent    digest.Digest
		record    digest.Digest
	}{
		{
			name:      "A files package, labels env-first",
			build:     func(t *testing.T) Record { return vectorRecordA(t, []string{"env", "team"}) },
			canonical: vectorACanonical, content: vectorAContent, intent: vectorAIntent, record: vectorARecord,
		},
		{
			name:      "A files package, labels team-first",
			build:     func(t *testing.T) Record { return vectorRecordA(t, []string{"team", "env"}) },
			canonical: vectorACanonical, content: vectorAContent, intent: vectorAIntent, record: vectorARecord,
		},
		{
			name:      "B typed kind data",
			build:     vectorRecordB,
			canonical: vectorBCanonical, content: vectorBContent, intent: vectorBIntent, record: vectorBRecord,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := tt.build(t)
			canonical, err := record.Canonical()
			require.NoError(t, err)
			assert.Equal(t, tt.canonical, string(canonical))
			assert.NotEqual(t, byte('\n'), canonical[len(canonical)-1], "no trailing newline")
			assert.Equal(t, tt.content, record.ContentDigest)
			intent, err := record.IntentDigest()
			require.NoError(t, err)
			assert.Equal(t, tt.intent, intent)
			recordDigest, err := record.RecordDigest()
			require.NoError(t, err)
			assert.Equal(t, tt.record, recordDigest)
			assert.Equal(t, digest.FromBytes([]byte(tt.canonical)), recordDigest, "record digest is the SHA-256 of the canonical bytes")
		})
	}
}

func TestFrozenVectorsDecodeRoundTrip(t *testing.T) {
	for name, canonical := range map[string]string{"A": vectorACanonical, "B": vectorBCanonical} {
		t.Run(name, func(t *testing.T) {
			record, err := DecodeRecord([]byte(canonical))
			require.NoError(t, err)
			again, err := record.Canonical()
			require.NoError(t, err)
			assert.Equal(t, canonical, string(again))
		})
	}
	record, err := DecodeRecord([]byte(vectorBCanonical))
	require.NoError(t, err)
	var kind vectorKindData
	require.NoError(t, record.KindData.Decode(&kind))
	assert.Equal(t, 7, kind.PRNumber)
	assert.Equal(t, "plat-ue2-dev", kind.Stack)
}

func TestIntentAndRecordDigestSensitivity(t *testing.T) {
	base := vectorRecordA(t, []string{"env", "team"})
	baseIntent, err := base.IntentDigest()
	require.NoError(t, err)
	baseRecord, err := base.RecordDigest()
	require.NoError(t, err)

	t.Run("created_at and provenance are excluded from intent but not from the record digest", func(t *testing.T) {
		changed := vectorRecordA(t, []string{"env", "team"})
		changed.CreatedAt = NewTimestamp(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC))
		changed.Metadata.Provenance = &Provenance{CI: &CIProvenance{Provider: "github", RunID: "999"}}
		intent, err := changed.IntentDigest()
		require.NoError(t, err)
		assert.Equal(t, baseIntent, intent)
		recordDigest, err := changed.RecordDigest()
		require.NoError(t, err)
		assert.NotEqual(t, baseRecord, recordDigest)
	})

	tests := []struct {
		name   string
		mutate func(r *Record)
	}{
		{"label value", func(r *Record) { r.Metadata.Labels["env"] = "dev" }},
		{"description", func(r *Record) { r.Metadata.Description = "other" }},
		{"kind", func(r *Record) { r.Kind = "component/terraform" }},
		{"version", func(r *Record) { r.Version = "1.2.1" }},
		{"name", func(r *Record) { r.Name = "components/terraform/vpc2" }},
		{"kind data", func(r *Record) {
			data, err := NewKindData(vectorKindData{Stack: "x"})
			require.NoError(t, err)
			r.KindData = data
		}},
		{"file mode changes content and intent", func(r *Record) {
			r.Inventory[1].Mode = 0o600
			require.NoError(t, r.Seal())
		}},
	}
	for _, tt := range tests {
		t.Run("intent changes with "+tt.name, func(t *testing.T) {
			changed := vectorRecordA(t, []string{"env", "team"})
			tt.mutate(&changed)
			if tt.name != "file mode changes content and intent" {
				// Fields other than the inventory do not require resealing the content digest.
				require.NoError(t, changed.Validate())
			}
			intent, err := changed.IntentDigest()
			require.NoError(t, err)
			assert.NotEqual(t, baseIntent, intent)
		})
	}
}

func TestCanonicalDoesNotMutateRecordAndIsIsolated(t *testing.T) {
	record := vectorRecordA(t, []string{"env", "team"})
	firstPath := record.Inventory[0].Path
	first, err := record.Canonical()
	require.NoError(t, err)
	assert.Equal(t, firstPath, record.Inventory[0].Path, "input inventory order is untouched by canonicalization")
	assert.Equal(t, "prod", record.Metadata.Labels["env"])

	// result -> source: scribbling on the returned bytes must not change the record or later results.
	scribbled := bytes.Clone(first)
	for i := range first {
		first[i] = 'x'
	}
	second, err := record.Canonical()
	require.NoError(t, err)
	assert.Equal(t, vectorACanonical, string(second))
	assert.NotEqual(t, string(scribbled), string(first))

	// source -> result: mutating the record afterwards must not affect bytes already returned.
	record.Metadata.Labels["env"] = "changed"
	record.Inventory[0].Path = "zzz"
	require.NoError(t, record.Seal())
	third, err := record.Canonical()
	require.NoError(t, err)
	assert.Equal(t, vectorACanonical, string(second), "earlier result is unaffected by later source mutation")
	assert.NotEqual(t, string(second), string(third))
}

func TestKindDataOrderIndependence(t *testing.T) {
	first, err := NewKindData(struct {
		B int `json:"b"`
		A int `json:"a"`
	}{B: 2, A: 1})
	require.NoError(t, err)
	second, err := NewKindData(struct {
		A int `json:"a"`
		B int `json:"b"`
	}{A: 1, B: 2})
	require.NoError(t, err)
	assert.Equal(t, `{"a":1,"b":2}`, string(first))
	assert.Equal(t, first, second)

	_, err = NewKindData(struct {
		Ratio float64 `json:"ratio"`
	}{Ratio: 0.5})
	require.ErrorIs(t, err, errUtils.ErrArtifactRecordInvalid, "floats are not allowed in the canonical path")
}

func TestDecodeRecordRejectsNonCanonicalBytes(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want error
	}{
		{"trailing newline", vectorBCanonical + "\n", errUtils.ErrArtifactRecordInvalid},
		{"extra whitespace", `{ ` + vectorBCanonical[1:], errUtils.ErrArtifactRecordInvalid},
		{"unknown field", vectorBCanonical[:len(vectorBCanonical)-1] + `,"extra":1}`, errUtils.ErrArtifactRecordInvalid},
		{"wrong schema version", mutateVectorB(`"schema_version":1`, `"schema_version":2`), errUtils.ErrArtifactRecordInvalid},
		{"tampered content digest", mutateVectorB("6a5cabe4", "00000000"), errUtils.ErrArtifactRecordInvalid},
		{"duplicate key", mutateVectorB(`"kind":"ci/planfile"`, `"kind":"ci/planfile","kind":"files"`), errUtils.ErrArtifactRecordInvalid},
		{"sub-second timestamp", mutateVectorB("03:04:05Z", "03:04:05.5Z"), errUtils.ErrArtifactRecordInvalid},
		{"empty input", ``, errUtils.ErrArtifactRecordInvalid},
		{"oversize", string(make([]byte, MaxRecordSize+1)), errUtils.ErrArtifactTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeRecord([]byte(tt.raw))
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.want), "got %v", err)
		})
	}
}

// mutateVectorB returns Vector B with the first occurrence of old replaced.
func mutateVectorB(old, replacement string) string {
	return string(bytes.Replace([]byte(vectorBCanonical), []byte(old), []byte(replacement), 1))
}
