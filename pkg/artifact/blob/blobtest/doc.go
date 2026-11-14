// Package blobtest is the shared conformance suite for [blob.Store] implementations. Every backend
// runs [RunStoreConformance] from its own tests; unsupported behavior is checked for clear errors where the suite can assert them; other
// cases absent from [blob.Capabilities] are skipped.
package blobtest
