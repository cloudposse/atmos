// Package invoke is the transport-neutral request and result mapping of the artifact operations.
//
// The CLI, workflow and custom-command steps, script step calls, and kind: step hooks all describe
// an operation as [Params], run it with [Run], and report the typed [Result] or [Collection]. One
// implementation builds the service requests, dispatches the action, populates the result, and
// names the outputs, so every entry point returns identical payloads, named outputs, and field
// names. Presentation (flags, text rendering, step output modes) stays with each entry point.
//
// The package depends only on the artifact domain. Production service assembly, which needs the
// plugin-aware registry preparation and the managed signer, lives in pkg/artifactwire.
package invoke
