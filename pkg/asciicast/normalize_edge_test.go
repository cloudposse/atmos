package asciicast

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const normalizeTestHeader = `{"version":3,"width":80,"height":24}`

// writeNormalizeRecording writes a header plus the given event lines and returns the path.
func writeNormalizeRecording(t *testing.T, events ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "recording.cast")
	content := normalizeTestHeader + "\n"
	for _, event := range events {
		content += event + "\n"
	}
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestNormalizeRecordingReplacementSemantics(t *testing.T) {
	tests := []struct {
		name         string
		events       []string
		replacements map[string]string
		want         []string
	}{
		{
			name:         "longest overlapping key wins",
			events:       []string{`[0.1,"o","/work/project/x"]`},
			replacements: map[string]string{"/work": "SHORT", "/work/project": "LONG"},
			want:         []string{`[0.1,"o","LONG/x"]`},
		},
		{
			name:         "equal length keys are both replaced",
			events:       []string{`[0.1,"o","ab ac ad"]`},
			replacements: map[string]string{"ac": "2", "ab": "1"},
			want:         []string{`[0.1,"o","1 2 ad"]`},
		},
		{
			name:         "replacement text is not rescanned",
			events:       []string{`[0.1,"o","aaa"]`},
			replacements: map[string]string{"a": "aa"},
			want:         []string{`[0.1,"o","aaaaaa"]`},
		},
		{
			name:         "multibyte text is preserved around a match",
			events:       []string{`[0.1,"o","say héllo wörld ✓"]`},
			replacements: map[string]string{"héllo": "hi"},
			want:         []string{`[0.1,"o","say hi wörld ✓"]`},
		},
		{
			name:         "multibyte match split across events",
			events:       []string{`[0.1,"o","a caf"]`, `[0.2,"e","é au lait"]`},
			replacements: map[string]string{"café": "coffee"},
			want:         []string{`[0.1,"o","a coffee"]`, `[0.2,"e"," au lait"]`},
		},
		{
			name:         "match spanning three events collapses into the first",
			events:       []string{`[0.1,"o","ab"]`, `[0.2,"o","cd"]`, `[0.3,"o","ef!"]`},
			replacements: map[string]string{"abcdef": "X"},
			want:         []string{`[0.1,"o","X"]`, `[0.2,"o",""]`, `[0.3,"o","!"]`},
		},
		{
			name:         "input events break output groups",
			events:       []string{`[0.1,"o","/loc"]`, `[0.2,"i","x"]`, `[0.3,"o","al"]`},
			replacements: map[string]string{"/local": "/repo"},
			want:         []string{`[0.1,"o","/loc"]`, `[0.2,"i","x"]`, `[0.3,"o","al"]`},
		},
		{
			name:         "non-output events are never rewritten",
			events:       []string{`[0.1,"m","/local"]`, `[0.2,"o","/local"]`},
			replacements: map[string]string{"/local": "/repo"},
			want:         []string{`[0.1,"m","/local"]`, `[0.2,"o","/repo"]`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeNormalizeRecording(t, tt.events...)
			require.NoError(t, NormalizeRecording(path, tt.replacements))
			actual, err := os.ReadFile(path)
			require.NoError(t, err)
			want := normalizeTestHeader + "\n" + strings.Join(tt.want, "\n") + "\n"
			require.Equal(t, want, string(actual))
		})
	}
}

func TestNormalizeRecordingHeaderOnlyRecordingIsPreserved(t *testing.T) {
	path := writeNormalizeRecording(t)
	require.NoError(t, NormalizeRecording(path, map[string]string{"a": "b"}))
	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, normalizeTestHeader+"\n", string(actual))
}

func TestNormalizeRecordingWithoutReplacementsDoesNotTouchFile(t *testing.T) {
	// No replacements means the file is never opened, so even a missing path succeeds.
	missing := filepath.Join(t.TempDir(), "missing.cast")
	require.NoError(t, NormalizeRecording(missing, nil))
	require.NoFileExists(t, missing)
}

func TestNormalizeRecordingErrors(t *testing.T) {
	replacements := map[string]string{"a": "b"}

	t.Run("missing file", func(t *testing.T) {
		err := NormalizeRecording(filepath.Join(t.TempDir(), "missing.cast"), replacements)
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("empty file has no header", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.cast")
		require.NoError(t, os.WriteFile(path, nil, 0o600))
		require.ErrorIs(t, NormalizeRecording(path, replacements), ErrEmptyCastFile)
	})

	t.Run("oversized event line", func(t *testing.T) {
		huge, err := json.Marshal([]any{0.1, "o", strings.Repeat("x", maxEventTokenSize+1)})
		require.NoError(t, err)
		path := writeNormalizeRecording(t, string(huge))
		require.ErrorIs(t, NormalizeRecording(path, replacements), bufio.ErrTooLong)
	})

	t.Run("malformed event is rejected and file is unchanged", func(t *testing.T) {
		path := writeNormalizeRecording(t, `[0.1,"o","a"]`, `[0.2,"o"]`)
		before, err := os.ReadFile(path)
		require.NoError(t, err)
		err = NormalizeRecording(path, replacements)
		require.ErrorIs(t, err, errUtils.ErrAutomation)
		require.ErrorContains(t, err, "malformed event")
		after, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, before, after)
	})

	t.Run("empty replacement key is rejected and file is unchanged", func(t *testing.T) {
		path := writeNormalizeRecording(t, `[0.1,"o","a"]`)
		before, err := os.ReadFile(path)
		require.NoError(t, err)
		err = NormalizeRecording(path, map[string]string{"": "b"})
		require.ErrorIs(t, err, errUtils.ErrAutomation)
		require.ErrorContains(t, err, "empty output replacement")
		after, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, before, after)
	})

	t.Run("unwritable recording reports the write failure", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("file permission denial is not enforced for this platform or user")
		}
		path := writeNormalizeRecording(t, `[0.1,"o","a"]`)
		require.NoError(t, os.Chmod(path, 0o400))
		t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
		err := NormalizeRecording(path, replacements)
		require.ErrorIs(t, err, os.ErrPermission)
		require.ErrorContains(t, err, "normalize staged recording")
	})
}
