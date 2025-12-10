package cmd

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// mockComponentResolver is a test implementation of ComponentResolver.
type mockComponentResolver struct {
	resolveFunc func(component, stack string) (string, error)
}

// ResolveComponentPath implements ComponentResolver interface.
func (m *mockComponentResolver) ResolveComponentPath(component, stack string) (string, error) {
	if m.resolveFunc != nil {
		return m.resolveFunc(component, stack)
	}
	return component, nil
}

func TestParseValidateComponentFlags(t *testing.T) {
	tests := []struct {
		name          string
		flagValues    map[string]string
		expectedStack string
		expectError   bool
	}{
		{
			name: "valid stack flag",
			flagValues: map[string]string{
				"stack": "prod-us-east-1",
			},
			expectedStack: "prod-us-east-1",
			expectError:   false,
		},
		{
			name: "empty stack flag",
			flagValues: map[string]string{
				"stack": "",
			},
			expectedStack: "",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test command with flags.
			cmd := &cobra.Command{}
			cmd.Flags().String("stack", "", "stack name")

			// Set flag values.
			for k, v := range tt.flagValues {
				require.NoError(t, cmd.Flags().Set(k, v))
			}

			// Parse flags.
			flags, err := parseValidateComponentFlags(cmd)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStack, flags.stack)
			}
		})
	}
}

func TestResolvePathBasedComponent_WithPath(t *testing.T) {
	// Use platform-specific absolute path.
	absolutePath := "/path/to/components/terraform/vpc"
	if filepath.Separator == '\\' {
		// Windows absolute path.
		absolutePath = "C:\\path\\to\\components\\terraform\\vpc"
	}

	tests := []struct {
		name              string
		component         string
		stack             string
		expectedComponent string
		expectError       bool
	}{
		{
			name:              "resolve absolute path",
			component:         absolutePath,
			stack:             "prod-us-east-1",
			expectedComponent: "vpc",
			expectError:       false,
		},
		{
			name:              "resolve relative path with ./",
			component:         "./components/terraform/vpc",
			stack:             "prod-us-east-1",
			expectedComponent: "vpc",
			expectError:       false,
		},
		{
			name:              "resolve relative path with ../",
			component:         "../terraform/vpc",
			stack:             "prod-us-east-1",
			expectedComponent: "vpc",
			expectError:       false,
		},
		{
			name:              "resolve current directory",
			component:         ".",
			stack:             "prod-us-east-1",
			expectedComponent: "current-component",
			expectError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk := NewTestKit(t)

			flags := &validateComponentFlags{
				stack: tt.stack,
			}

			// Create mock resolver that returns expected component.
			resolver := &mockComponentResolver{
				resolveFunc: func(component, stack string) (string, error) {
					return tt.expectedComponent, nil
				},
			}

			result, err := resolvePathBasedComponent(tt.component, flags, resolver)

			if tt.expectError {
				assert.Error(tk, err)
			} else {
				require.NoError(tk, err)
				assert.Equal(tk, tt.expectedComponent, result)
			}
		})
	}
}

func TestResolvePathBasedComponent_WithoutPath(t *testing.T) {
	tests := []struct {
		name              string
		component         string
		stack             string
		expectedComponent string
	}{
		{
			name:              "component name without path",
			component:         "vpc",
			stack:             "prod-us-east-1",
			expectedComponent: "vpc",
		},
		{
			name:              "component name with slash",
			component:         "vpc/security-group",
			stack:             "prod-us-east-1",
			expectedComponent: "vpc/security-group",
		},
		{
			name:              "empty stack flag",
			component:         "vpc",
			stack:             "",
			expectedComponent: "vpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk := NewTestKit(t)

			flags := &validateComponentFlags{
				stack: tt.stack,
			}

			// Mock resolver should not be called for non-path components.
			resolver := &mockComponentResolver{
				resolveFunc: func(component, stack string) (string, error) {
					t.Error("resolver should not be called for non-path component")
					return "", errors.New("unexpected call")
				},
			}

			result, err := resolvePathBasedComponent(tt.component, flags, resolver)

			require.NoError(tk, err)
			assert.Equal(tk, tt.expectedComponent, result)
		})
	}
}

func TestResolvePathBasedComponent_MissingStack(t *testing.T) {
	tk := NewTestKit(t)

	// Test path-based component without stack flag.
	component := "./components/terraform/vpc"
	flags := &validateComponentFlags{
		stack: "", // Missing stack.
	}

	resolver := &mockComponentResolver{}

	result, err := resolvePathBasedComponent(component, flags, resolver)

	assert.Error(tk, err)
	assert.Empty(tk, result)
	// Use sentinel error check for robust error type verification.
	assert.ErrorIs(tk, err, errUtils.ErrMissingStack)
	// Secondary check for user-facing message text.
	assert.Contains(tk, err.Error(), "--stack")
}

func TestResolvePathBasedComponent_ResolverError(t *testing.T) {
	tk := NewTestKit(t)

	// Test resolver returning an error.
	component := "./components/terraform/vpc"
	flags := &validateComponentFlags{
		stack: "prod-us-east-1",
	}

	expectedErr := errors.New("resolution failed")
	resolver := &mockComponentResolver{
		resolveFunc: func(component, stack string) (string, error) {
			return "", expectedErr
		},
	}

	result, err := resolvePathBasedComponent(component, flags, resolver)

	assert.Error(tk, err)
	assert.Empty(tk, result)
	assert.Equal(tk, expectedErr, err)
}

func TestDefaultComponentResolver_ResolveComponentPath(t *testing.T) {
	tk := NewTestKit(t)

	// This test verifies that DefaultComponentResolver can be instantiated.
	// Full integration testing is done in pkg/component/resolver tests.
	resolver := &DefaultComponentResolver{}
	assert.NotNil(tk, resolver)

	// The actual resolution requires a valid atmos config, which is tested in integration tests.
	// Here we just verify the interface is implemented.
	_, ok := interface{}(resolver).(ComponentResolver)
	assert.True(tk, ok, "DefaultComponentResolver should implement ComponentResolver")
}

func TestValidateComponentCmd_FlagsRegistered(t *testing.T) {
	// Test that all expected flags are registered.
	tests := []struct {
		flagName     string
		flagType     string
		defaultValue string
	}{
		{
			flagName:     "stack",
			flagType:     "string",
			defaultValue: "",
		},
		{
			flagName:     "schema-path",
			flagType:     "string",
			defaultValue: "",
		},
		{
			flagName:     "schema-type",
			flagType:     "string",
			defaultValue: "",
		},
		{
			flagName:     "module-paths",
			flagType:     "stringSlice",
			defaultValue: "[]",
		},
		{
			flagName:     "timeout",
			flagType:     "int",
			defaultValue: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.flagName, func(t *testing.T) {
			flag := validateComponentCmd.PersistentFlags().Lookup(tt.flagName)
			require.NotNil(t, flag, "flag %s should be registered", tt.flagName)
			assert.Equal(t, tt.flagType, flag.Value.Type(), "flag %s should have type %s", tt.flagName, tt.flagType)
			assert.Equal(t, tt.defaultValue, flag.DefValue, "flag %s should have default value %s", tt.flagName, tt.defaultValue)
		})
	}
}

func TestValidateComponentCmd_StackFlagRequired(t *testing.T) {
	// Test that the stack flag is marked as required.
	flag := validateComponentCmd.PersistentFlags().Lookup("stack")
	require.NotNil(t, flag, "stack flag should be registered")

	// Check if annotation exists for required flag.
	annotations := flag.Annotations
	_, hasRequired := annotations[cobra.BashCompOneRequiredFlag]
	assert.True(t, hasRequired, "stack flag should be marked as required")
}
