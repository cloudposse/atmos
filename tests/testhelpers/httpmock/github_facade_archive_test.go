package httpmock

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// multiEntryArchiveFiles returns a map with enough entries that Go's randomized map iteration
// order would very likely reorder them across builds if BuildTarGz/BuildZip iterated the map
// directly instead of writing entries in sorted order.
func multiEntryArchiveFiles() map[string]string {
	return map[string]string{
		"zeta/tool":     "zeta content",
		"alpha/tool":    "alpha content",
		"mu/tool":       "mu content",
		"beta/tool.txt": "beta content",
		"gamma/README":  "gamma content",
	}
}

// TestBuildTarGz_Deterministic asserts that building the same multi-entry archive twice
// produces byte-identical output, guarding against map-iteration-order nondeterminism.
func TestBuildTarGz_Deterministic(t *testing.T) {
	files := multiEntryArchiveFiles()

	first, err := BuildTarGz(files)
	require.NoError(t, err)

	second, err := BuildTarGz(files)
	require.NoError(t, err)

	require.Equal(t, first, second, "BuildTarGz must produce identical bytes for identical input")
}

// TestBuildZip_Deterministic asserts that building the same multi-entry archive twice produces
// byte-identical output, guarding against map-iteration-order nondeterminism.
func TestBuildZip_Deterministic(t *testing.T) {
	files := multiEntryArchiveFiles()

	first, err := BuildZip(files)
	require.NoError(t, err)

	second, err := BuildZip(files)
	require.NoError(t, err)

	require.Equal(t, first, second, "BuildZip must produce identical bytes for identical input")
}

// TestSortedArchiveNames asserts the helper returns names in sorted order regardless of the
// map's internal iteration order.
func TestSortedArchiveNames(t *testing.T) {
	files := multiEntryArchiveFiles()

	names := sortedArchiveNames(files)

	require.Equal(t, []string{
		"alpha/tool",
		"beta/tool.txt",
		"gamma/README",
		"mu/tool",
		"zeta/tool",
	}, names)
}
