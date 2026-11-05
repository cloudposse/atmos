// Package blobtest is the shared conformance suite for [blob.Store] implementations. Every backend
// runs [RunStoreConformance] from its own tests; behavior a backend does not advertise through
// [blob.Capabilities] is asserted to fail clearly instead of being skipped.
package blobtest
