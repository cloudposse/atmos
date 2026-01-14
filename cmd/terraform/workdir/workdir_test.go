package workdir

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSetAtmosConfig(t *testing.T) {
	// Save original.
	original := atmosConfigPtr
	defer func() { atmosConfigPtr = original }()

	// Test setting config.
	config := &schema.AtmosConfiguration{
		BasePath: "/test/path",
	}
	SetAtmosConfig(config)

	assert.Equal(t, config, atmosConfigPtr)
	assert.Equal(t, "/test/path", atmosConfigPtr.BasePath)
}

func TestSetAtmosConfig_Nil(t *testing.T) {
	// Save original.
	original := atmosConfigPtr
	defer func() { atmosConfigPtr = original }()

	// Test setting nil config.
	SetAtmosConfig(nil)

	assert.Nil(t, atmosConfigPtr)
}

func TestSetAtmosConfig_MultipleUpdates(t *testing.T) {
	// Save original.
	original := atmosConfigPtr
	defer func() { atmosConfigPtr = original }()

	// Set first config.
	config1 := &schema.AtmosConfiguration{BasePath: "/path1"}
	SetAtmosConfig(config1)
	assert.Equal(t, "/path1", atmosConfigPtr.BasePath)

	// Set second config.
	config2 := &schema.AtmosConfiguration{BasePath: "/path2"}
	SetAtmosConfig(config2)
	assert.Equal(t, "/path2", atmosConfigPtr.BasePath)
}

func TestGetWorkdirCommand(t *testing.T) {
	cmd := GetWorkdirCommand()

	assert.NotNil(t, cmd)
	assert.Equal(t, "workdir", cmd.Use)
	assert.Equal(t, "Manage component working directories", cmd.Short)

	// Check subcommands are registered.
	subcommands := cmd.Commands()
	subcommandNames := make([]string, len(subcommands))
	for i, sub := range subcommands {
		subcommandNames[i] = sub.Name()
	}

	assert.Contains(t, subcommandNames, "list")
	assert.Contains(t, subcommandNames, "describe")
	assert.Contains(t, subcommandNames, "show")
	assert.Contains(t, subcommandNames, "clean")
}

func TestGetWorkdirCommand_HasSubcommands(t *testing.T) {
	cmd := GetWorkdirCommand()

	// Verify subcommands are registered.
	subcommands := cmd.Commands()
	assert.GreaterOrEqual(t, len(subcommands), 4)

	// Check for expected subcommands by Use string.
	foundList := false
	foundDescribe := false
	foundShow := false
	foundClean := false

	for _, sub := range subcommands {
		switch sub.Use {
		case "list":
			foundList = true
		case "describe <component>":
			foundDescribe = true
		case "show <component>":
			foundShow = true
		case "clean [component]":
			foundClean = true
		}
	}

	assert.True(t, foundList, "list subcommand should be registered")
	assert.True(t, foundDescribe, "describe subcommand should be registered")
	assert.True(t, foundShow, "show subcommand should be registered")
	assert.True(t, foundClean, "clean subcommand should be registered")
}

func TestBuildConfigAndStacksInfoFromFlags(t *testing.T) {
	tests := []struct {
		name     string
		flags    *global.Flags
		expected schema.ConfigAndStacksInfo
	}{
		{
			name:     "nil flags returns empty struct",
			flags:    nil,
			expected: schema.ConfigAndStacksInfo{},
		},
		{
			name: "all flags set",
			flags: &global.Flags{
				BasePath:   "/custom/base/path",
				Config:     []string{"config1.yaml", "config2.yaml"},
				ConfigPath: []string{"/path/one", "/path/two"},
				Profile:    []string{"prod", "us-east-1"},
			},
			expected: schema.ConfigAndStacksInfo{
				AtmosBasePath:           "/custom/base/path",
				AtmosConfigFilesFromArg: []string{"config1.yaml", "config2.yaml"},
				AtmosConfigDirsFromArg:  []string{"/path/one", "/path/two"},
				ProfilesFromArg:         []string{"prod", "us-east-1"},
			},
		},
		{
			name: "only base path set",
			flags: &global.Flags{
				BasePath: "/only/base",
			},
			expected: schema.ConfigAndStacksInfo{
				AtmosBasePath: "/only/base",
			},
		},
		{
			name: "only config files set",
			flags: &global.Flags{
				Config: []string{"custom.yaml"},
			},
			expected: schema.ConfigAndStacksInfo{
				AtmosConfigFilesFromArg: []string{"custom.yaml"},
			},
		},
		{
			name: "only config path set",
			flags: &global.Flags{
				ConfigPath: []string{"/config/dir"},
			},
			expected: schema.ConfigAndStacksInfo{
				AtmosConfigDirsFromArg: []string{"/config/dir"},
			},
		},
		{
			name: "only profile set",
			flags: &global.Flags{
				Profile: []string{"dev"},
			},
			expected: schema.ConfigAndStacksInfo{
				ProfilesFromArg: []string{"dev"},
			},
		},
		{
			name:     "empty flags",
			flags:    &global.Flags{},
			expected: schema.ConfigAndStacksInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildConfigAndStacksInfoFromFlags(tt.flags)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildConfigAndStacksInfo(t *testing.T) {
	// Create a minimal command for testing.
	cmd := &cobra.Command{
		Use: "test",
	}

	// Create viper instance.
	v := viper.New()

	// buildConfigAndStacksInfo calls ParseGlobalFlags which reads from cmd flags.
	// Since no flags are registered, it returns empty/default values.
	result := buildConfigAndStacksInfo(cmd, v)

	// Should return a ConfigAndStacksInfo struct.
	assert.IsType(t, schema.ConfigAndStacksInfo{}, result)
}

func TestWorkdirCmd_Structure(t *testing.T) {
	assert.Equal(t, "workdir", workdirCmd.Use)
	assert.Equal(t, "Manage component working directories", workdirCmd.Short)
	assert.Contains(t, workdirCmd.Long, "List, describe, show, and clean")
}

func TestWorkdirCmd_IsNotRunnable(t *testing.T) {
	// The parent workdir command should not be runnable directly.
	// It's a container for subcommands.
	assert.Nil(t, workdirCmd.Run)
	assert.Nil(t, workdirCmd.RunE)
}
