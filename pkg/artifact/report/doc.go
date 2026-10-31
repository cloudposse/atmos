// Package report defines the seam through which artifact operations report their results to a
// summary renderer.
//
// This package holds only the [Collector] interface and a no-op default. Execution boundaries
// (the artifact step handler, and in the future the CLI) record each completed or failed
// operation into an injected collector; a renderer implements the interface and decides what to
// show. Collectors are passed explicitly through runner services and options, never through
// context values.
package report
