# Gotcha Test Runner PRD

## Overview

**Gotcha** is a sophisticated Go test runner with real-time progress tracking, beautiful terminal output, and flexible result formatting. Built with the Charm ecosystem, gotcha transforms the Go testing experience by providing intuitive visual feedback, GitHub Actions integration, and comprehensive test result analysis.

### Core Technologies
- **Fang** - Lightweight Cobra wrapper for beautiful CLIs with signal handling
- **Charmbracelet Log** - Structured logging with color profiles and environment detection
- **Bubble Tea** - Full-featured TUI framework for interactive terminal experiences
- **Lipgloss** - Terminal styling with precise hex color control
- **Viper** - Configuration management with environment variable binding

### Operation Modes
- **Stream Mode** - Real-time test execution with interactive TUI
- **Parse Mode** - Post-processing of existing `go test -json` output

## Motivation

### Current Problems
- **Overwhelming output**: Go test produces megabytes of unstructured logs in large codebases
- **Poor CI visibility**: Difficult to identify failed tests in GitHub Actions and other CI environments
- **No progress tracking**: Long-running test suites provide no real-time feedback
- **Complex coverage analysis**: Coverage reports are hard to parse and visualize
- **Limited GitHub integration**: No native support for test summaries in PR workflows
- **Inconsistent tooling**: Existing solutions lack cohesive design and modern CLI experience

### Business Impact
- Developers waste time scrolling through verbose test logs
- Failed tests are missed in CI, leading to broken deployments  
- Poor test visibility reduces confidence in code changes
- Manual test result analysis slows down code review process
- Lack of visual progress feedback frustrates developers during long test runs

## Goals

1. **Beautiful CLI Experience**: Leverage Charm ecosystem for modern, visually appealing interface
2. **Real-time Progress Tracking**: Provide immediate feedback during test execution with Bubble Tea TUI
3. **CI/CD Integration**: Native GitHub Actions support with PR comments and job summaries
4. **Flexible Configuration**: Support YAML config files and environment variables via Viper
5. **Cross-platform Compatibility**: Work consistently across Linux, macOS, and Windows
6. **Pass-through Arguments**: Support `--` separator for direct `go test` argument passing
7. **Coverage Visualization**: Transform coverage data into visual, actionable insights
8. **Multiple Output Formats**: Support terminal, markdown, and GitHub-specific formats

## Out of Scope

- **Distributed test execution** across multiple machines
- **Custom test parallelization strategies** beyond Go's built-in capabilities
- **Non-Go test frameworks** (Jest, pytest, etc.)
- **Historical trend analysis** and test result dashboards
- **Test result persistence** in databases or external storage
- **Integration with non-Go build systems** (Maven, Gradle, npm)
- **Custom test reporters** beyond the specified formats (terminal, markdown, GitHub)
- **Test generation or mutation testing** capabilities

## Detailed Requirements

### CLI Framework (Fang + Cobra)

#### Command Structure
```bash
gotcha [command] [path] [--flags] [-- go-test-args]
```

#### Core Commands
- **Root command**: `gotcha` (defaults to stream mode)
- **Stream subcommand**: `gotcha stream` (explicit real-time execution)
- **Parse subcommand**: `gotcha parse` (process existing JSON)
- **Version subcommand**: `gotcha version` (version information)

#### Fang Integration
```go
// Use Fang for beautiful CLI with signal handling
return fang.Execute(ctx, rootCmd)
```

#### Pass-through Arguments
- **Double-dash separator**: Everything after `--` passes to `go test`
- **Examples**:
  - `gotcha -- -run TestSpecific`
  - `gotcha stream -- -race -short -count=3`
  - `gotcha -- -run "TestConfig.*" -v`

### Logging System (Charmbracelet Log)

#### Structured Logging Configuration
```go
globalLogger = log.New(os.Stderr)
globalLogger.SetLevel(log.InfoLevel)
globalLogger.SetColorProfile(profile)
```

#### Log Level Configuration
- **CLI Flag**: `--log-level` (persistent flag available to all commands)
- **Environment Variable**: `GOTCHA_LOG_LEVEL`
- **Configuration File**: `log.level` in `.gotcha.yaml`
- **Supported Levels**: `debug`, `info`, `warn`, `error`, `fatal`
- **Default Level**: `info`
- **Precedence**: CLI flag > Environment variable > Config file > Default

#### Log Level Styling with Hex Colors
- **DEBUG**: Background color `#3F51B5` (indigo), black foreground
- **INFO**: Background color `#4CAF50` (green), black foreground  
- **WARN**: Background color `#FF9800` (orange), black foreground
- **ERROR**: Background color `#F44336` (red), black foreground
- **FATAL**: Background color `#F44336` (red), white foreground

#### Context-aware Output
- Structured logging for programmatic parsing
- Human-readable formatting for terminal display
- Color profile detection for different environments
- Dynamic log level adjustment at runtime

### Terminal Styling (Lipgloss with Hex Colors)

#### Color Constants
```go
colorGreen     = "#2ECC40" // Bright green for pass symbols (✔)
colorRed       = "#DC143C" // Crimson red for fail symbols (✘)
colorAmber     = "#FFB347" // Peach orange for skip symbols (⊘)
colorLightGray = "#D3D3D3" // Light gray for test names (primary text)
colorDarkGray  = "#666666" // Dark gray for durations (de-emphasized)
colorBlue      = "#5DADE2" // Blue for spinner animations
colorDarkRed   = "#B22222" // Dark red for error backgrounds
colorWhite     = "#FFFFFF" // White for error text on dark backgrounds
```

#### Visual Hierarchy Requirements

The visual hierarchy MUST follow these strict requirements to ensure optimal readability:

1. **Test Status Symbols** (Highest Visual Priority)
   - ✔ Pass: `colorGreen` (#2ECC40) - Immediately visible success indicator
   - ✘ Fail: `colorRed` (#FF0000) - Immediately visible failure indicator (updated for proper ANSI mapping)
   - ⊘ Skip: `colorAmber` (#FFB347) - Immediately visible skip indicator

2. **Test Names** (Secondary Visual Priority)
   - Color: `colorLightGray` (#D3D3D3)
   - Purpose: Readable and clear, but doesn't compete with status symbols
   - Example: `TestNewSSMStore/valid_options_with_all_fields`

3. **Duration/Metadata** (Tertiary Visual Priority)
   - Color: `colorDarkGray` (#666666)
   - Purpose: Available when needed but de-emphasized
   - Example: `(0.03s)`

4. **Subtest Summary** (Inline with Parent Test)
   - Format: `[X/Y passed]` where X is passed subtests, Y is total subtests
   - Color: Matches parent test status color
   - Purpose: Quick overview of subtest results without overwhelming display
   - Example: `✘ TestWithSubtests (1.23s) [2/5 passed]`

#### Example Output Display
```
✔ TestPasses (0.01s)
✘ TestFails (0.02s)
⊘ TestSkipped (0.00s)
✘ TestWithSubtests (1.23s) [2/5 passed]
```
Where:
- ✔/✘/⊘ are colored per status (green/red/amber)
- Test names are light gray for readability
- Durations in parentheses are dark gray for de-emphasis
- Subtest summaries show inline pass/fail counts

#### Visual Elements
- **Unicode symbols**: ✔ (pass), ✘ (fail), ⊘ (skip)
- **Progress indicators**: Animated spinners and progress bars
- **Mini progress indicators**: Visual subtest progress using colored dots on parent test lines
  - Format: `[●●●●●]` with proportional colored dots based on pass/fail ratio
  - Green dots (●) represent passed subtests, red dots (●) represent failed subtests
  - Display on parent test lines with subtests: `✘ TestName (0.00s) [●●●●●] 40% passed`
  - Show 5 dots total, proportionally distributed based on subtest results
  - Update when parent test completes with final subtest statistics
  - Uses ANSI color codes via Lipgloss styles for terminal compatibility
- **Test result styling**: Color-coded output with consistent formatting
- **Error highlighting**: High-contrast error displays with background colors
- **Subtest visualization**: Inline summary with detailed breakdown on failure

### Enhanced Subtest Visualization

#### Subtest Statistics Tracking
- **Real-time tracking**: Monitor pass/fail/skip counts for each parent test's subtests
- **Inline summary display**: Show `[X/Y passed]` format alongside parent test
- **Detailed breakdown**: Display comprehensive subtest results for failed parent tests

#### Display Formats

##### Successful Parent Test with All Subtests Passing
```
✔ TestWithSubtests (0.45s) [5/5 passed]
```

##### Failed Parent Test with Mixed Results
```
✘ TestWithSubtests (1.23s) [2/5 passed]
  Passed:
    • ValidInput
    • EdgeCase
  Failed:
    • InvalidInput
    • EmptyInput
  Skipped:
    • ConditionalTest
```

##### Parent Test Failed (Not Due to Subtests)
```
✘ TestSetupFailure (0.01s)
  (Test failed during setup/teardown)
```

#### Implementation Requirements
- **Data Structure**: Track subtest results in a map keyed by parent test name
- **Event Processing**: Capture subtest events and associate with parent tests
- **Display Logic**: Show detailed breakdown only for tests with failed/skipped subtests
- **Progress Tracking**: Only count top-level tests for progress bar calculations
- **Both Modes**: Support identical visualization in both TUI and simple (non-TTY) modes

#### Logger Key Styling
- **Log Keys**: Style with dark gray (#666666) and bold formatting
- **Log Values**: Keep unstyled or use appropriate color based on context
- **Separator**: Use consistent separator character (`:` or `=`) between keys and values
- **Example**: `level=INFO msg="Starting test execution" mode=stream`

### Color Support in CI Environments

#### Environment Detection via Viper
```go
viper.BindEnv("NO_COLOR")           // Disable colors completely
viper.BindEnv("FORCE_COLOR")        // Force color output
viper.BindEnv("GITHUB_ACTIONS")     // GitHub Actions environment
viper.BindEnv("CI")                 // General CI detection
viper.BindEnv("TERM")               // Terminal capabilities
viper.BindEnv("COLORTERM")          // Extended terminal capabilities
```

#### Color Profile Detection
- **TrueColor**: Modern terminals with full RGB support
- **ANSI256**: GitHub Actions and most CI environments  
- **ANSI**: Basic CI environments with limited color support
- **NoColor**: Disabled via environment variables or detection

#### Charm Ecosystem Integration
```go
lipgloss.SetColorProfile(profile)
globalLogger.SetColorProfile(profile)
```

### Configuration Management (Viper)

#### Configuration Sources (in precedence order)
1. **CLI Flags** (highest priority)
2. **Environment Variables** (with `GOTCHA_` prefix support)
3. **Configuration File** (`.gotcha.yaml`)
4. **Built-in Defaults** (lowest priority)

#### Environment Variable Bindings
```go
viper.BindEnv("GOTCHA_LOG_LEVEL", "LOG_LEVEL")
viper.BindEnv("GOTCHA_FORCE_NO_TTY", "FORCE_NO_TTY")
viper.BindEnv("GOTCHA_FORCE_TTY", "FORCE_TTY")
viper.BindEnv("GOTCHA_TIMEOUT", "TIMEOUT")
viper.BindEnv("GOTCHA_OUTPUT", "OUTPUT")
```

#### Configuration File Format (.gotcha.yaml)
```yaml
# Logging configuration
log:
  level: info  # Log level: debug, info, warn, error, fatal

# Output format: stream, markdown, github
format: stream

# Space-separated list of packages to test
packages:
  - "./..."

# Additional arguments to pass to go test
testargs: "-timeout 40m"

# Filter displayed tests: all, failed, passed, skipped
show: all

# Output file for test results
output: gotcha-results.json

# Coverage profile file
coverprofile: coverage.out

# Exclude mock files from coverage
exclude-mocks: true

# Package filtering
filter:
  include:
    - ".*"
  exclude: []
```

#### Custom Configuration File
- **Flag**: `--config` to specify custom configuration file path
- **Example**: `gotcha stream --config=/path/to/config.yaml`
- **Discovery**: Searches `.gotcha.yaml` in current and parent directories (up to 3 levels)

### TUI Framework (Bubble Tea)

#### Interactive Components
- **Progress Bar**: Real-time test execution progress
- **Test Counter**: Running tally of passed/failed/skipped tests
- **Live Log View**: Filtered test output with syntax highlighting
- **Statistics Panel**: Summary metrics updated in real-time
- **Spinner Animation**: Visual indicator for running tests

#### Non-TTY Fallback
- Automatic detection of TTY capability
- Graceful degradation to simple streaming output
- CI-friendly output without interactive elements

### Stream Mode Features

#### Real-time Execution
- **Live progress tracking** with test count and elapsed time
- **Package filtering** with regex include/exclude patterns
- **Test result filtering** by status (all, failed, passed, skipped)
- **Interactive TUI** with Bubble Tea components
- **JSON output** to configurable file (default: `gotcha-results.json`)
- **Subtest tracking** with real-time pass/fail/skip statistics

#### Configuration Options
- **Timeout control**: Configurable test timeout (default: 40m)
- **Alert system**: Optional terminal bell on completion
- **Coverage generation**: Integrated coverage profile creation
- **Output customization**: Multiple display modes and formats

### Parse Mode Features

#### Input Processing
- **JSON input**: Process `go test -json` output from stdin or file
- **Coverage integration**: Combine test results with coverage profiles
- **Format conversion**: Transform JSON to various output formats

#### Output Formats
- **Terminal**: Styled output for command-line viewing
- **Markdown**: GitHub-compatible markdown summaries
- **GitHub**: Specialized format for GitHub Actions job summaries

#### Analysis Capabilities
- **Coverage badges**: Visual indicators with color coding
- **Test categorization**: Group results by package and status
- **Error extraction**: Highlight and format test failures
- **Statistics generation**: Comprehensive test run metrics
- **Subtest analysis**: Detailed breakdown of subtest results with pass/fail counts

### GitHub Actions Integration

#### Job Summary Generation
- **Write to `$GITHUB_STEP_SUMMARY`**: Automatic job summary creation
- **Markdown formatting**: GitHub-compatible summary layout
- **Coverage visualization**: Badges and detailed coverage tables
- **Test failure highlighting**: Prominent display of failed tests

#### PR Comment System
- **Automated commenting**: Post test results as PR comments
- **Comment deduplication**: UUID-based tracking to update existing comments
- **Size management**: Smart truncation for large test suites
- **Template-based formatting**: Consistent comment structure

#### GitHub API Integration
- **Authentication**: Token-based API access
- **Comment CRUD operations**: Create, read, update PR comments
- **Error handling**: Graceful fallback when API is unavailable
- **Rate limiting**: Respect GitHub API rate limits

### Coverage Analysis

#### Coverage Profile Processing
- **Mock file exclusion**: Filter out generated mock files from coverage
- **Function-level analysis**: Detailed coverage at function granularity
- **Package-level summaries**: Aggregated coverage by Go package
- **Visual indicators**: Color-coded coverage percentages

#### Coverage Badges
- **Color coding**: 
  - 🟢 Green: ≥80% coverage (excellent)
  - 🟡 Yellow: 50-79% coverage (good)
  - 🔴 Red: <50% coverage (needs improvement)
- **Shields.io integration**: Generate standard coverage badges
- **Multiple formats**: Support for various badge styles and formats

## Alternative Tools Evaluated

### Tools Assessed
- **gotestdox**: Provides readable test names but lacks real-time progress and modern TUI
- **gotest** (rakyll/gotest): Basic colorization but no progress tracking or sophisticated interface
- **gotestsum** (gotestyourself/gotestsum): Good summaries but limited GitHub integration and no Charm ecosystem
- **richgo**: Enhanced output but no streaming mode or comprehensive CI integration
- **go-junit-report**: XML output focus, no real-time capabilities or modern CLI experience

### Why Gotcha Was Built
Existing tools failed to provide:
1. **Modern CLI experience** with beautiful, consistent design (Charm ecosystem)
2. **Real-time progress tracking** with interactive TUI elements
3. **Comprehensive GitHub integration** with native PR comments and job summaries
4. **Flexible configuration** supporting multiple sources (CLI, env vars, config files)
5. **Professional color management** with hex colors and CI environment detection
6. **Cohesive architecture** built on proven, modern Go frameworks

## Test Plan

### Unit Testing
- **Component isolation**: Test each module independently
- **Mock implementations**: Use mocks for external dependencies (GitHub API, filesystem)
- **Coverage target**: Achieve >90% code coverage across all packages
- **Edge case handling**: Test error conditions, malformed input, and boundary cases

### Integration Testing
- **End-to-end workflows**: Test complete user scenarios from CLI to output
- **CI environment simulation**: Test behavior in various CI platforms
- **GitHub API integration**: Test PR comment creation, updating, and error handling
- **Cross-platform validation**: Ensure consistent behavior across operating systems

### Manual Testing Scenarios
- **Real codebase testing**: Run against actual Go projects with various test patterns
- **Performance validation**: Test with large test suites (1000+ tests)
- **Network failure simulation**: Test GitHub integration with network issues
- **Terminal compatibility**: Verify output across different terminal emulators

### Automated Testing in CI
- **GitHub Actions workflow**: Automated testing on push and PR
- **Matrix testing**: Multiple Go versions and operating systems
- **Integration validation**: Test actual GitHub API interactions in controlled environment
- **Regression testing**: Ensure changes don't break existing functionality

## Documentation Updates

### Main Repository Documentation
- **README.md**: Add gotcha section with installation and basic usage
- **Development workflow**: Update testing procedures to use gotcha
- **CI/CD documentation**: Document gotcha integration in GitHub Actions

### Tool-Specific Documentation
- **tools/gotcha/README.md**: Comprehensive usage guide and examples
- **Configuration guide**: Document all configuration options and environment variables
- **GitHub integration**: Step-by-step setup for PR comments and job summaries
- **Troubleshooting**: Common issues and solutions

### Code Documentation
- **Inline comments**: Document complex algorithms and design decisions
- **Package documentation**: Clear godoc for all exported functions and types
- **Example code**: Provide working examples for common use cases
- **API documentation**: Document all configuration options and their effects

## Implementation Notes

### Architecture Principles
- **Modular design**: Clear separation between CLI, TUI, processing, and output components
- **Interface-driven**: Use interfaces for testability and future extensibility
- **Error handling**: Comprehensive error handling with proper exit codes
- **Performance**: Efficient streaming with minimal memory overhead

### Key Dependencies
- **Fang**: Provides beautiful CLI wrapper around Cobra with signal handling
- **Bubble Tea**: Enables rich terminal UI with proper event handling
- **Lipgloss**: Consistent styling with hex color support
- **Charmbracelet Log**: Structured logging with color profile management
- **Viper**: Configuration management with environment variable binding

### Design Patterns
- **Observer pattern**: For real-time test result updates
- **Strategy pattern**: For different output formats
- **Factory pattern**: For creating appropriate renderers based on environment
- **Command pattern**: For structured CLI command handling

### Security Considerations
- **GitHub token handling**: Secure storage and transmission of API tokens
- **Input validation**: Sanitize all user inputs and file paths
- **Output escaping**: Prevent injection attacks in generated content
- **Permission handling**: Respect file system permissions and access controls

## Acceptance Criteria

### Core Functionality
- ✅ **Stream mode**: Tests run with real-time progress display using Bubble Tea TUI
- ✅ **Parse mode**: Existing JSON results processed correctly into multiple formats
- ✅ **Pass-through args**: Arguments after `--` separator correctly passed to `go test`
- ✅ **Exit codes**: Test failure exit codes properly propagated to calling process

### CLI Experience
- ✅ **Fang integration**: Beautiful CLI interface with proper signal handling
- ✅ **Color support**: Hex colors work correctly in all supported environments  
- ✅ **Configuration**: Viper correctly handles YAML files, environment variables, and CLI flags
- ✅ **Cross-platform**: Consistent behavior on Linux, macOS, and Windows

### GitHub Integration
- ✅ **Job summaries**: GitHub Actions job summaries generated and displayed correctly
- ✅ **PR comments**: Test results posted as PR comments with proper formatting
- ✅ **Coverage badges**: Coverage percentages displayed with appropriate color coding
- ✅ **Error handling**: Graceful degradation when GitHub API is unavailable

### Output Quality
- ✅ **Visual consistency**: All output uses consistent Charm ecosystem styling
- ✅ **Information hierarchy**: Important information (failures) prominently displayed
- ✅ **Visual hierarchy colors**: Test symbols use status colors (green/red/amber), test names use light gray, durations use dark gray
- ✅ **Performance**: Tool completes processing within reasonable time limits
- ✅ **Accessibility**: Output readable in various terminal configurations
- ✅ **Subtest visualization**: Tests with subtests display inline summary `[X/Y passed]` and detailed breakdown on failure

### Configuration Management
- ✅ **YAML config**: `.gotcha.yaml` files loaded and applied correctly
- ✅ **Environment variables**: All configuration options available via env vars
- ✅ **Precedence**: Configuration sources respect documented precedence order
- ✅ **Validation**: Invalid configuration values handled with clear error messages

## References

- [Charm.sh](https://charm.sh/) - The Charm ecosystem for beautiful CLI tools
- [Fang](https://github.com/charmbracelet/fang) - Lightweight Cobra wrapper for beautiful CLIs
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Powerful TUI framework for Go
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Style definitions for nice terminal layouts
- [Charmbracelet Log](https://github.com/charmbracelet/log) - Structured, leveled logging for Go
- [Viper](https://github.com/spf13/viper) - Go configuration with fangs
- [Cobra](https://github.com/spf13/cobra) - Commander for modern Go CLI applications
- [GitHub REST API](https://docs.github.com/en/rest) - GitHub API documentation
- [Go test](https://golang.org/pkg/testing/) - Go testing package documentation
- [Go test JSON output](https://golang.org/cmd/test2json/) - JSON test output format specification