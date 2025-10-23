package cmd

import (
	"testing"
)

// Mock types for testing.
type Command struct{}

func (c *Command) Execute() error {
	return nil
}

func (c *Command) SetArgs(args []string) {
}

// Mock RootCmd for testing purposes.
var RootCmd = &Command{}

// Test function using RootCmd without TestKit (testkit-required rule).
func TestBadRootCmd(t *testing.T) { // want "test function TestBadRootCmd uses RootCmd but does not call NewTestKit; use t := NewTestKit\\(t\\) to ensure proper RootCmd state cleanup"
	_ = RootCmd
}

// Test function using Execute without TestKit (testkit-required rule).
func TestBadExecute(t *testing.T) { // want "test function TestBadExecute uses RootCmd but does not call NewTestKit; use t := NewTestKit\\(t\\) to ensure proper RootCmd state cleanup"
	_ = RootCmd.Execute()
}

// Test function using SetArgs without TestKit (testkit-required rule).
func TestBadSetArgs(t *testing.T) { // want "test function TestBadSetArgs uses RootCmd but does not call NewTestKit; use t := NewTestKit\\(t\\) to ensure proper RootCmd state cleanup"
	RootCmd.SetArgs([]string{"arg1", "arg2"})
}

// Test function properly using TestKit.
func TestGoodWithTestKit(t *testing.T) {
	t = NewTestKit(t)
	_ = RootCmd
}

// Helper function NewTestKit for testing.
func NewTestKit(t *testing.T) *testing.T {
	return t
}
