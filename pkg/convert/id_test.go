package convert

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

// TestMakeId tests basic functionality of MakeId function.
func TestMakeId(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		wantLen  int
		checkHex bool
	}{
		{
			name:     "simple string",
			input:    []byte("hello"),
			wantLen:  40, // SHA1 produces 40 hex characters.
			checkHex: true,
		},
		{
			name:     "another string",
			input:    []byte("world"),
			wantLen:  40,
			checkHex: true,
		},
		{
			name:     "numeric string",
			input:    []byte("12345"),
			wantLen:  40,
			checkHex: true,
		},
		{
			name:     "single character",
			input:    []byte("a"),
			wantLen:  40,
			checkHex: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MakeId(tt.input)

			// Check length.
			if len(got) != tt.wantLen {
				t.Errorf("MakeId() length = %v, want %v", len(got), tt.wantLen)
			}

			// Check if it's valid hexadecimal.
			if tt.checkHex {
				if _, err := hex.DecodeString(got); err != nil {
					t.Errorf("MakeId() returned invalid hex: %v", err)
				}
			}

			// Check if it's lowercase.
			if got != strings.ToLower(got) {
				t.Errorf("MakeId() should return lowercase hex, got %v", got)
			}
		})
	}
}

// TestMakeId_EdgeCases tests edge cases.
func TestMakeId_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string // Known SHA1 values for edge cases.
	}{
		{
			name:  "empty input",
			input: []byte{},
			want:  "da39a3ee5e6b4b0d3255bfef95601890afd80709", // SHA1 of empty string.
		},
		{
			name:  "nil input",
			input: nil,
			want:  "da39a3ee5e6b4b0d3255bfef95601890afd80709", // SHA1 of nil is same as empty.
		},
		{
			name:  "single byte zero",
			input: []byte{0},
			want:  "5ba93c9db0cff93f52b521d7420e43f6eda2784f",
		},
		{
			name:  "single byte max",
			input: []byte{255},
			want:  "85e53271e14006f0265921d02d4d736cdc580b0b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MakeId(tt.input)
			if got != tt.want {
				t.Errorf("MakeId() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMakeId_SpecialCharacters tests special characters and binary data.
func TestMakeId_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{
			name:  "unicode string",
			input: []byte("Hello, 世界! 🌍"),
		},
		{
			name:  "null bytes",
			input: []byte{0, 1, 2, 0, 3, 0},
		},
		{
			name:  "non-printable characters",
			input: []byte{0x01, 0x02, 0x03, 0x04, 0x05},
		},
		{
			name:  "mixed content",
			input: append([]byte("text"), 0, 255, 128),
		},
		{
			name:  "newlines and tabs",
			input: []byte("line1\nline2\ttab"),
		},
		{
			name:  "special symbols",
			input: []byte("!@#$%^&*()_+-=[]{}|;':\",./<>?"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MakeId(tt.input)

			// Verify it produces a valid SHA1 hash.
			if len(got) != 40 {
				t.Errorf("MakeId() length = %v, want 40", len(got))
			}

			if _, err := hex.DecodeString(got); err != nil {
				t.Errorf("MakeId() produced invalid hex: %v", err)
			}
		})
	}
}

// TestMakeId_Consistency tests that the function is deterministic.
func TestMakeId_Consistency(t *testing.T) {
	input := []byte("test consistency")

	// Generate hash multiple times.
	hash1 := MakeId(input)
	hash2 := MakeId(input)
	hash3 := MakeId(input)

	// All should be identical.
	if hash1 != hash2 || hash2 != hash3 {
		t.Errorf("MakeId() is not consistent: got %v, %v, %v", hash1, hash2, hash3)
	}

	// Different input should produce different hash.
	differentInput := []byte("different input")
	hash4 := MakeId(differentInput)

	if hash1 == hash4 {
		t.Errorf("MakeId() produced same hash for different inputs")
	}
}

// TestMakeId_KnownValues tests against known SHA1 values.
func TestMakeId_KnownValues(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{
			name:  "empty string",
			input: []byte(""),
			want:  "da39a3ee5e6b4b0d3255bfef95601890afd80709",
		},
		{
			name:  "hello world",
			input: []byte("hello world"),
			want:  "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed",
		},
		{
			name:  "The quick brown fox jumps over the lazy dog",
			input: []byte("The quick brown fox jumps over the lazy dog"),
			want:  "2fd4e1c67a2d28fced849ee1bb76e7391b93eb12",
		},
		{
			name:  "abc",
			input: []byte("abc"),
			want:  "a9993e364706816aba3e25717850c26c9cd0d89d",
		},
		{
			name:  "sha1 test vector",
			input: []byte("SHA1 is a cryptographic hash function"),
			want:  "241607185eeaba8029c50755b83d9a5b30bd1ade",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MakeId(tt.input)
			if got != tt.want {
				t.Errorf("MakeId() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMakeId_LargeInputs tests with large inputs.
func TestMakeId_LargeInputs(t *testing.T) {
	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
	}

	for _, sz := range sizes {
		t.Run(sz.name, func(t *testing.T) {
			// Create input of specified size.
			input := make([]byte, sz.size)
			for i := range input {
				input[i] = byte(i % 256)
			}

			got := MakeId(input)

			// Verify it produces valid output.
			if len(got) != 40 {
				t.Errorf("MakeId() with %v input: length = %v, want 40", sz.name, len(got))
			}

			// Verify consistency with large input.
			got2 := MakeId(input)
			if got != got2 {
				t.Errorf("MakeId() not consistent with %v input", sz.name)
			}
		})
	}
}

// TestMakeId_Parallel tests thread safety.
func TestMakeId_Parallel(t *testing.T) {
	// Run parallel tests to ensure thread safety.
	inputs := [][]byte{
		[]byte("test1"),
		[]byte("test2"),
		[]byte("test3"),
		[]byte("test4"),
		[]byte("test5"),
	}

	results := make(map[string]string)

	// First, get expected results.
	for _, input := range inputs {
		key := string(input)
		results[key] = MakeId(input)
	}

	// Now run in parallel and verify.
	t.Run("parallel execution", func(t *testing.T) {
		for _, input := range inputs {
			input := input // capture range variable.
			t.Run(string(input), func(t *testing.T) {
				t.Parallel()

				// Run multiple times in parallel.
				for i := 0; i < 100; i++ {
					got := MakeId(input)
					expected := results[string(input)]
					if got != expected {
						t.Errorf("MakeId() in parallel = %v, want %v", got, expected)
					}
				}
			})
		}
	})
}

// ExampleMakeId demonstrates how to use MakeId function.
func ExampleMakeId() {
	// Generate a stable ID from a resource identifier.
	resourceID := []byte("user:12345:session:67890")
	id := MakeId(resourceID)

	fmt.Printf("Stable ID: %s (length: %d)\n", id[:8]+"...", len(id))
	// Output: Stable ID: c41212f6... (length: 40)
}

// BenchmarkMakeId benchmarks the MakeId function with various input sizes.
func BenchmarkMakeId(b *testing.B) {
	benchmarks := []struct {
		name string
		size int
	}{
		{"Small_10B", 10},
		{"Medium_1KB", 1024},
		{"Large_10KB", 10 * 1024},
		{"VeryLarge_1MB", 1024 * 1024},
	}

	for _, bm := range benchmarks {
		input := make([]byte, bm.size)
		for i := range input {
			input[i] = byte(i % 256)
		}

		b.Run(bm.name, func(b *testing.B) {
			b.SetBytes(int64(bm.size))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = MakeId(input)
			}
		})
	}
}

// BenchmarkMakeId_Parallel benchmarks parallel execution.
func BenchmarkMakeId_Parallel(b *testing.B) {
	input := []byte("benchmark parallel test input")

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = MakeId(input)
		}
	})
}

// TestMakeId_Properties tests various properties of the hash function.
func TestMakeId_Properties(t *testing.T) {
	t.Run("output format", func(t *testing.T) {
		result := MakeId([]byte("test"))

		// Should be exactly 40 characters (SHA1 in hex).
		if len(result) != 40 {
			t.Errorf("Expected length 40, got %d", len(result))
		}

		// Should only contain valid hex characters.
		for _, c := range result {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("Invalid hex character: %c", c)
			}
		}
	})

	t.Run("deterministic output", func(t *testing.T) {
		input := []byte("deterministic test")
		results := make([]string, 10)

		for i := 0; i < 10; i++ {
			results[i] = MakeId(input)
		}

		// All results should be identical.
		for i := 1; i < 10; i++ {
			if results[i] != results[0] {
				t.Errorf("Non-deterministic output detected")
			}
		}
	})

	t.Run("collision resistance", func(t *testing.T) {
		// While SHA1 has known collision vulnerabilities,
		// for our test inputs they should all produce unique hashes.
		hashes := make(map[string][]byte)

		testInputs := [][]byte{
			[]byte("test1"),
			[]byte("test2"),
			[]byte("test3"),
			[]byte("1test"),
			[]byte("2test"),
			[]byte("3test"),
			{0x01, 0x02},
			{0x02, 0x01},
			bytes.Repeat([]byte("a"), 100),
			bytes.Repeat([]byte("b"), 100),
		}

		for _, input := range testInputs {
			hash := MakeId(input)
			if existing, found := hashes[hash]; found {
				if !bytes.Equal(existing, input) {
					t.Errorf("Hash collision detected: %v and %v both produce %v",
						existing, input, hash)
				}
			}
			hashes[hash] = input
		}
	})
}
