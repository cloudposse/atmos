package merge

import (
	"testing"
)

// BenchmarkMergeContext_WithProvenance_Enabled benchmarks MergeContext operations with provenance enabled.
func BenchmarkMergeContext_WithProvenance_Enabled(b *testing.B) {
	ctx := NewMergeContext()
	ctx.EnableProvenance()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := ProvenanceEntry{
			File: "config.yaml",
			Line: i,
			Type: ProvenanceTypeInline,
		}
		ctx.RecordProvenance("vars.name", entry)
	}
}

// BenchmarkMergeContext_WithProvenance_Disabled benchmarks MergeContext operations with provenance disabled.
func BenchmarkMergeContext_WithProvenance_Disabled(b *testing.B) {
	ctx := NewMergeContext()
	// Provenance not enabled - should have zero overhead.

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := ProvenanceEntry{
			File: "config.yaml",
			Line: i,
			Type: ProvenanceTypeInline,
		}
		ctx.RecordProvenance("vars.name", entry)
	}
}

// BenchmarkMergeContext_Clone_WithProvenance benchmarks cloning with provenance enabled.
func BenchmarkMergeContext_Clone_WithProvenance(b *testing.B) {
	ctx := NewMergeContext()
	ctx.EnableProvenance()

	// Populate with some provenance data.
	for i := 0; i < 100; i++ {
		entry := ProvenanceEntry{
			File: "config.yaml",
			Line: i,
			Type: ProvenanceTypeInline,
		}
		ctx.RecordProvenance("vars.name", entry)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ctx.Clone()
	}
}

// BenchmarkMergeContext_Clone_WithoutProvenance benchmarks cloning without provenance.
func BenchmarkMergeContext_Clone_WithoutProvenance(b *testing.B) {
	ctx := NewMergeContext()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ctx.Clone()
	}
}

// BenchmarkProvenanceStorage_Record benchmarks recording provenance.
func BenchmarkProvenanceStorage_Record(b *testing.B) {
	storage := NewProvenanceStorage()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := ProvenanceEntry{
			File: "config.yaml",
			Line: i,
			Type: ProvenanceTypeInline,
		}
		storage.Record("vars.name", entry)
	}
}

// BenchmarkProvenanceStorage_Get benchmarks retrieving provenance.
func BenchmarkProvenanceStorage_Get(b *testing.B) {
	storage := NewProvenanceStorage()

	// Populate with some entries.
	for i := 0; i < 100; i++ {
		entry := ProvenanceEntry{
			File: "config.yaml",
			Line: i,
			Type: ProvenanceTypeInline,
		}
		storage.Record("vars.name", entry)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = storage.Get("vars.name")
	}
}

// BenchmarkProvenanceStorage_Has benchmarks checking provenance existence.
func BenchmarkProvenanceStorage_Has(b *testing.B) {
	storage := NewProvenanceStorage()

	// Populate with some entries.
	for i := 0; i < 100; i++ {
		entry := ProvenanceEntry{
			File: "config.yaml",
			Line: i,
			Type: ProvenanceTypeInline,
		}
		storage.Record("vars.name", entry)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = storage.Has("vars.name")
	}
}

// BenchmarkProvenanceStorage_Clone benchmarks cloning provenance storage.
func BenchmarkProvenanceStorage_Clone(b *testing.B) {
	storage := NewProvenanceStorage()

	// Populate with realistic data.
	paths := []string{"vars.name", "vars.tags", "settings.foo", "settings.bar"}
	for _, path := range paths {
		for i := 0; i < 10; i++ {
			entry := ProvenanceEntry{
				File: "config.yaml",
				Line: i,
				Type: ProvenanceTypeInline,
			}
			storage.Record(path, entry)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = storage.Clone()
	}
}

// BenchmarkHashValue benchmarks the value hashing function.
func BenchmarkHashValue(b *testing.B) {
	value := "test-value-for-hashing"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hashValue(value)
	}
}

// BenchmarkProvenanceEntry_Clone benchmarks cloning provenance entries.
func BenchmarkProvenanceEntry_Clone(b *testing.B) {
	entry := ProvenanceEntry{
		File:      "config.yaml",
		Line:      10,
		Column:    5,
		Type:      ProvenanceTypeInline,
		ValueHash: "abcd1234",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = entry.Clone()
	}
}
