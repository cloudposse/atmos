---
slug: enhanced-error-handling-sentry-integration
title: Enhanced Error Handling & Sentry Integration
authors: [atmos]
tags: [atmos, errors, observability, sentry, developer-experience]
---

# Making Errors Work For You: Enhanced Error Handling in Atmos

We're excited to announce a major improvement to how Atmos handles and reports errors. This enhancement focuses on making errors more helpful, more actionable, and more observable—so you spend less time debugging and more time shipping.

## The Problem: Generic Errors Don't Help

If you've worked with complex infrastructure tooling, you've probably encountered errors like this:

```
Error: failed to process component
```

What failed? Why did it fail? What should you do about it? These generic errors force you to dig through logs, check configurations, and experiment with fixes—wasting precious time.

## Our Solution: Errors That Guide You

We've redesigned Atmos error handling from the ground up with three core principles:

### 1. Rich Context

Every error now includes detailed context about what was happening when it failed:

```
Error: component not found in stack

Explanation:
Component 'vpc' could not be found in stack 'prod/us-east-1'

Context:
  component: vpc
  stack: prod/us-east-1
  config_file: stacks/prod/us-east-1.yaml

Hints:
💡 Use 'atmos list components --stack prod/us-east-1' to see available components
💡 Check that the component is defined in your stack configuration
```

No more guessing what went wrong—the error tells you exactly what happened and where.

### 2. Actionable Hints

The most frustrating errors are the ones that don't tell you how to fix them. Every Atmos error now includes specific, actionable hints:

- **Commands to run** to investigate the issue
- **Configuration to check** that might be misconfigured
- **Links to documentation** for complex scenarios
- **Next steps** to resolve the problem

These aren't generic suggestions—they're context-aware guidance based on the specific error and your configuration.

### 3. Observability with Sentry Integration

The best error messages help you fix problems before users report them. That's why we've integrated Sentry error tracking into Atmos.

#### What This Means For You

**For Development Teams:**
- **See errors before they become incidents** - Get notified when errors occur in CI/CD pipelines
- **Understand error patterns** - See which errors are most common and prioritize fixes
- **Debug faster** - Full stack traces and context automatically captured
- **Track error trends** - Monitor error rates over time to catch regressions early

**For Platform Teams:**
- **Centralized error monitoring** - All Atmos errors across your organization in one place
- **Smart alerting** - Get notified about new errors or error spikes
- **Team collaboration** - Assign errors, track fixes, and document resolutions
- **Privacy-first** - Sensitive data is automatically filtered before sending to Sentry

**For Operations:**
- **Production visibility** - Know immediately when infrastructure operations fail
- **Historical analysis** - Review past errors to understand failure patterns
- **Correlation** - Link errors to specific deployments, components, or stacks
- **Zero configuration** - Works out of the box with your existing Sentry account

## How It Works

### Getting Started

Enable Sentry integration in your `atmos.yaml`:

```yaml
errors:
  sentry:
    enabled: true
    dsn: "https://your-sentry-dsn@sentry.io/project"
    environment: "production"  # or staging, development, etc.
    traces_sample_rate: 0.1    # Capture 10% for performance monitoring
```

That's it! Atmos will now automatically report errors to Sentry with full context.

### Privacy & Security

We take privacy seriously. The Sentry integration:

- **Filters sensitive data** automatically (secrets, tokens, API keys)
- **Requires explicit opt-in** - disabled by default
- **Respects your Sentry settings** - uses your organization's data scrubbing rules
- **Is fully configurable** - control what gets sent and what stays local

### Verbose Mode for Local Debugging

Need even more detail? Enable verbose error mode:

```bash
atmos terraform plan vpc -s prod --verbose
```

Verbose mode shows:
- Full stack traces
- Detailed error chains
- Internal debugging information
- Performance metrics

Perfect for troubleshooting complex issues locally, while keeping production logs clean.

## Better Errors = Better Experience

These improvements aren't just about error messages—they're about respecting your time and making Atmos a better tool to work with:

- **Faster debugging** - Find root causes in seconds, not hours
- **Reduced support burden** - Self-service troubleshooting with helpful hints
- **Proactive monitoring** - Catch problems before they impact users
- **Knowledge sharing** - Document error resolutions in Sentry for the whole team
- **Continuous improvement** - Use error analytics to prioritize bug fixes

## What's Next

This is just the beginning. We're continuing to improve error handling with:

- **More granular error types** for specific failure scenarios
- **Integration with other observability platforms** (DataDog, Honeycomb, etc.)
- **AI-powered error analysis** to suggest fixes automatically
- **Error recovery strategies** for common failure patterns

## Try It Today

These enhancements are available now in the latest version of Atmos. To get started:

1. **Update Atmos** to the latest version
2. **Configure Sentry** (optional but recommended) in your `atmos.yaml`
3. **Run any command** and see the improved error messages

We believe good error handling is a feature, not an afterthought. These improvements make Atmos more reliable, more debuggable, and more pleasant to use.

## Feedback Welcome

Have thoughts on the new error handling? Found an error message that could be more helpful? [Open an issue](https://github.com/cloudposse/atmos/issues) or join the discussion in our [Community Slack](https://slack.cloudposse.com).

Happy (error-free) infrastructure coding!

---

*The Atmos team is committed to making infrastructure-as-code more accessible, reliable, and enjoyable. These error handling improvements are part of our ongoing effort to reduce friction and increase productivity for infrastructure teams everywhere.*
