// Package compat holds test-only compatibility fixtures that freeze the byte-level behavior of the
// existing CI planfile archive format and the existing plugin OCI release format before the
// artifact implementation replaces any of their code paths.
//
// It lives apart from pkg/artifact so the core domain package never imports pkg/ci or pkg/plugin.
// Fixture bytes under testdata are committed and must never be regenerated to hide an
// incompatible change; see compat_test.go for the generation rules.
package compat
