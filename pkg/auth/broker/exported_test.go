package broker

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// unsetForTest registers a restore for key with t.Setenv, then removes it for the test.
func unsetForTest(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	require.NoError(t, os.Unsetenv(key))
}

func TestWithoutBrokeredEnvironment_RestoresPreBrokerState(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	const (
		newKey      = "ATMOS_BROKER_TEST_NEW"
		replacedKey = "ATMOS_BROKER_TEST_REPLACED"
		untouched   = "ATMOS_BROKER_TEST_UNTOUCHED"
	)
	unsetForTest(t, newKey)
	t.Setenv(replacedKey, "before")
	t.Setenv(untouched, "mine")

	Register(&fakeProvider{name: "fake", enabled: true, env: map[string]string{newKey: "minted", replacedKey: "after"}})
	runEnabledBrokers(t.Context(), &schema.AtmosConfiguration{})
	require.Equal(t, "minted", os.Getenv(newKey), "prerequisite: the broker exported into the process")
	require.Equal(t, "after", os.Getenv(replacedKey))

	input := []string{"PATH=/bin", newKey + "=minted", replacedKey + "=after", untouched + "=mine", "not-a-variable"}
	restored := WithoutBrokeredEnvironment(input)

	assert.Equal(t, []string{"PATH=/bin", replacedKey + "=before", untouched + "=mine", "not-a-variable"}, restored,
		"a variable the broker created is removed and one it replaced gets its earlier value back")
	assert.Equal(t, newKey+"=minted", input[1], "the input is not modified")

	value, exported := PreBrokerValue(newKey)
	assert.True(t, exported)
	assert.Empty(t, value, "the variable was unset before the broker ran")
	value, exported = PreBrokerValue(replacedKey)
	assert.True(t, exported)
	assert.Equal(t, "before", value)
	_, exported = PreBrokerValue(untouched)
	assert.False(t, exported, "a variable no broker exported is not broker-owned")
}

// Without a broker run there is nothing to restore: the environment passes through untouched.
func TestWithoutBrokeredEnvironment_NoBrokerRunIsANoOp(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	input := []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=url.https://example.com/.insteadOf"}

	assert.Equal(t, input, WithoutBrokeredEnvironment(input))
	_, exported := PreBrokerValue("GIT_CONFIG_COUNT")
	assert.False(t, exported)
}

// Two brokers exporting the same variable: the remembered state is the one from before either ran.
func TestRecordExport_KeepsTheEarliestState(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	const key = "ATMOS_BROKER_TEST_SHARED"
	t.Setenv(key, "original")

	Register(&fakeProvider{name: "first", enabled: true, env: map[string]string{key: "first"}})
	Register(&fakeProvider{name: "second", enabled: true, env: map[string]string{key: "second"}})
	runEnabledBrokers(t.Context(), &schema.AtmosConfiguration{})
	require.Equal(t, "second", os.Getenv(key))

	value, exported := PreBrokerValue(key)
	assert.True(t, exported)
	assert.Equal(t, "original", value)
}

// rejectedValue is a value os.Setenv refuses on every platform: it contains a NUL byte.
const rejectedValue = "bad\x00value"

// A later broker whose export of a variable fails must not erase the record of an earlier broker
// that exported it successfully: the earlier credential is still in the process and still owned.
func TestRunEnabledBrokers_FailedExportKeepsAnEarlierBrokersRecord(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	const key = "ATMOS_BROKER_TEST_CONTESTED"
	unsetForTest(t, key)

	Register(&fakeProvider{name: "first", enabled: true, env: map[string]string{key: "minted"}})
	Register(&fakeProvider{name: "second", enabled: true, env: map[string]string{key: rejectedValue}})
	runEnabledBrokers(t.Context(), &schema.AtmosConfiguration{})
	require.Equal(t, "minted", os.Getenv(key), "prerequisite: the second export was rejected, so the first broker's value is still exported")

	_, exported := PreBrokerValue(key)
	assert.True(t, exported, "the first broker's export stays recorded")
	assert.Equal(t, []string{"PATH=/bin"}, WithoutBrokeredEnvironment([]string{"PATH=/bin", key + "=minted"}),
		"so its credential is still withheld from a named identity")
}

// A variable whose only export attempt failed was never exported, so it is not broker-owned.
func TestRunEnabledBrokers_FailedExportIsNotRecorded(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	const key = "ATMOS_BROKER_TEST_REJECTED"
	t.Setenv(key, "ambient")

	Register(&fakeProvider{name: "only", enabled: true, env: map[string]string{key: rejectedValue}})
	runEnabledBrokers(t.Context(), &schema.AtmosConfiguration{})

	_, exported := PreBrokerValue(key)
	assert.False(t, exported)
	assert.Equal(t, []string{key + "=ambient"}, WithoutBrokeredEnvironment([]string{key + "=ambient"}))
}
