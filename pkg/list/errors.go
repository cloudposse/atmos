package list

import "fmt"

// ErrNoValuesFound is returned when no values are found for a component
type ErrNoValuesFound struct {
	Component string
}

func (e *ErrNoValuesFound) Error() string {
	return fmt.Sprintf("no values found for component '%s'", e.Component)
}

// IsNoValuesFoundError checks if an error is a NoValuesFound error
func IsNoValuesFoundError(err error) bool {
	_, ok := err.(*ErrNoValuesFound)
	return ok
}
