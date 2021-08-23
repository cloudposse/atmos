package config

import (
	f "atmos/internal/utils"
	"encoding/json"
	"fmt"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
	"os"
	"path"
	"strings"
)

const (
	ConfigFilePath1 = "/usr/local/etc/atmos"
	ConfigFileName  = "atmos.yaml"
)

type Configuration struct {
	StackDir     string `mapstructure:"StackDir"`
	TerraformDir string `mapstructure:"TerraformDir"`
}

var (
	// Default values
	defaultConfig = map[string]interface{}{
		// Default path to stack configs
		"StackDir": "./stacks",
		// Default path to terraform components
		"TerraformDir": "./components/terraform",
	}

	// Config is the CLI configuration structure
	Config Configuration
)

// https://dev.to/techschoolguru/load-config-from-file-environment-variables-in-golang-with-viper-2j2d
// https://medium.com/@bnprashanth256/reading-configuration-files-and-environment-variables-in-go-golang-c2607f912b63
func InitConfig() error {
	// Config is loaded from these locations (from lower to higher priority):
	// /usr/local/etc/atmos
	// ~/.atmos
	// from the current directory
	// from ENV vars
	// from command-line arguments

	fmt.Println(strings.Repeat("-", 120))
	fmt.Println("Processing and merging configurations in the order of precedence...")

	v := viper.New()
	v.SetConfigType("yaml")

	// Add default config
	err := v.MergeConfigMap(defaultConfig)
	if err != nil {
		return err
	}

	// Process config in /usr/local/etc/atmos
	configFile1 := path.Join(ConfigFilePath1, ConfigFileName)
	err = processConfigFile(configFile1, v)
	if err != nil {
		return err
	}

	// Process config in user's HOME dir
	configFilePath2, err := homedir.Dir()
	if err != nil {
		return err
	}
	configFile2 := path.Join(configFilePath2, ".atmos", ConfigFileName)
	err = processConfigFile(configFile2, v)
	if err != nil {
		return err
	}

	// Process config in the current dir
	configFilePath3, err := os.Getwd()
	if err != nil {
		return err
	}
	configFile3 := path.Join(configFilePath3, ConfigFileName)
	err = processConfigFile(configFile3, v)
	if err != nil {
		return err
	}

	// https://gist.github.com/chazcheadle/45bf85b793dea2b71bd05ebaa3c28644
	// https://sagikazarmark.hu/blog/decoding-custom-formats-with-viper/
	err = v.Unmarshal(&Config)
	if err != nil {
		return err
	}

	fmt.Println("Final CLI configuration:")
	j, _ := json.MarshalIndent(&Config, "", "\t")
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", j)

	return nil
}

// https://github.com/NCAR/go-figure
// https://github.com/spf13/viper/issues/181
// https://medium.com/@bnprashanth256/reading-configuration-files-and-environment-variables-in-go-golang-c2607f912b63
func processConfigFile(path string, v *viper.Viper) error {
	if !f.FileExists(path) {
		fmt.Println("No config found at " + path)
		return nil
	}

	fmt.Println("Found config at " + path)

	reader, err := os.Open(path)
	if err != nil {
		return err
	}

	defer func(reader *os.File) {
		err := reader.Close()
		if err != nil {
			fmt.Println("Error closing file " + path + ". " + err.Error())
		}
	}(reader)

	err = v.MergeConfig(reader)
	if err != nil {
		return err
	}

	fmt.Println("Processed config at " + path)

	return nil
}
