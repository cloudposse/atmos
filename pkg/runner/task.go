package runner

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// Task is an alias for schema.Task.
// It represents a unit of work that the runner executes.
type Task = schema.Task

// Tasks is an alias for schema.Tasks.
// It supports flexible YAML parsing - both strings and structs.
type Tasks = schema.Tasks
