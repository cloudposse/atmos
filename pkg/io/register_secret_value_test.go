package io

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterSecretValueMasksStructuredStringsWithoutMaskingMetadata(t *testing.T) {
	for name, value := range map[string]any{
		"JSON string": `{"password":"structured-secret-value","enabled":true,"port":8080}`,
		"map": map[string]any{
			"password": "structured-secret-value",
			"enabled":  true,
			"port":     8080,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(Reset)
			Reset()
			require.NoError(t, Initialize())

			RegisterSecretValue(value)

			assert.Equal(
				t,
				"password=<MASKED> enabled=true port=8080",
				MaskString("password=structured-secret-value enabled=true port=8080"),
			)
		})
	}
}

func TestRegisterSecretValueMasksDirectScalar(t *testing.T) {
	t.Cleanup(Reset)
	Reset()
	require.NoError(t, Initialize())

	RegisterSecretValue(8080)

	assert.Equal(t, "port=<MASKED>", MaskString("port=8080"))
}
