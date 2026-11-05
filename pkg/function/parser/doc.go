// Package parser owns the grammars used by Atmos configuration functions.
//
// It parses function arguments and encodes their tagged YAML forms for deferred
// evaluation. Stack document loading, template rendering, and expression evaluation
// remain the responsibility of their existing layers.
// Callers receive typed argument values and can apply execution-context defaults.
package parser
