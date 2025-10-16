---
slug: enhanced-cli-experience-and-documentation
title: Enhanced CLI Experience and Comprehensive Terraform Documentation
authors: [atmos]
tags: [atmos, terraform, cli, documentation, ux]
---

We're excited to announce a major update to Atmos that significantly improves the CLI experience, documentation, and code quality. This release focuses on making Atmos more accessible, maintainable, and powerful for managing your cloud infrastructure.

<!-- truncate -->

## Complete Terraform Command Documentation

We've added comprehensive documentation for all Terraform commands integrated with Atmos. Each command now includes:

- **Detailed usage examples** with real-world scenarios
- **Complete flag reference** with descriptions and defaults
- **Atmos-specific behavior** explanations
- **Visual screengrabs** showing actual CLI output
- **Backend configuration** details and limitations

Key additions include:
- Terraform planfile workflow documentation with security considerations
- All Terraform subcommands (`plan`, `apply`, `destroy`, `validate`, `fmt`, `output`, etc.)
- Best practices for using Terraform with Atmos

[View Terraform Documentation](/cli/commands/terraform/usage)

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

## Improved Help System

The help system has been completely refactored for better maintainability and user experience:

- **Reduced cognitive complexity** - Large functions split into smaller, testable units
- **Consistent formatting** - All help text now uses proper markdown rendering
- **Better color profiles** - Automatic detection with TrueColor, ANSI256, and ASCII fallbacks
- **Performance tracking** - Built-in instrumentation for monitoring command execution

Example improvements:
- `renderFlags()` function reduced from 94 lines to focused 20-line implementation
- `configureWriter()` split into 5 specialized functions
- Added comprehensive unit tests for help rendering logic

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
- Use of `errors.Join()` for combining multiple errors

## Screengrab Generation Infrastructure

We've built a complete system for generating accurate, up-to-date CLI help screengrabs:

- **Automated generation** - All help screengrabs generated from actual CLI output
- **Color preservation** - ANSI colors converted to HTML for documentation
- **Docker/Podman support** - Cross-platform screengrab generation
- **CI/CD integration** - GitHub Actions workflow for automatic updates

This ensures our documentation always reflects the actual CLI behavior.

## Documentation Fixes

Numerous documentation improvements including:

- Fixed broken links across the documentation site
- Corrected Terraform command examples
- Updated helmfile sync description to accurately reflect behavior
- Added security warnings for credential handling in planfiles
- Improved markdown formatting consistency

## What's Next

We're continuing to improve Atmos with:
- Additional template functions for dynamic configurations
- Enhanced validation and policy enforcement
- Better integration with cloud provider CLIs
- More comprehensive testing infrastructure

## Upgrade Notes

This release maintains backward compatibility. To take advantage of the new features:

1. Update to the latest Atmos version
2. Review the new [Terraform documentation](/cli/commands/terraform/usage)
3. Consider using `ATMOS_FORCE_COLOR` in CI/CD pipelines
4. Check your workflows against the updated command descriptions

## Contributors

This release includes contributions from the Atmos team and community. Thank you to everyone who provided feedback, reported issues, and contributed code!

For the complete list of changes, see the [GitHub release notes](https://github.com/cloudposse/atmos/releases).

---

Have questions or feedback? Join us on [Slack](https://slack.cloudposse.com/) or open an issue on [GitHub](https://github.com/cloudposse/atmos/issues).
