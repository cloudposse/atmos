package initcmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

type recordingProgress struct {
	messages []string
	stops    int
}

func (p *recordingProgress) Update(message string) { p.messages = append(p.messages, message) }
func (p *recordingProgress) Stop()                 { p.stops++ }

func TestInitProgressStopsOnSuccessAndFailure(t *testing.T) {
	for _, occupied := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "occupied-target"}[occupied], func(t *testing.T) {
			src, target := t.TempDir(), t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(src, "file"), []byte("source"), 0o600))
			if occupied {
				require.NoError(t, os.WriteFile(filepath.Join(target, "file"), []byte("existing"), 0o600))
			}
			progress := &recordingProgress{}
			err := executeInit(context.Background(), &initOptions{
				templateName: src, targetDir: target, copy: true, git: true, progress: progress,
			})
			if occupied {
				require.ErrorIs(t, err, errUtils.ErrTargetDirectoryNotEmpty)
				assert.NotContains(t, progress.messages, "Copying files")
			} else {
				require.NoError(t, err)
				assert.Contains(t, progress.messages, "Copying files")
				assert.Contains(t, progress.messages, "Initializing Git repository")
			}
			assert.Equal(t, 1, progress.stops, "the indicator must stop once before results or errors")
		})
	}
}
