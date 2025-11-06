package describe

import "github.com/cloudposse/atmos/pkg/flags/global"

// StacksOptions contains parsed flags for the describe stacks command.
type StacksOptions struct {
	global.Flags
	Stack              string
	Format             string
	File               string
	ProcessTemplates   bool
	ProcessFunctions   bool
	Components         []string
	ComponentTypes     []string
	Sections           []string
	IncludeEmptyStacks bool
	Skip               []string
	Query              string
}
