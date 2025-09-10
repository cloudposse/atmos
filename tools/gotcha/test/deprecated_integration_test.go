//go:build gotcha_binary_integration
// +build gotcha_binary_integration

// Package test contains integration tests for gotcha.
//
// DEPRECATED: These tests build and run the gotcha binary, which is slow
// and causes issues in CI. New tests should use the testdata approach
// demonstrated in parser_integration_test.go instead.
//
// To run these deprecated tests:
//
//	go test -tags=gotcha_binary_integration ./test
package test

// This file serves as documentation for the deprecated tests.
// The following test files require the gotcha_binary_integration build tag:
//
// - actual_bug_reproduction_test.go
// - comprehensive_bug_test.go
// - config_integration_test.go
// - reproduce_show_failed_bug_test.go
// - tui_progress_test.go
// - tui_subtest_integration_test.go
//
// These tests are preserved for historical reference and complex
// integration testing scenarios, but should not be used as examples
// for new tests.
