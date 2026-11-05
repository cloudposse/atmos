package ui

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
)

func TestScaffoldHookValidationBeforeWrites(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		for _, pin := range []string{"", "2026-10-18", "2026-10-19"} {
			t.Run(pin+"/dry-run="+strconv.FormatBool(dryRun), func(t *testing.T) {
				ui := createTestUI(t)
				ui.SetEdition(pin)
				ui.processor.DryRun = dryRun
				ui.SetSkipHooks(func(string) bool { return true })
				target := filepath.Join(t.TempDir(), "generated")
				template := &templates.Configuration{Name: "test", Files: []templates.File{
					{Path: "scaffold.yaml", Content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test
spec:
  hooks:
    typo:
      kind: step
      events: [after.scaffold.genrate]
      when: "false"
      type: shell
      command: should-never-run
`},
					{Path: "result.txt", Content: "generated", Permissions: 0o644},
				}}
				err := ui.Execute(template, target, false, false, true, nil)
				if pin == "2026-10-18" {
					require.NoError(t, err)
					if !dryRun {
						require.FileExists(t, filepath.Join(target, "result.txt"))
					}
				} else {
					require.ErrorIs(t, err, errUtils.ErrUnsupportedHookEvent)
					require.NoDirExists(t, target)
				}
				if dryRun {
					require.NoDirExists(t, target)
				}
			})
		}
	}
}
