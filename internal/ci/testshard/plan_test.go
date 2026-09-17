package testshard

import (
	"math"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanCompleteness(t *testing.T) {
	names := []string{"TestSlow", "TestNew", "TestFast", "Example", "FuzzSeed", "TestÉclair", "Test", "Fuzz"}
	weights := map[string]float64{"TestSlow": 20, "TestFast": 0, "TestDeleted": 999}
	for _, count := range []int{1, 2, 4, 9} {
		groups, err := Plan(names, count, weights)
		require.NoError(t, err)
		require.Len(t, groups, count)
		var assigned []string
		for _, group := range groups {
			assigned = append(assigned, group...)
			matcher := regexp.MustCompile(Pattern(group))
			for _, name := range names {
				assert.Equal(t, slices.Contains(group, name), matcher.MatchString(name))
			}
			assert.False(t, matcher.MatchString("TestSlowSuffix"))
		}
		assert.ElementsMatch(t, names, assigned)
		reversed := slices.Clone(names)
		slices.Reverse(reversed)
		again, err := Plan(reversed, count, weights)
		require.NoError(t, err)
		assert.Equal(t, groups, again)
	}
	assert.Equal(t, "TestSlow", names[0], "caller inventory is not reordered")
}

func TestPlanBalancesDurationsAndValidatesInputs(t *testing.T) {
	groups, err := Plan([]string{"TestA", "TestB", "TestC", "TestD"}, 2, map[string]float64{"TestA": 10, "TestB": 6, "TestC": 4, "TestD": 0})
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"TestA", "TestD"}, {"TestB", "TestC"}}, groups)
	for _, names := range [][]string{nil, {"TestA", "TestA"}, {"TestMain"}, {"Testbad"}, {"TestA|.*"}} {
		_, err := Plan(names, 2, nil)
		require.ErrorIs(t, err, ErrPlan)
	}
	_, err = Plan([]string{"TestA"}, 0, nil)
	require.ErrorIs(t, err, ErrPlan)
	groups, err = Plan([]string{"TestA", "TestB", "TestC"}, 3, map[string]float64{"TestA": math.NaN(), "TestB": math.Inf(1), "TestC": -1})
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"TestA"}, {"TestB"}, {"TestC"}}, groups)
	assert.False(t, regexp.MustCompile(Pattern(nil)).MatchString("TestAnything"))
}

func TestDiscover(t *testing.T) {
	names, err := Discover("noise\nTestZ\nExample\nFuzzSeed\nTestÉclair\nTestMain\nok example 1s\n")
	require.NoError(t, err)
	assert.Equal(t, []string{"Example", "FuzzSeed", "TestZ", "TestÉclair"}, names)
	for _, output := range []string{"ok example (no tests)", "TestA\nTestA\n"} {
		_, err := Discover(output)
		require.ErrorIs(t, err, ErrPlan)
	}
}

func TestReadTimings(t *testing.T) {
	events := `noise
{"Action":"pass","Package":"p","Test":"TestA/child","Elapsed":8}
{"Action":"pass","Package":"p","Test":"TestA","Elapsed":10}
{"Action":"output","Package":"p","Test":"TestA","Elapsed":99}
{"Action":"fail","Package":"p","Test":"TestB","Elapsed":2}
{"Action":"skip","Package":"p","Test":"TestC","Elapsed":0}
{"Action":"fail","Package":"p","Elapsed":13}
{"Action":"pass","Test":"TestMissingPackage","Elapsed":99}
`
	timings, err := ReadTimings(strings.NewReader(events))
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"TestA": 10, "TestB": 2, "TestC": 0}, timings.Tests["p"])
	assert.Equal(t, map[string]float64{"p": 13}, timings.Packages)
	_, err = ReadTimings(strings.NewReader(strings.Repeat("x", 4*1024*1024+1)))
	require.Error(t, err)
}
