package errors

import "github.com/pkg/errors"

// ErrPlanHasDiff is returned when there are differences between two Terraform plan files.
var ErrPlanHasDiff = errors.New("plan files have differences")
