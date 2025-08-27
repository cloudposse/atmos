package filetype_test

import (
	"fmt"
	"os"

	"github.com/cloudposse/atmos/pkg/filetype"
)

// ExampleIsYAML demonstrates how to check if a string is valid YAML
func ExampleIsYAML() {
	input := `
name: example
values:
  - one
  - two
metadata:
  version: 1.0
`
	if filetype.IsYAML(input) {
		fmt.Println("Valid YAML")
	} else {
		fmt.Println("Invalid YAML")
	}
	// Output: Valid YAML
}

// ExampleIsJSON demonstrates how to check if a string is valid JSON
func ExampleIsJSON() {
	input := `{
  "name": "example",
  "values": ["one", "two"],
  "metadata": {
    "version": "1.0"
  }
}`
	if filetype.IsJSON(input) {
		fmt.Println("Valid JSON")
	} else {
		fmt.Println("Invalid JSON")
	}
	// Output: Valid JSON
}

// ExampleIsHCL demonstrates how to check if a string is valid HCL
func ExampleIsHCL() {
	input := `
name = "example"
values = ["one", "two"]
metadata = {
  version = "1.0"
}
`
	if filetype.IsHCL(input) {
		fmt.Println("Valid HCL")
	} else {
		fmt.Println("Invalid HCL")
	}
	// Output: Valid HCL
}

// ExampleDetectFormatAndParseFile demonstrates automatic format detection and parsing
func ExampleDetectFormatAndParseFile() {
	// Create a mock file reader for demonstration
	mockFileReader := func(filename string) ([]byte, error) {
		// In a real scenario, this would read from the file system
		if filename == "config.json" {
			return []byte(`{"setting": "value", "enabled": true}`), nil
		}
		return nil, os.ErrNotExist
	}

	// Parse the file with automatic format detection
	result, err := filetype.DetectFormatAndParseFile(mockFileReader, "config.json")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// The result is parsed into appropriate Go types
	if config, ok := result.(map[string]any); ok {
		fmt.Printf("Setting: %v\n", config["setting"])
		fmt.Printf("Enabled: %v\n", config["enabled"])
	}
	// Output:
	// Setting: value
	// Enabled: true
}

// Example_formatDetection shows how to detect and handle different file formats
func Example_formatDetection() {
	inputs := []struct {
		name    string
		content string
	}{
		{name: "config.yaml", content: "key: value"},
		{name: "config.json", content: `{"key": "value"}`},
		{name: "config.hcl", content: `key = "value"`},
		{name: "readme.txt", content: "Plain text content"},
	}

	for _, input := range inputs {
		switch {
		case filetype.IsJSON(input.content):
			fmt.Printf("%s is JSON\n", input.name)
		case filetype.IsYAML(input.content):
			fmt.Printf("%s is YAML\n", input.name)
		case filetype.IsHCL(input.content):
			fmt.Printf("%s is HCL\n", input.name)
		default:
			fmt.Printf("%s is plain text\n", input.name)
		}
	}
	// Output:
	// config.yaml is YAML
	// config.json is JSON
	// config.hcl is HCL
	// readme.txt is plain text
}
