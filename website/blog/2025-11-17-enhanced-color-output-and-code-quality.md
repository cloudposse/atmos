---
slug: enhanced-color-output-and-code-quality
title: Enhanced Color Output Support and Code Quality Improvements
authors: [osterman]
tags: [cli, contributors]
---

This release brings powerful enhancements to color output in CI/CD environments and significant code quality improvements that make Atmos more maintainable and performant.

<!-- truncate -->

## Enhanced Color Output Support

Color output in CI/CD pipelines and non-TTY environments is now fully supported through the `ATMOS_FORCE_COLOR` environment variable:

```bash
# Force colored output in CI/CD
export ATMOS_FORCE_COLOR=true
atmos terraform plan myapp -s dev

# Supports truthy values: 1, true, yes, on, always, 2, 3
# Supports falsy values: 0, false, no, off
```

This enhancement ensures that Atmos help text, logs, and command output render beautifully even when piped or redirected, making debugging in CI/CD environments much easier.

## Code Quality Improvements

This release includes significant internal improvements:

### Reduced Cognitive Complexity
- Functions with cognitive complexity >15 have been refactored
- Better separation of concerns with single-responsibility functions
- Improved testability across the codebase

### Enhanced Performance Tracking
All public functions now include performance instrumentation:

```go
defer perf.Track(atmosConfig, "package.FunctionName")()
```

This allows us to identify bottlenecks and optimize performance in future releases.

### Better Error Handling
- Consistent use of static errors from `errors/errors.go`
- Proper error wrapping with context
- Use of `errors.Join()` for combining multiple errors.

## What's Next

We're continuing to improve Atmos with:
- Additional template functions for dynamic configurations
- Enhanced validation and policy enforcement
- Better integration with cloud provider CLIs
- More comprehensive testing infrastructure

## Upgrade Notes

This release maintains backward compatibility. To take advantage of the new features:

1. Update to the latest Atmos version
2. Consider using `ATMOS_FORCE_COLOR` in CI/CD pipelines for better debugging

## Contributors

This release includes contributions from the Atmos team and community. Thank you to everyone who provided feedback, reported issues, and contributed code!

For the complete list of changes, see the [GitHub release notes](https://github.com/cloudposse/atmos/releases).

---

Have questions or feedback? Join us on [Slack](https://slack.cloudposse.com/) or open an issue on [GitHub](https://github.com/cloudposse/atmos/issues).
