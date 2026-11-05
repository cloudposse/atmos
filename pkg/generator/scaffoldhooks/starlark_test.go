package scaffoldhooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
)

func TestRunEmbeddedStarlark(t *testing.T) {
	t.Parallel()
	target := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(target, "README.md"), []byte("ready"), 0o600))
	var hooksMap map[string]hooks.Hook
	require.NoError(t, yaml.Unmarshal([]byte(`verify:
  events: [after.scaffold.generate]
  kind: step
  type: script
  with:
    interpreter: starlark
    env:
      NAME: '{{ .Answers.name }}'
    script: |
      if not fs.exists("README.md"):
          fail("Generated project is missing README.md")
      result = steps.join(options=[env["NAME"], fs.read_file("README.md")], separator=" ")
      if result.value != "app ready":
          fail("Scaffold answers or step library unavailable")
`), &hooksMap))
	in := RunInput{
		HooksMap: hooksMap, Event: hooks.AfterScaffoldGenerate,
		Answers: map[string]any{"name": "app"}, Status: "success", TargetPath: target,
	}
	require.NoError(t, Run(in))
	require.NoError(t, os.Remove(filepath.Join(target, "README.md")))
	err := Run(in)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	require.ErrorContains(t, err, "Generated project is missing README.md")
}
