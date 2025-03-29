package flags

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// Store the original function for restoration in tests.
var originalGetFlagsFn = getFlagsFn

// mockFlagGetter implements the flagGetter interface for testing.
type mockFlagGetter struct {
	shouldErrorOn map[string]bool
}

// GetString mocks the GetString method.
func (m *mockFlagGetter) GetString(name string) (string, error) {
	if m.shouldErrorOn[name] {
		return "", errors.New("mock flag error")
	}
	return "test-value", nil
}

// GetInt mocks the GetInt method.
func (m *mockFlagGetter) GetInt(name string) (int, error) {
	if m.shouldErrorOn[name] {
		return 0, errors.New("mock flag error")
	}
	return 10, nil
}

// TestGetCommonListFlags_Success tests the happy path of GetCommonListFlags.
func TestGetCommonListFlags_Success(t *testing.T) {
	defer func() {
		getFlagsFn = originalGetFlagsFn
	}()

	mockFlagGetter := &mockFlagGetter{
		shouldErrorOn: make(map[string]bool),
	}

	getFlagsFn = func(cmd *cobra.Command) flagGetter {
		return mockFlagGetter
	}

	flags, err := GetCommonListFlags(&cobra.Command{})

	assert.NoError(t, err)
	assert.NotNil(t, flags)
	assert.Equal(t, "test-value", flags.Format)
	assert.Equal(t, 10, flags.MaxColumns)
	assert.Equal(t, "test-value", flags.Delimiter)
	assert.Equal(t, "test-value", flags.Stack)
	assert.Equal(t, "test-value", flags.Query)
}

// TestGetCommonListFlags_Errors tests error handling in GetCommonListFlags.
func TestGetCommonListFlags_Errors(t *testing.T) {
	defer func() {
		getFlagsFn = originalGetFlagsFn
	}()

	testCases := []struct {
		name      string
		errorFlag string
	}{
		{
			name:      "Format flag error",
			errorFlag: "format",
		},
		{
			name:      "MaxColumns flag error",
			errorFlag: "max-columns",
		},
		{
			name:      "Delimiter flag error",
			errorFlag: "delimiter",
		},
		{
			name:      "Stack flag error",
			errorFlag: "stack",
		},
		{
			name:      "Query flag error",
			errorFlag: "query",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockFlagGetter := &mockFlagGetter{
				shouldErrorOn: map[string]bool{
					tc.errorFlag: true,
				},
			}

			getFlagsFn = func(cmd *cobra.Command) flagGetter {
				return mockFlagGetter
			}

			flags, err := GetCommonListFlags(&cobra.Command{})

			assert.Error(t, err)
			assert.Nil(t, flags)
		})
	}
}

// TestAddCommonListFlags tests that AddCommonListFlags adds the expected flags.
func TestAddCommonListFlags(t *testing.T) {
	cmd := &cobra.Command{}

	AddCommonListFlags(cmd)

	assert.NotNil(t, cmd.PersistentFlags().Lookup("format"))
	assert.NotNil(t, cmd.PersistentFlags().Lookup("max-columns"))
	assert.NotNil(t, cmd.PersistentFlags().Lookup("delimiter"))
	assert.NotNil(t, cmd.PersistentFlags().Lookup("stack"))
	assert.NotNil(t, cmd.PersistentFlags().Lookup("query"))
}
