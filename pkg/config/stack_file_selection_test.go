package config

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSelectStaticStackFiles(t *testing.T) {
	root := t.TempDir()
	files := []string{filepath.Join(root, "dev.yaml"), filepath.Join(root, "other.yaml"), filepath.Join(root, "unrelated.yaml")}
	config := &schema.AtmosConfiguration{StacksBaseAbsolutePath: root}
	for _, source := range []string{"name: dev\n", "vars: {stage: dev}\n", "import: [catalog/name]\n", "name: '{{ .vars.stage }}'\n", "name: !literal dev\n", "{{ if .enabled }}\nname: dev\n{{ end }}\n", "name: prod\nname: dev\n", "name: !env STACK\n", "name: \"\"\n", "name: [dev]\n", "name: 123\n", "- dev\n", "\n", "alias: &name dev\nname: *name\n"} {
		t.Run(source, func(t *testing.T) {
			originals := append([]string(nil), files...)
			reads := []string{}
			reader := func(file string) (string, error) {
				reads = append(reads, file)
				if file == files[1] {
					return source, nil
				}
				return "name: prod\nimport: [https://must-not-fetch.invalid/file.yaml]\n", nil
			}
			got, err := SelectStaticStackFiles(config, "dev", files, reader)
			require.NoError(t, err)
			require.Equal(t, files[:2], got)
			require.Equal(t, files[1:], reads)
			require.Equal(t, originals, files)
		})
	}
}

func TestSelectStaticStackFilesReadErrors(t *testing.T) {
	expected := errors.New("source denied")
	reader := func(string) (string, error) { return "", expected }
	config := &schema.AtmosConfiguration{}
	_, err := SelectStaticStackFiles(config, "dev", []string{"other.yaml"}, reader)
	require.ErrorIs(t, err, expected)
	files := []string{"other.yaml"}
	got, err := SelectStaticStackFiles(config, "", files, reader)
	require.NoError(t, err)
	require.Equal(t, files, got)
	config.Templates.Settings.Delimiters = []string{"[["}
	_, err = SelectStaticStackFiles(config, "dev", files, reader)
	require.Error(t, err)
}
