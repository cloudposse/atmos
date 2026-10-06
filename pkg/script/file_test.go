package script

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectFilePreservesNormalCLI(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"terraform", "plan"}, {"--help"}, {"deploy"}, {"--chdir=repo", "deploy.star"}} {
		file, err := DetectFile(args)
		require.NoError(t, err)
		assert.Nil(t, file)
	}
}

func TestDetectStandaloneFile(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source string
		detected     bool
	}{
		{"deploy.star", `print("hello")`, true},
		{"deploy", "#!/usr/bin/env atmos\nprint('hello')", true},
		{"absolute", "#!/usr/bin/atmos\nprint('hello')", true},
		{"env-s", "#!/usr/bin/env -S atmos\nprint('hello')", true},
		{"bash", "#!/bin/sh\necho hello", false},
		{"not-atmos", "#!/usr/bin/env atmos-other\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), tc.name)
			require.NoError(t, os.WriteFile(path, []byte(tc.source), 0o600))
			file, err := DetectFile([]string{path, "--help", "--chdir=/not-atmos", "two words"})
			require.NoError(t, err)
			if !tc.detected {
				assert.Nil(t, file)
				return
			}
			assert.Equal(t, []string{"--help", "--chdir=/not-atmos", "two words"}, file.Args)
			physical, err := filepath.EvalSymlinks(path)
			require.NoError(t, err)
			assert.Equal(t, physical, file.Path)
		})
	}
	_, err := DetectFile([]string{filepath.Join(t.TempDir(), "missing.star")})
	require.Error(t, err)
	_, err = DetectFile([]string{t.TempDir()})
	require.ErrorContains(t, err, "regular file")
}
