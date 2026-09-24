package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestEvaluationPathsForQuery(t *testing.T) {
	assert.Equal(t, [][]string{{"vars", "literal"}}, deferred.PathsForQuery(".vars.literal"))
	assert.Nil(t, deferred.PathsForQuery(".vars | select(.enabled)"))
	assert.Nil(t, deferred.PathsForQuery("."))
	assert.Nil(t, deferred.PathsForQuery(".vars[.key]"))
}

func TestDeferredDescribeComponentQuery(t *testing.T) {
	t.Chdir(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "list-deferred-auth"))
	ClearFindStacksMapCache()
	ac, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	deferred.ConfigureAuth(&ac, "")
	setDeferredAuthFactory(&ac, deferred.NewMockAuthFactory(gomock.NewController(t)))
	result, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		AtmosConfig: &ac, Component: "example", Stack: "dev",
		ProcessTemplates: true, ProcessYamlFunctions: true,
		ErrorOptions: DescribeStacksErrorOptions{EvaluationPaths: deferred.PathsForQuery(".vars.literal")},
	})
	require.NoError(t, err)
	assert.Equal(t, "preserved", result["vars"].(map[string]any)["literal"])
}
