package degradation

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAtmosComputedValue_String(t *testing.T) {
	assert.Equal(t, "(computed)", AtmosComputedValue{}.String())
}

func TestAtmosComputedValue_Stringer(t *testing.T) {
	var v fmt.Stringer = AtmosComputedValue{}
	assert.Equal(t, "(computed)", v.String())

	// fmt itself must pick up String() automatically, matching how text/template
	// renders a column value via fmt.Fprint.
	assert.Equal(t, "(computed)", fmt.Sprintf("%v", AtmosComputedValue{}))
}

func TestAtmosComputedValue_MarshalJSON(t *testing.T) {
	b, err := json.Marshal(AtmosComputedValue{})
	require.NoError(t, err)
	assert.Equal(t, `"(computed)"`, string(b))

	// Also confirm it round-trips correctly when nested in a map, matching how a
	// degraded value would appear inside a larger JSON-rendered component map.
	b, err = json.Marshal(map[string]any{"bucket": AtmosComputedValue{}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"bucket":"(computed)"}`, string(b))
}

func TestAtmosComputedValue_MarshalYAML(t *testing.T) {
	b, err := yaml.Marshal(AtmosComputedValue{})
	require.NoError(t, err)
	assert.Equal(t, "(computed)\n", string(b))

	b, err = yaml.Marshal(map[string]any{"bucket": AtmosComputedValue{}})
	require.NoError(t, err)
	assert.Equal(t, "bucket: (computed)\n", string(b))
}

func TestCollector_AddAndCount(t *testing.T) {
	var c Collector
	assert.Equal(t, 0, c.Count())

	c.Add(Warning{Stack: "dev", Component: "vpc", Function: "!terraform.state vpc dev bucket", Reason: "terraform state not provisioned"})
	assert.Equal(t, 1, c.Count())

	c.Add(Warning{Stack: "staging", Component: "eks", Function: "!terraform.output eks staging endpoint", Reason: "terraform output not found"})
	assert.Equal(t, 2, c.Count())
}

func TestCollector_Count_NilReceiver(t *testing.T) {
	var c *Collector
	assert.Equal(t, 0, c.Count())
}

func TestCollector_Summary_NoWarnings_NoPanic(t *testing.T) {
	var c Collector
	// No warnings collected: Summary must be a no-op (nothing to assert on output
	// directly here since ui.Warningf writes to the process's real stderr formatter,
	// but this must not panic and must return immediately).
	c.Summary()
	assert.Equal(t, 0, c.Count())
}

func TestCollector_Summary_WithWarnings_NoPanic(t *testing.T) {
	var c Collector
	c.Add(Warning{Stack: "dev", Component: "vpc", Function: "!terraform.state vpc dev bucket", Reason: "terraform state not provisioned"})
	c.Add(Warning{Stack: "dev", Component: "eks", Function: "!terraform.output eks dev endpoint", Reason: "terraform output not found"})

	// Summary must not panic and must reflect the accumulated count.
	c.Summary()
	assert.Equal(t, 2, c.Count())
}
