package exec

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// The include-not-found hints must survive stack processing, which wraps the parse error with
// the manifest's merge context, so the error renderer (errUtils.AllHints) can still show them.
// A template manifest reports the error from the final parse rather than from locals extraction.
func TestStackProcessingKeepsIncludeNotFoundHints(t *testing.T) {
	ClearLocalsExtractionCache()
	ClearFileContentCache()
	dir := writeStackFixture(t, map[string]string{
		"config/data.yaml":            "src: base_path\n",
		"stacks/deploy/dev.yaml.tmpl": "vars:\n  data: !include ./config/data.yaml .src\n",
	})
	stacksDir := filepath.Join(dir, "stacks")
	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}

	_, _, _, _, err := ProcessYAMLConfigFiles(atmosConfig, stacksDir, "", "", "", "", //nolint:dogsled // Only the error is under test.
		[]string{filepath.Join(stacksDir, "deploy", "dev.yaml.tmpl")}, false, false, false)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidStackManifest)
	assert.ErrorIs(t, err, u.ErrIncludeYamlFunctionInvalidFile)
	hints := strings.Join(errUtils.AllHints(err), "\n")
	assert.Contains(t, hints, "Did you mean `!include config/data.yaml .src`?")
}
