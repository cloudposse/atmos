package hooks

import (
	"strconv"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// depthRecorder is an engine that records the context it ran with.
type depthRecorder struct {
	seen []*ExecContext
}

func (d *depthRecorder) Run(ctx *ExecContext) (*Output, error) {
	d.seen = append(d.seen, ctx)
	return nil, nil
}

func (d *depthRecorder) Validate(*Hook) error { return nil }

// registerDepthKind registers a throwaway hook kind and removes it when the test ends.
func registerDepthKind(t *testing.T, engine Engine) string {
	t.Helper()
	name := "wp5-depth-" + strconv.Itoa(len(t.Name()))
	require.NoError(t, RegisterKind(&Kind{Name: name, Engine: engine}))
	t.Cleanup(func() {
		kindsMu.Lock()
		defer kindsMu.Unlock()
		delete(kinds, name)
	})
	return name
}

func depthHooks(kind string) *Hooks {
	return &Hooks{
		config: &schema.AtmosConfiguration{},
		info:   &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev"},
		items:  map[string]Hook{"recursive": {Events: []string{"after.terraform.plan"}, Kind: kind}},
	}
}

func TestRunAllExportsTheNextHookDepth(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  string
		set  bool
		want int
	}{
		{"unset starts at one", "", false, 1},
		{"zero", "0", true, 1},
		{"increments the inherited level", "3", true, 4},
		{"the last allowed level", "7", true, 8},
		{"a malformed value counts as zero", "many", true, 1},
		{"a negative value counts as zero", "-4", true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(HookDepthEnvVar, tc.env)
			}
			engine := &depthRecorder{}
			h := depthHooks(registerDepthKind(t, engine))

			require.NoError(t, h.RunAll(AfterTerraformPlan, h.config, h.info, nil, nil))

			require.Len(t, engine.seen, 1)
			assert.Equal(t, tc.want, engine.seen[0].HookDepth)
			assert.Equal(t, strconv.Itoa(tc.want), BuildAtmosEnv(engine.seen[0], "", "")[HookDepthEnvVar],
				"the hook's commands and steps receive the next level in their environment")
		})
	}
}

func TestRunAllStopsAtTheRecursionLimit(t *testing.T) {
	t.Setenv(HookDepthEnvVar, strconv.Itoa(maxHookDepth))
	engine := &depthRecorder{}
	h := depthHooks(registerDepthKind(t, engine))

	err := h.RunAll(AfterTerraformPlan, h.config, h.info, nil, nil)

	require.ErrorIs(t, err, errUtils.ErrHookRecursionLimit)
	assert.Empty(t, engine.seen, "the hook must not run once the limit is reached")
	details := cockroachErrors.FlattenDetails(err)
	assert.Contains(t, details, `"recursive"`, "the error names the hook")
	assert.Contains(t, details, "after.terraform.plan", "the error names the event")
}

func TestBuildAtmosEnvLeavesTheDepthUnsetOutsideRunAll(t *testing.T) {
	env := BuildAtmosEnv(&ExecContext{Info: &schema.ConfigAndStacksInfo{}}, "", "")
	assert.NotContains(t, env, HookDepthEnvVar)
}
