package downloader

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveArtifactUsesContentHashForLocalDirectory(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "main.tf"), []byte("terraform {}"), 0o644))
	artifact, err := ResolveArtifact(context.Background(), nil, directory, directory)
	require.NoError(t, err)
	require.Equal(t, "local", artifact.Kind)
	require.Regexp(t, `^sha256:[a-f0-9]{64}$`, artifact.Identity)
}

func TestRedactSourceRemovesCredentialsAndQuery(t *testing.T) {
	require.Equal(t, "https://github.example.com/org/module.git", RedactSource("https://token@example@github.example.com/org/module.git?signature=secret"))
}
