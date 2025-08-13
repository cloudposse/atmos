package main

import (
	"os"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/experiments/init/internal/initcmd"
	"github.com/cloudposse/atmos/experiments/init/internal/scaffoldcmd"
)

var (
	cfgFile  string
	logLevel string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "poc",
		Short: "Atmos Init Experiment - Initialize scaffold template configurations and examples",
		Long: `Atmos Init Experiment

The atmos init command initializes configurations and examples for Atmos projects.

This is an experimental implementation using Go embeds to ship configurations
that match the version of Atmos in use.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Configure logging
			level, err := log.ParseLevel(logLevel)
			if err != nil {
				level = log.InfoLevel
			}
			logger := log.NewWithOptions(os.Stderr, log.Options{
				ReportCaller:    false,
				ReportTimestamp: true,
				Level:           level,
			})
			log.SetDefault(logger)

			// Initialize viper configuration
			if cfgFile != "" {
				viper.SetConfigFile(cfgFile)
			} else {
				viper.SetConfigName("atmos")
				viper.SetConfigType("yaml")
				viper.AddConfigPath(".")
			}

			// Set up environment variable bindings
			viper.SetEnvPrefix("ATMOS")
			viper.AutomaticEnv()
			viper.BindEnv("accessible", "ATMOS_ACCESSIBLE")

			if err := viper.ReadInConfig(); err != nil {
				if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
					log.Error("Error reading config file", "error", err)
				}
			}
		},
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./atmos.yaml)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")

	// Add init command
	initCmd := initcmd.NewInitCmd()
	rootCmd.AddCommand(initCmd)

	// Add scaffold command
	rootCmd.AddCommand(scaffoldcmd.ScaffoldCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
