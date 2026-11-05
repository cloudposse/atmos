package validator

import (
	"strings"

	"github.com/xeipuuv/gojsonschema"
)

const (
	// Field name that gojsonschema gives the document root.
	schemaRootField = "(root)"
	// Error type that gojsonschema reports for a failed anyOf keyword.
	anyOfErrorType = "number_any_of"
	// Error type that gojsonschema reports for a failed oneOf keyword.
	oneOfErrorType = "number_one_of"
)

// FilterCascadeErrors drops the generic anyOf and oneOf failures that only restate a more specific
// failure, so diagnostics point at the offending leaf value instead of every ancestor. The generated
// schemas wrap most values in anyOf or oneOf to admit YAML functions, null, and deprecated shapes, so
// one bad leaf otherwise produces a "Must validate one and only one schema" failure at each ancestor.
//
// A composite failure is dropped when another failure lies strictly below its path, or sits at its path
// and is not itself a composite failure. A composite failure with nothing more specific is kept.
func FilterCascadeErrors(failures []gojsonschema.ResultError) []gojsonschema.ResultError {
	kept := make([]gojsonschema.ResultError, 0, len(failures))
	for i, failure := range failures {
		if isCompositeError(failure) && hasMoreSpecificError(failures, i) {
			continue
		}
		kept = append(kept, failure)
	}
	return kept
}

func isCompositeError(failure gojsonschema.ResultError) bool {
	kind := failure.Type()
	return kind == anyOfErrorType || kind == oneOfErrorType
}

// hasMoreSpecificError reports whether a failure other than failures[index] lies below the path of
// failures[index], or at the same path and is not a composite failure.
func hasMoreSpecificError(failures []gojsonschema.ResultError, index int) bool {
	field := failures[index].Field()
	for i, other := range failures {
		if i == index {
			continue
		}
		switch {
		case isBelow(other.Field(), field):
			return true
		case other.Field() == field && !isCompositeError(other):
			return true
		}
	}
	return false
}

// isBelow reports whether child is a strict descendant of field. The root contains every other field.
func isBelow(child, field string) bool {
	if field == schemaRootField {
		return child != schemaRootField
	}
	return strings.HasPrefix(child, field+".")
}
