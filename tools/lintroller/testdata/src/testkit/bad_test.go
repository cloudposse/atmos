package cmd

import (
	"testing"
)

// Mock types for testing.
type Command struct{}

func (c *Command) Commands() []*Command {
	return nil
}

func (c *Command) Execute() error {
	return nil
}

func (c *Command) SetArgs(args []string) {
}

// Mock RootCmd for testing purposes.
var RootCmd = &Command{}

// Test function READING RootCmd without modifying - this is OK, no TestKit needed.
func TestGoodReadOnly(t *testing.T) {
	_ = RootCmd.Commands() // Read-only access is fine.
}

// Test function using Execute without TestKit (testkit-required rule).
func TestBadExecute(t *testing.T) { // want "test function TestBadExecute modifies RootCmd state but does not call NewTestKit; use _ = NewTestKit\\(t\\) to ensure proper RootCmd state cleanup \\(only needed for Execute/SetArgs/ParseFlags/flag modifications, not read-only access\\)"
	_ = RootCmd.Execute()
}

// Test function using SetArgs without TestKit (testkit-required rule).
func TestBadSetArgs(t *testing.T) { // want "test function TestBadSetArgs modifies RootCmd state but does not call NewTestKit; use _ = NewTestKit\\(t\\) to ensure proper RootCmd state cleanup \\(only needed for Execute/SetArgs/ParseFlags/flag modifications, not read-only access\\)"
	RootCmd.SetArgs([]string{"arg1", "arg2"})
}

// Test function properly using TestKit when modifying RootCmd.
func TestGoodWithTestKit(t *testing.T) {
	_ = NewTestKit(t)
	RootCmd.SetArgs([]string{"test"})
}

// Helper function NewTestKit for testing.
func NewTestKit(t *testing.T) *testing.T {
	return t
}
