package cmd

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRegisterSemanticFlagCompletions(t *testing.T) {
	_ = NewTestKit(t)

	tests := []struct {
		name                string
		commandConfig       *schema.Command
		flagsToCreate       []string
		expectedCompletions []string
	}{
		{
			name: "registers stack completion",
			commandConfig: &schema.Command{
				Component: &schema.CommandComponent{Type: "script"},
				Flags: []schema.CommandFlag{
					{Name: "stack", SemanticType: "stack"},
				},
			},
			flagsToCreate:       []string{"stack"},
			expectedCompletions: []string{"stack"},
		},
		{
			name: "registers component completion",
			commandConfig: &schema.Command{
				Component: &schema.CommandComponent{Type: "script"},
				Flags: []schema.CommandFlag{
					{Name: "component", SemanticType: "component"},
				},
			},
			flagsToCreate:       []string{"component"},
			expectedCompletions: []string{"component"},
		},
		{
			name: "skips non-semantic flags",
			commandConfig: &schema.Command{
				Component: &schema.CommandComponent{Type: "script"},
				Flags: []schema.CommandFlag{
					{Name: "verbose", SemanticType: ""},
					{Name: "output", SemanticType: "string"},
				},
			},
			flagsToCreate:       []string{"verbose", "output"},
			expectedCompletions: []string{}, // Neither should have completion.
		},
		{
			name:                "nil component returns early",
			commandConfig:       &schema.Command{},
			flagsToCreate:       []string{},
			expectedCompletions: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			for _, flagName := range tt.flagsToCreate {
				cmd.PersistentFlags().String(flagName, "", flagName+" flag")
			}

			registerSemanticFlagCompletions(cmd, tt.commandConfig)

			for _, expectedFlag := range tt.expectedCompletions {
				_, hasCompletion := cmd.GetFlagCompletionFunc(expectedFlag)
				assert.True(t, hasCompletion, "Flag %q should have completion function", expectedFlag)
			}
		})
	}
}

func TestSetSemanticArgCompletion(t *testing.T) {
	_ = NewTestKit(t)

	tests := []struct {
		name          string
		commandConfig *schema.Command
		wantSet       bool
	}{
		{
			name: "sets completion for component type",
			commandConfig: &schema.Command{
				Component: &schema.CommandComponent{Type: "script"},
				Arguments: []schema.CommandArgument{{Name: "app", Type: "component"}},
			},
			wantSet: true,
		},
		{
			name: "sets completion for stack type",
			commandConfig: &schema.Command{
				Component: &schema.CommandComponent{Type: "script"},
				Arguments: []schema.CommandArgument{{Name: "stack", Type: "stack"}},
			},
			wantSet: true,
		},
		{
			name: "does not set for string type",
			commandConfig: &schema.Command{
				Component: &schema.CommandComponent{Type: "script"},
				Arguments: []schema.CommandArgument{{Name: "name", Type: "string"}},
			},
			wantSet: false,
		},
		{
			name:          "nil component returns early",
			commandConfig: &schema.Command{},
			wantSet:       false,
		},
		{
			name: "empty arguments returns early",
			commandConfig: &schema.Command{
				Component: &schema.CommandComponent{Type: "script"},
				Arguments: []schema.CommandArgument{},
			},
			wantSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}

			setSemanticArgCompletion(cmd, tt.commandConfig)

			if tt.wantSet {
				assert.NotNil(t, cmd.ValidArgsFunction, "ValidArgsFunction should be set")
			} else {
				assert.Nil(t, cmd.ValidArgsFunction, "ValidArgsFunction should not be set")
			}
		})
	}
}

func TestPromptSemanticArguments(t *testing.T) {
	tests := []struct {
		name           string
		arg            schema.CommandArgument
		existingValue  string
		mockComponents []string
		mockStacks     []string
		promptResult   string
		wantValue      string
	}{
		{
			name:      "skips non-required argument",
			arg:       schema.CommandArgument{Name: "comp", Type: "component", Required: false},
			wantValue: "",
		},
		{
			name:           "prompts for component argument",
			arg:            schema.CommandArgument{Name: "comp", Type: "component", Required: true},
			mockComponents: []string{"app1", "app2"},
			promptResult:   "app1",
			wantValue:      "app1",
		},
		{
			name:         "prompts for stack argument",
			arg:          schema.CommandArgument{Name: "stack", Type: "stack", Required: true},
			mockStacks:   []string{"dev", "prod"},
			promptResult: "dev",
			wantValue:    "dev",
		},
		{
			name:          "skips when value already exists",
			arg:           schema.CommandArgument{Name: "comp", Type: "component", Required: true},
			existingValue: "existing",
			wantValue:     "existing",
		},
		{
			name:           "handles empty components list",
			arg:            schema.CommandArgument{Name: "comp", Type: "component", Required: true},
			mockComponents: []string{}, // Empty.
			wantValue:      "",         // No prompt, no value.
		},
		{
			name:      "skips unknown type",
			arg:       schema.CommandArgument{Name: "name", Type: "string", Required: true},
			wantValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			mockCfg := &PromptConfig{
				ListComponents: func(_ context.Context, _ string, _ map[string]any) ([]string, error) {
					return tt.mockComponents, nil
				},
				ListStacks: func(_ *cobra.Command) ([]string, error) {
					return tt.mockStacks, nil
				},
				PromptArg: func(_, _ string, _ flags.CompletionFunc, _ *cobra.Command, _ []string) (string, error) {
					return tt.promptResult, nil
				},
				PromptFlag: func(_, _ string, _ flags.CompletionFunc, _ *cobra.Command, _ []string) (string, error) {
					return tt.promptResult, nil
				},
			}

			argumentsData := map[string]string{}
			if tt.existingValue != "" {
				argumentsData[tt.arg.Name] = tt.existingValue
			}

			commandConfig := &schema.Command{
				Component: &schema.CommandComponent{Type: "script"},
				Arguments: []schema.CommandArgument{tt.arg},
			}

			promptSemanticArguments(nil, commandConfig, argumentsData, nil, mockCfg)

			if tt.wantValue == "" {
				if tt.existingValue != "" {
					assert.Equal(t, tt.existingValue, argumentsData[tt.arg.Name])
				} else {
					assert.Empty(t, argumentsData[tt.arg.Name])
				}
			} else {
				assert.Equal(t, tt.wantValue, argumentsData[tt.arg.Name])
			}
		})
	}
}

func TestPromptSemanticFlags(t *testing.T) {
	tests := []struct {
		name           string
		flag           schema.CommandFlag
		existingValue  string
		mockComponents []string
		mockStacks     []string
		promptResult   string
		wantValue      string
	}{
		{
			name:      "skips non-required flag",
			flag:      schema.CommandFlag{Name: "comp", SemanticType: "component", Required: false},
			wantValue: "",
		},
		{
			name:           "prompts for component flag",
			flag:           schema.CommandFlag{Name: "comp", SemanticType: "component", Required: true},
			mockComponents: []string{"app1", "app2"},
			promptResult:   "app1",
			wantValue:      "app1",
		},
		{
			name:         "prompts for stack flag",
			flag:         schema.CommandFlag{Name: "stack", SemanticType: "stack", Required: true},
			mockStacks:   []string{"dev", "prod"},
			promptResult: "dev",
			wantValue:    "dev",
		},
		{
			name:          "skips when value already exists",
			flag:          schema.CommandFlag{Name: "comp", SemanticType: "component", Required: true},
			existingValue: "existing",
			wantValue:     "existing",
		},
		{
			name:      "skips unknown semantic type",
			flag:      schema.CommandFlag{Name: "name", SemanticType: "", Required: true},
			wantValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			mockCfg := &PromptConfig{
				ListComponents: func(_ context.Context, _ string, _ map[string]any) ([]string, error) {
					return tt.mockComponents, nil
				},
				ListStacks: func(_ *cobra.Command) ([]string, error) {
					return tt.mockStacks, nil
				},
				PromptArg: func(_, _ string, _ flags.CompletionFunc, _ *cobra.Command, _ []string) (string, error) {
					return tt.promptResult, nil
				},
				PromptFlag: func(_, _ string, _ flags.CompletionFunc, _ *cobra.Command, _ []string) (string, error) {
					return tt.promptResult, nil
				},
			}

			flagsData := map[string]any{}
			if tt.existingValue != "" {
				flagsData[tt.flag.Name] = tt.existingValue
			}

			commandConfig := &schema.Command{
				Component: &schema.CommandComponent{Type: "script"},
				Flags:     []schema.CommandFlag{tt.flag},
			}

			promptSemanticFlags(nil, commandConfig, flagsData, nil, mockCfg)

			if tt.wantValue == "" {
				if tt.existingValue != "" {
					assert.Equal(t, tt.existingValue, flagsData[tt.flag.Name])
				} else {
					_, exists := flagsData[tt.flag.Name]
					assert.False(t, exists, "Flag should not exist in map")
				}
			} else {
				assert.Equal(t, tt.wantValue, flagsData[tt.flag.Name])
			}
		})
	}
}

func TestPromptForSemanticValues_NilComponent(t *testing.T) {
	_ = NewTestKit(t)

	// Should return early without error when Component is nil.
	commandConfig := &schema.Command{}
	argumentsData := map[string]string{}
	flagsData := map[string]any{}

	// This should not panic.
	promptForSemanticValues(nil, commandConfig, argumentsData, flagsData, nil)

	assert.Empty(t, argumentsData)
	assert.Empty(t, flagsData)
}
