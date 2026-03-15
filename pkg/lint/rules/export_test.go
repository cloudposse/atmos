// Package rules exposes private helper functions for white-box testing.
// This file is compiled only during tests (note the _test.go suffix).
package rules

// ExportedFormatCyclePath exposes the private formatCyclePath function for testing.
var ExportedFormatCyclePath = formatCyclePath

// ExportedConcernGroup exposes the private concernGroup function for testing.
var ExportedConcernGroup = concernGroup

// ExportedNormalizeForComparison exposes the private normalizeForComparison function for testing.
var ExportedNormalizeForComparison = normalizeForComparison

// ExportedStackNameToFile exposes the private stackNameToFile function for testing.
var ExportedStackNameToFile = stackNameToFile
