package configuration

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	encryptedBackend   = "terraform {\n  backend \"local\" {}\n  encryption {\n    key_provider \"pbkdf2\" \"default\" {\n      passphrase = \"x\"\n    }\n  }\n}\n"
	unencryptedBackend = "terraform {\n  backend \"local\" {}\n}\n"
)

func backendFile(content, policy string) schema.ComponentConfigurationFile {
	return schema.ComponentConfigurationFile{Path: "backend.tf", Content: []byte(content), IfExists: policy}
}

func writeModuleFile(t *testing.T, directory, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600))
}

func TestWriteFilesReportListsOnlyFilesSkipKeptWithDifferentBytes(t *testing.T) {
	t.Run("skip creates a missing file", func(t *testing.T) {
		directory := t.TempDir()
		report, err := WriteFilesReport(directory, []schema.ComponentConfigurationFile{backendFile(encryptedBackend, "skip")})
		require.NoError(t, err)
		require.Empty(t, report.Retained, "a file skip created is not retained")
		content, err := os.ReadFile(filepath.Join(directory, "backend.tf"))
		require.NoError(t, err)
		require.Equal(t, encryptedBackend, string(content))
	})
	t.Run("skip keeps a different file", func(t *testing.T) {
		directory := t.TempDir()
		writeModuleFile(t, directory, "backend.tf", unencryptedBackend)
		file := backendFile(encryptedBackend, "skip")
		report, err := WriteFilesReport(directory, []schema.ComponentConfigurationFile{file})
		require.NoError(t, err)
		require.Len(t, report.Retained, 1)
		require.Equal(t, file.Path, report.Retained[0].Path)
		require.Equal(t, file.Content, report.Retained[0].Content)
	})
	t.Run("skip finds the same bytes", func(t *testing.T) {
		directory := t.TempDir()
		writeModuleFile(t, directory, "backend.tf", encryptedBackend)
		report, err := WriteFilesReport(directory, []schema.ComponentConfigurationFile{backendFile(encryptedBackend, "skip")})
		require.NoError(t, err)
		require.Empty(t, report.Retained)
	})
	t.Run("skip keeps bytes this host wrote earlier", func(t *testing.T) {
		directory := t.TempDir()
		_, err := WriteFilesReport(directory, []schema.ComponentConfigurationFile{backendFile(unencryptedBackend, "skip")})
		require.NoError(t, err)
		report, err := WriteFilesReport(directory, []schema.ComponentConfigurationFile{backendFile(encryptedBackend, "skip")})
		require.NoError(t, err)
		require.Len(t, report.Retained, 1, "skip never replaces an existing file, even one it generated")
	})
	t.Run("overwrite replaces", func(t *testing.T) {
		directory := t.TempDir()
		writeModuleFile(t, directory, "backend.tf", unencryptedBackend)
		report, err := WriteFilesReport(directory, []schema.ComponentConfigurationFile{backendFile(encryptedBackend, "overwrite")})
		require.NoError(t, err)
		require.Empty(t, report.Retained)
	})
	t.Run("a kept lock file is not retained", func(t *testing.T) {
		directory := t.TempDir()
		lock := schema.ComponentConfigurationFile{Path: lockFileName, Content: []byte("unit"), IfExists: "overwrite"}
		_, err := WriteFilesReport(directory, []schema.ComponentConfigurationFile{lock})
		require.NoError(t, err)
		writeModuleFile(t, directory, lockFileName, "updated by terraform")
		report, err := WriteFilesReport(directory, []schema.ComponentConfigurationFile{lock})
		require.NoError(t, err)
		require.Empty(t, report.Retained)
	})
	t.Run("a failed write reports nothing", func(t *testing.T) {
		directory := t.TempDir()
		writeModuleFile(t, directory, "backend.tf", unencryptedBackend)
		report, err := WriteFilesReport(directory, []schema.ComponentConfigurationFile{backendFile(encryptedBackend, "error")})
		require.Error(t, err)
		require.Empty(t, report.Retained)
	})
}

func TestUnencryptedRetainedFiles(t *testing.T) {
	for _, tc := range []struct {
		name      string
		existing  string
		generated string
		extra     map[string]string
		want      []string
	}{
		{name: "retained file drops encryption", existing: unencryptedBackend, generated: encryptedBackend, want: []string{"backend.tf"}},
		{name: "retained file has its own encryption", existing: encryptedBackend + "\n# local edit\n", generated: encryptedBackend},
		{name: "another file configures encryption", existing: unencryptedBackend, generated: encryptedBackend, extra: map[string]string{"encryption.tf": "terraform {\n  encryption {}\n}\n"}},
		{name: "a JSON file configures encryption", existing: unencryptedBackend, generated: encryptedBackend, extra: map[string]string{"encryption.tf.json": `{"terraform": {"encryption": {}}}`}},
		{name: "a tofu file configures encryption", existing: unencryptedBackend, generated: encryptedBackend, extra: map[string]string{"encryption.tofu": "terraform {\n  encryption {}\n}\n"}},
		{name: "other files without encryption", existing: unencryptedBackend, generated: encryptedBackend, extra: map[string]string{"main.tf": "resource \"null_resource\" \"x\" {}\n", "notes.txt": "terraform { encryption {} }"}, want: []string{"backend.tf"}},
		{name: "generated file had no encryption", existing: "# kept\n", generated: unencryptedBackend},
		{name: "a module file cannot be parsed", existing: unencryptedBackend, generated: encryptedBackend, extra: map[string]string{"broken.tf": "terraform {"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			writeModuleFile(t, directory, "backend.tf", tc.existing)
			for name, content := range tc.extra {
				writeModuleFile(t, directory, name, content)
			}
			report, err := WriteFilesReport(directory, []schema.ComponentConfigurationFile{backendFile(tc.generated, "skip")})
			require.NoError(t, err)
			require.Len(t, report.Retained, 1)
			require.Equal(t, tc.want, UnencryptedRetainedFiles(directory, report))
		})
	}
	t.Run("nothing retained", func(t *testing.T) {
		require.Nil(t, UnencryptedRetainedFiles(t.TempDir(), WriteReport{}))
	})
	t.Run("directory missing", func(t *testing.T) {
		report := WriteReport{Retained: []schema.ComponentConfigurationFile{backendFile(encryptedBackend, "skip")}}
		require.Nil(t, UnencryptedRetainedFiles(filepath.Join(t.TempDir(), "absent"), report), "an unknown outcome is not reported")
	})
	t.Run("generated JSON with encryption", func(t *testing.T) {
		directory := t.TempDir()
		writeModuleFile(t, directory, "backend.tf.json", `{"terraform": {"backend": {"local": {}}}}`)
		file := schema.ComponentConfigurationFile{Path: "backend.tf.json", Content: []byte(`{"terraform": {"encryption": {}}}`), IfExists: "skip"}
		report, err := WriteFilesReport(directory, []schema.ComponentConfigurationFile{file})
		require.NoError(t, err)
		require.Equal(t, []string{"backend.tf.json"}, UnencryptedRetainedFiles(directory, report))
	})
}

// Each run owns its working directory, so concurrent components do not share report state.
func TestWriteFilesReportConcurrentDirectories(t *testing.T) {
	const runs = 8
	var wait sync.WaitGroup
	results := make([][]string, runs)
	errs := make([]error, runs)
	directories := make([]string, runs)
	for i := range runs {
		directories[i] = t.TempDir()
		if i%2 == 0 {
			writeModuleFile(t, directories[i], "backend.tf", unencryptedBackend)
		}
	}
	for i := range runs {
		wait.Add(1)
		go func() {
			defer wait.Done()
			report, err := WriteFilesReport(directories[i], []schema.ComponentConfigurationFile{backendFile(fmt.Sprintf("%s# run %d\n", encryptedBackend, i), "skip")})
			errs[i] = err
			results[i] = UnencryptedRetainedFiles(directories[i], report)
		}()
	}
	wait.Wait()
	for i := range runs {
		require.NoError(t, errs[i])
		if i%2 == 0 {
			require.Equal(t, []string{"backend.tf"}, results[i])
		} else {
			require.Nil(t, results[i])
		}
	}
}
