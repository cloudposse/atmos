// config/config.go
package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var DefaultConfigHandler, err = New()

// ConfigHandler holds the application's configuration
type ConfigHandler struct {
	atmosConfig *schema.AtmosConfiguration
	v           *viper.Viper
}

// ConfigOptions defines options for adding a configuration parameter
type ConfigOptions struct {
	FlagName     string      // Custom flag name (optional)
	EnvVar       string      // Custom environment variable (optional)
	Description  string      // Flag description
	Key          string      // Key of the data in atmosConfiguration
	DefaultValue interface{} // Default value of the data in atmosConfiguration
}

// New creates a new Config instance with initialized Viper
func New() (*ConfigHandler, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetTypeByDefaultValue(true)
	configHandler := &ConfigHandler{
		v:           v,
		atmosConfig: &schema.AtmosConfiguration{},
	}
	return configHandler, configHandler.load()
}

// AddConfig adds a configuration parameter to both Cobra and Viper with options
func (c *ConfigHandler) AddConfig(cmd *cobra.Command, opts ConfigOptions) error {
	key := opts.Key
	defaultValue := opts.DefaultValue
	// Set default value in Viper
	c.v.SetDefault(key, defaultValue)

	// Determine flag name
	flagName := opts.FlagName
	if flagName == "" {
		flagName = strings.ReplaceAll(key, ".", "-")
	}

	// Register flag with Cobra
	flagSet := cmd.PersistentFlags()

	switch defaultValue.(type) {
	case string:
		flagSet.String(flagName, defaultValue.(string), opts.Description)
	case int:
		flagSet.Int(flagName, defaultValue.(int), opts.Description)
	case bool:
		flagSet.Bool(flagName, defaultValue.(bool), opts.Description)
	case []string:
		flagSet.StringSlice(flagName, defaultValue.([]string), opts.Description)
	default:
		return fmt.Errorf("unsupported type for key %s", key)
	}

	// Bind the flag to Viper
	if err := c.v.BindPFlag(key, flagSet.Lookup(flagName)); err != nil {
		return fmt.Errorf("failed to bind %s: %w", key, err)
	}

	// Handle environment variable binding
	if opts.EnvVar != "" {
		if err := c.v.BindEnv(key, opts.EnvVar); err != nil {
			return fmt.Errorf("failed to bind custom env var %s for %s: %w", opts.EnvVar, key, err)
		}
	} else {
		if err := c.v.BindEnv(key); err != nil {
			return fmt.Errorf("failed to bind default env var for %s: %w", key, err)
		}
	}

	return nil
}

// load reads and merges the configuration
func (c *ConfigHandler) load() error {
	// Read config file if exists (non-blocking)
	if err := loadConfigSources(c.v, ""); err != nil {
		return err
	}
	if c.v.ConfigFileUsed() != "" {
		// get dir of atmosConfigFilePath
		atmosConfigDir := filepath.Dir(c.v.ConfigFileUsed())
		c.atmosConfig.CliConfigPath = atmosConfigDir
		// Set the CLI config path in the atmosConfig struct
		if !filepath.IsAbs(c.atmosConfig.CliConfigPath) {
			absPath, err := filepath.Abs(c.atmosConfig.CliConfigPath)
			if err != nil {
				return err
			}
			c.atmosConfig.CliConfigPath = absPath
		}
	}
	// TODO: This is copy paste need to set this in the right place
	// We want the editorconfig color by default to be true
	c.atmosConfig.Validate.EditorConfig.Color = true

	viper.AutomaticEnv()

	// Unmarshal into AtmosConfiguration struct
	if err := c.v.Unmarshal(c.atmosConfig); err != nil {
		return err
	}
	c.atmosConfig.ProcessSchemas()

	return nil
}

// Get returns the populated AtmosConfiguration
func (c *ConfigHandler) Get() *schema.AtmosConfiguration {
	return c.atmosConfig
}

func (c *ConfigHandler) BindEnv(key ...string) {
	c.v.BindEnv(key...)
}

// GetString retrieves a string value from the config
func (c *ConfigHandler) GetString(key string) string {
	return c.v.GetString(key)
}

// GetInt retrieves an int value from the config
func (c *ConfigHandler) GetInt(key string) int {
	return c.v.GetInt(key)
}

// GetBool retrieves a bool value from the config
func (c *ConfigHandler) GetBool(key string) bool {
	return c.v.GetBool(key)
}

// GetStringSlice retrieves a string slice value from the config
func (c *ConfigHandler) SetDefault(key string, value any) {
	c.v.SetDefault(key, value)
}
