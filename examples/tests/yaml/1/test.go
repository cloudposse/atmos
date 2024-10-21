package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"strconv"
	"strings"
)

// Define a structure that includes a dynamic vars section
type Config struct {
	Name string                 `yaml:"name"`
	Age  int                    `yaml:"age"`
	Vars map[string]interface{} `yaml:"vars"`
}

// Helper function to evaluate lazy expressions
func parseLazy(node *yaml.Node) interface{} {
	if node.Tag == "!!lazy" {
		// Return a lambda function to evaluate the lazy expression
		return func() int {
			parts := strings.Split(node.Value, " + ")
			if len(parts) != 2 {
				fmt.Println("Invalid lazy expression")
				return 0
			}
			a, _ := strconv.Atoi(parts[0])
			b, _ := strconv.Atoi(parts[1])
			return a + b
		}
	}
	return node.Value
}

func main() {
	// Sample YAML data
	data := `
name: "Sample Config"
age: 30
vars:
  customVar1: "some string"
  customVar2: 42
  customLazy: !!lazy "5 + 3"
`

	// Unmarshal the YAML into a map[string]interface{} to process vars dynamically
	var rawYAML map[string]*yaml.Node
	if err := yaml.Unmarshal([]byte(data), &rawYAML); err != nil {
		panic(err)
	}

	// Parse the known fields directly
	config := Config{
		Name: rawYAML["name"].Value,
		Age:  mustAtoi(rawYAML["age"].Value),
		Vars: make(map[string]interface{}),
	}

	// Process the dynamic vars section
	varsNode := rawYAML["vars"]
	for _, node := range varsNode.Content {
		key := node.Value
		valueNode := node
		config.Vars[key] = parseLazy(valueNode)
	}

	// Access and print the regular and custom vars
	fmt.Println("Name:", config.Name)
	fmt.Println("Age:", config.Age)
	for key, value := range config.Vars {
		if lazyFunc, ok := value.(func() int); ok {
			fmt.Printf("%s (lazy): %d\n", key, lazyFunc())
		} else {
			fmt.Printf("%s: %v\n", key, value)
		}
	}
}

// Helper function to convert string to int
func mustAtoi(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}
