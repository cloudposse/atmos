package terraform

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component/configuration"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestWarnRetainedUnencrypted(t *testing.T) {
	const encrypted = "terraform {\n  encryption {}\n}\n"
	var output bytes.Buffer
	ioCtx, err := iolib.NewContext(iolib.WithStreams(warningTestStreams{stderr: &output}))
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	t.Cleanup(ui.ReinitFormatter)
	t.Setenv("TF_ENCRYPTION", "")
	retainedIn := func(t *testing.T) (string, configuration.WriteReport) {
		t.Helper()
		directory := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(directory, "backend.tf"), []byte("terraform {}\n"), 0o600))
		report, err := configuration.WriteFilesReport(directory, []schema.ComponentConfigurationFile{{Path: "backend.tf", Content: []byte(encrypted), IfExists: "skip"}})
		require.NoError(t, err)
		require.Len(t, report.Retained, 1)
		return directory, report
	}
	for _, tc := range []struct {
		name        string
		info        schema.ConfigAndStacksInfo
		processEnv  string
		wantWarning bool
	}{
		{name: "no other encryption", wantWarning: true},
		{name: "env list sets TF_ENCRYPTION", info: schema.ConfigAndStacksInfo{ComponentEnvList: []string{"TF_ENCRYPTION=key_provider {}"}}},
		{name: "env list clears TF_ENCRYPTION", info: schema.ConfigAndStacksInfo{ComponentEnvList: []string{"TF_ENCRYPTION=x", "TF_ENCRYPTION="}}, processEnv: "x", wantWarning: true},
		{name: "env section sets TF_ENCRYPTION", info: schema.ConfigAndStacksInfo{ComponentEnvSection: schema.AtmosSectionMapType{"TF_ENCRYPTION": "key_provider {}"}}},
		{name: "process sets TF_ENCRYPTION", processEnv: "key_provider {}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output.Reset()
			t.Setenv("TF_ENCRYPTION", tc.processEnv)
			directory, report := retainedIn(t)
			info := tc.info
			info.ComponentFromArg, info.Stack = "unit", "dev"
			warnRetainedUnencrypted(&info, directory, report)
			if tc.wantWarning {
				require.Contains(t, output.String(), "state will not be encrypted")
				require.Contains(t, output.String(), "backend.tf")
			} else {
				require.Empty(t, output.String())
			}
		})
	}
	t.Run("nothing retained", func(t *testing.T) {
		output.Reset()
		warnRetainedUnencrypted(&schema.ConfigAndStacksInfo{}, t.TempDir(), configuration.WriteReport{})
		require.Empty(t, output.String())
	})
}
