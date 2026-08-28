// Package parser owns the grammars used by Atmos configuration functions.
//
// It deliberately parses only function arguments. YAML parsing, template rendering,
// and expression evaluation remain the responsibility of their existing layers.
// Callers receive typed argument values and can apply execution-context defaults.
package parser
