// Package cache provides neutral, source-scoped discovery snapshots shared by plugin discovery and
// artifact discovery.
//
// A [Store] owns one private directory. Each discovery source (a repository listing, a catalog)
// has one snapshot file whose name is derived from a scope hash plus the source kind and
// reference, so snapshots are isolated by effective configuration, authentication realm, and host
// bindings without ever storing a credential value. [Load] applies the shared policy: a fresh
// snapshot is served without contacting the source, an explicit refresh or a stale snapshot fetches
// a new one, offline mode serves only an existing valid snapshot, and a fetched snapshot replaces
// the old file atomically only after the fetch completed.
//
// The package knows nothing about plugin or artifact schemas. Callers embed [Header] first in their
// snapshot struct (so the historical JSON keys stay in order), supply a decoder that validates the
// content, and supply the fetch and policy callbacks.
package cache
