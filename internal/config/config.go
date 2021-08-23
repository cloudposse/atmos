package config

import (
	f "atmos/internal/utils"
	"fmt"
	"github.com/mitchellh/go-homedir"
	"os"
	"path"
	"strings"
)

const (
	ConfigFilePath1 = "/usr/local/etc/atmos"
	ConfigFileName  = "atmos.yaml"
)

type Configuration struct {
	StackDir     string
	TerraformDir string
}

var (
	// Default path to stack configs
	stackDir = "./stacks"

	// Default path to terraform components
	terraformDir = "./components/terraform"

	// Config is the CLI configuration structure
	Config Configuration
)

func InitConfig() error {
	// Config is loaded from these locations (from lower to higher priority):
	// /usr/local/etc/atmos
	// ~/.atmos
	// from the current directory
	// from ENV vars
	// from command-line arguments

	fmt.Println(strings.Repeat("-", 120))
	fmt.Println("Processing and merging configurations in the order of precedence...")

	// Process config in /usr/local/etc/atmos
	configFile1 := path.Join(ConfigFilePath1, ConfigFileName)
	err := processConfigFile(configFile1)
	if err != nil {
		return err
	}

	// Process config in user's HOME dir
	configFilePath2, err := homedir.Dir()
	if err != nil {
		return err
	}
	configFile2 := path.Join(configFilePath2, ".atmos", ConfigFileName)
	err = processConfigFile(configFile2)
	if err != nil {
		return err
	}

	// Process config in the current dir
	configFilePath3, err := os.Getwd()
	if err != nil {
		return err
	}
	configFile3 := path.Join(configFilePath3, ConfigFileName)
	err = processConfigFile(configFile3)
	if err != nil {
		return err
	}

	if len(Config.StackDir) == 0 {
		Config.StackDir = stackDir
	}

	if len(Config.TerraformDir) == 0 {
		Config.TerraformDir = terraformDir
	}

	return nil
}

func processConfigFile(path string) error {
	if !f.FileExists(path) {
		fmt.Println("No config found at " + path)
		return nil
	}

	fmt.Println("Found config at " + path)

	return nil
}
