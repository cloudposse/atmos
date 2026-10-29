package config_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/script"
	runtime "github.com/cloudposse/atmos/pkg/script/starlark"
)

func TestConfigMerge(t *testing.T) {
	t.Parallel()
	result, err := runtime.New().Execute(t.Context(), script.Spec{Source: `
rule = {"database":{"host":str,"port":int},"tags":schema.list(str),"enabled":bool}
base = config.validate({"database":{"host":"localhost","port":5432},"tags":["base"],"enabled":True},schema=rule)
overlay = config.validate({"database":{"host":"prod"},"tags":["prod"],"enabled":False},schema={"database":{"host":str},"tags":schema.list(str),"enabled":bool})
merged = base.merge(overlay)
output = [merged,base,overlay,merged.database.port,base.merge({}).merge({"enabled":False}).enabled]
`})
	require.NoError(t, err)
	assert.JSONEq(t, `[{"database":{"host":"prod","port":5432},"tags":["prod"],"enabled":false},{"database":{"host":"localhost","port":5432},"tags":["base"],"enabled":true},{"database":{"host":"prod"},"tags":["prod"],"enabled":false},5432,false]`, result.Value)
}

func TestConfigMergeListStrategies(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ strategy, want string }{
		{"replace", `{"values":[9,8,7],"objects":[{"port":2,"tags":["b"]},{"port":3}],"nested":{"values":[]}}`},
		{"append", `{"values":[1,2,9,8,7],"objects":[{"name":"api","port":1,"tags":["a"]},{"port":2,"tags":["b"]},{"port":3}],"nested":{"values":[1]}}`},
		{"merge", `{"values":[1,2,7],"objects":[{"name":"api","port":2,"tags":["a"]},{"port":3}],"nested":{"values":[1]}}`},
	} {
		t.Run(tc.strategy, func(t *testing.T) {
			t.Parallel()
			result, err := runtime.New().Execute(t.Context(), script.Spec{Source: fmt.Sprintf(`
base = config.validate({"values":[1,2],"objects":[{"name":"api","port":1,"tags":["a"]}],"nested":{"values":[1]}},schema={"values":schema.list(int),"objects":schema.list({"name":str,"port":int,"tags":schema.list(str)}),"nested":{"values":schema.list(int)}})
output = base.merge({"values":[9,8,7],"objects":[{"port":2,"tags":["b"]},{"port":3}],"nested":{"values":[]}}, list_merge_strategy=%q)
`, tc.strategy)})
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, result.Value)
		})
	}
}

func TestConfigMergeScalarsAndTypes(t *testing.T) {
	t.Parallel()
	result, err := runtime.New().Execute(t.Context(), script.Spec{Source: `
base = config.validate({"value":1,"count":10,"name":"api","flag":True,"nested":{"x":1},"list":[1],"big":123456789012345678901234567890,"ratio":1.0},schema={"value":int,"count":int,"name":str,"flag":bool,"nested":{"x":int},"list":schema.list(int),"big":int,"ratio":float})
merged = base.merge({"value":None,"count":0,"name":"","flag":False,"nested":[],"list":{"new":1}})
output = [merged, str(merged.big),type(merged.big),type(merged.ratio)]
`})
	require.NoError(t, err)
	assert.JSONEq(t, `[{"value":null,"count":0,"name":"","flag":false,"nested":[],"list":{"new":1},"big":123456789012345678901234567890,"ratio":1.0},"123456789012345678901234567890","int","float"]`, result.Value)
}

func TestConfigMergeMethodsAndParallel(t *testing.T) {
	t.Parallel()
	result, err := runtime.New().Execute(t.Context(), script.Spec{Source: `
base = config.validate({"merge":"data","nested":{"tags":["base"]}},schema={"merge":str,"nested":{"tags":schema.list(str)}})
overlay = {"nested":{"tags":["prod"]}}
merged = base.merge(other=overlay)
overlay["nested"]["tags"].append("later")
merge = base.merge
def run():
    return merge({"nested":{"tags":["task"]}},list_merge_strategy="append")
output = [merged,base["merge"],dir(base),len(base),dict(base),steps.parallel(functions=[run,run]),base.nested.merge({"tags":[]})]
`})
	require.NoError(t, err)
	assert.JSONEq(t, `[{"merge":"data","nested":{"tags":["prod"]}},"data",["merge","nested"],2,{"merge":"data","nested":{"tags":["base"]}},[{"merge":"data","nested":{"tags":["base","task"]}},{"merge":"data","nested":{"tags":["base","task"]}}],{"tags":[]}]`, result.Value)
}

func TestConfigMergeErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, source, want string }{
		{"missing", `base.merge()`, "missing argument for other"},
		{"not an object", `base.merge([])`, "config record or dictionary"},
		{"strategy", `base.merge({},list_merge_strategy="invalid")`, "replace, append, or merge"},
		{"strategy positional", `base.merge({},"append")`, "keyword argument"},
		{"bad key", `base.merge({1:"x"})`, "keys must be strings"},
		{"function", `base.merge({"value":len})`, "unsupported configuration value"},
		{"cycle", `cycle = []
cycle.append(cycle)
base.merge({"cycle":cycle})`, "nesting limit or contains a cycle"},
		{"frozen", `merged = base.merge({"tags":[]})
merged.tags.append("oops")`, "frozen"},
		{"revalidation", `merged = base.merge({"port":"wrong"})
config.validate(merged,schema={"port":int})`, "port: expected int"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runtime.New().Execute(t.Context(), script.Spec{Source: `base = config.validate({"port":5432},schema={"port":int})` + "\n" + tc.source})
			require.ErrorContains(t, err, tc.want)
		})
	}
}
