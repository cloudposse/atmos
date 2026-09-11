package autoinit

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDataDir covers precedence between the subprocess-env lookup and the real process
// environment, relative-vs-absolute resolution, and the default fallback.
func TestDataDir(t *testing.T) {
	// filepath.VolumeName is "" on Unix and a drive letter (e.g. "C:") on Windows; prefixing
	// with it lets the "absolute value from lookup" case below build a path filepath.IsAbs
	// recognizes as absolute on every platform -- a bare `\abs\data` is NOT absolute on
	// Windows (it's rooted but drive-relative), unlike on Unix where a leading separator
	// alone is sufficient.
	absPath := filepath.Join(filepath.VolumeName(t.TempDir())+string(filepath.Separator), "abs", "data")

	tests := []struct {
		name          string
		componentPath string
		lookup        func(key string) string
		osEnv         string
		want          func(componentPath string) string
	}{
		{
			name:          "lookup wins over os env",
			componentPath: "component",
			lookup:        func(string) string { return "from-lookup" },
			osEnv:         "from-os-env",
			want:          func(cp string) string { return filepath.Join(cp, "from-lookup") },
		},
		{
			name:          "nil lookup falls back to os env",
			componentPath: "component",
			lookup:        nil,
			osEnv:         "from-os-env",
			want:          func(cp string) string { return filepath.Join(cp, "from-os-env") },
		},
		{
			name:          "lookup returning empty falls back to os env",
			componentPath: "component",
			lookup:        func(string) string { return "" },
			osEnv:         "from-os-env",
			want:          func(cp string) string { return filepath.Join(cp, "from-os-env") },
		},
		{
			name:          "no lookup, no os env, defaults to .terraform",
			componentPath: "component",
			lookup:        nil,
			osEnv:         "",
			want:          func(cp string) string { return filepath.Join(cp, ".terraform") },
		},
		{
			name:          "absolute value from lookup is honored as-is",
			componentPath: "component",
			lookup:        func(string) string { return absPath },
			osEnv:         "",
			want:          func(string) string { return absPath },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TF_DATA_DIR", tt.osEnv)
			componentPath := filepath.Join(t.TempDir(), tt.componentPath)

			got := DataDir(componentPath, tt.lookup)

			assert.Equal(t, filepath.Clean(tt.want(componentPath)), got)
		})
	}
}

// TestMarkerPath asserts the marker file lives directly inside the data directory.
func TestMarkerPath(t *testing.T) {
	dataDir := filepath.Join("some", "data-dir")

	got := MarkerPath(dataDir)

	assert.Equal(t, filepath.Join(dataDir, MarkerFileName), got)
}
