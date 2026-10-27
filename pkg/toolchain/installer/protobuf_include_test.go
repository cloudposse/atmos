package installer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

func TestProtocInstallPreservesBundledStandardIncludes(t *testing.T) {
	directory := t.TempDir()
	staging := filepath.Join(directory, "staging")
	binarySource := filepath.Join("bin", "protoc")
	includeSource := filepath.Join("include", "google", "protobuf", "empty.proto")
	writeFileUnder(t, staging, binarySource, "PROTOC")
	writeFileUnder(t, staging, includeSource, "syntax = \"proto3\"; package google.protobuf; message Empty {}")
	writeFileUnder(t, staging, "readme.txt", "Protocol Buffers compiler")
	instance := &Installer{binDir: directory}
	versionDir := filepath.Join(directory, "protocolbuffers", "protobuf", "v36.2")
	tool := &registry.Tool{Files: []registry.File{{Name: "protoc", Src: binarySource}}}
	require.NoError(t, instance.extractFilesFromDir(staging, filepath.Join(versionDir, "protoc"), tool, nil, nil))

	binaryPath := instance.GetBinaryPath("protocolbuffers", "protobuf", "v36.2", "protoc")
	binary, err := os.ReadFile(binaryPath)
	require.NoError(t, err)
	require.Equal(t, "PROTOC", string(binary))
	// protoc locates its well-known types in ../include relative to its executable.
	include, err := os.ReadFile(filepath.Join(filepath.Dir(binaryPath), "..", includeSource))
	require.NoError(t, err, "flattening protoc silently drops the standard type imports needed to compile API definitions")
	require.Equal(t, "syntax = \"proto3\"; package google.protobuf; message Empty {}", string(include))
}

func TestProtocReinstallReplacesFlattenedInstall(t *testing.T) {
	directory := t.TempDir()
	instance := &Installer{binDir: directory}
	versionDir := filepath.Join(directory, "protocolbuffers", "protobuf", "v36.2")
	binaryName := EnsureWindowsExeExtension("protoc")
	writeFileUnder(t, versionDir, binaryName, "OLD-FLAT-PROTOC")
	archive := filepath.Join(directory, "protoc.tar.gz")
	writeTarGzTree(t, archive, []tarEntry{
		{name: filepath.ToSlash(filepath.Join("bin", binaryName)), content: "PROTOC", mode: 0o755},
		{name: "include/google/protobuf/empty.proto", content: "EMPTY-PROTO", mode: 0o644},
	})
	tool := &registry.Tool{RepoOwner: "protocolbuffers", RepoName: "protobuf", Files: []registry.File{{Name: "protoc", Src: filepath.Join("bin", "protoc")}}}
	resolved, err := instance.extractAndInstall(tool, archive, "v36.2")
	require.NoError(t, err)
	binary, err := os.ReadFile(resolved)
	require.NoError(t, err)
	require.Equal(t, "PROTOC", string(binary))
	include, err := os.ReadFile(filepath.Join(filepath.Dir(resolved), "..", "include", "google", "protobuf", "empty.proto"))
	require.NoError(t, err)
	require.Equal(t, "EMPTY-PROTO", string(include))
	require.NoFileExists(t, filepath.Join(versionDir, binaryName), "stale flat binary must not shadow the preserved installation")
}
