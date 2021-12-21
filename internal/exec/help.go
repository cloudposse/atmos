package exec

import "fmt"

// processHelp processes help commands
func processHelp(componentType string, command string) error {
	message := fmt.Sprintf("Help for atmos %s %s", componentType, command)
	fmt.Println(message)
	return nil
}
