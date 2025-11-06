package describe

import (
	"github.com/cloudposse/atmos/pkg/flags"
)

// StacksOptions contains parsed flags for the describe stacks command.
type StacksOptions struct {
	flags.GlobalFlags
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
