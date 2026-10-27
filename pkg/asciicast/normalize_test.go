package asciicast

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRecordingAcrossEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording.cast")
	original := `{"version":3,"width":80,"height":24}
[0.100000000000000001,"o","before /local/"]
[0.02,"e","project after"]
[0,"m","checkpoint"]
[0.03,"o","/local/"]
[0,"m","boundary"]
[0.04,"o","project"]
`
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
	require.NoError(t, NormalizeRecording(path, map[string]string{"/local/project": "/repo"}))
	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, `{"version":3,"width":80,"height":24}
[0.100000000000000001,"o","before /repo"]
[0.02,"e"," after"]
[0,"m","checkpoint"]
[0.03,"o","/local/"]
[0,"m","boundary"]
[0.04,"o","project"]
`, string(actual))
}
