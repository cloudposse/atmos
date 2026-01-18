# Atmos VS Code Extension

## Overview

Comprehensive VS Code extension that integrates Atmos infrastructure-as-code capabilities directly into the development environment. Supports both CLI-first integration and optional server mode for enhanced performance and real-time features.

## Design Principles

1. **Flexible Integration**: Support both CLI-only and server mode approaches
2. **Real-time Validation**: Show configuration errors and warnings as users type
3. **Seamless Navigation**: Enable easy navigation between related configurations
4. **Visual Understanding**: Provide visual tools for understanding complex relationships
5. **Integrated Workflows**: Execute Atmos operations directly from the editor
6. **Zero Configuration**: Auto-detect Atmos projects and work out-of-the-box
7. **Progressive Enhancement**: Basic features with CLI, advanced features with server mode

## Integration Approaches

### CLI-First Approach (MVP)
- Direct execution of Atmos CLI commands via `child_process`
- JSON output parsing for structured data
- File system watching for configuration changes
- Process management for long-running operations
- Caching layer for performance optimization

### Server Mode Approach (Enhanced)
- WebSocket connection for real-time updates
- REST API for efficient data retrieval
- Language Server Protocol for advanced language features
- Streaming operation logs and progress
- Advanced caching and performance

### Hybrid Approach (Recommended)
- Automatic detection of server availability
- Graceful fallback from server mode to CLI mode
- Per-feature switching based on performance needs
- User preference for integration method

## Feature Set

### Core Features

#### Project Discovery and Setup
- **Auto-detection**: Automatically detect Atmos projects by finding `atmos.yaml`
- **Workspace configuration**: Support multi-root workspaces with multiple Atmos projects
- **Extension activation**: Activate extension only in workspaces with Atmos projects
- **Server detection**: Automatically detect and connect to Atmos server if available
- **CLI validation**: Verify Atmos CLI is available and working
- **Version compatibility**: Check and warn about version mismatches
- **Configuration profiles**: Support multiple Atmos configuration profiles

#### Language Support

##### YAML IntelliSense
- **Stack/Component autocomplete**: Names via CLI queries or server API
- **Variable autocomplete**: Variable names and values from configuration hierarchy
- **Template function support**: Go template and Sprig/Gomplate function autocomplete
- **Schema validation**: Real-time validation using `atmos validate` or server validation
- **Error diagnostics**: Show configuration errors with contextual help and quick fixes
- **Hover information**: Show resolved variable values and component details
- **Code folding**: Intelligent folding for configuration sections
- **Syntax highlighting**: Enhanced highlighting for Atmos-specific constructs

##### Navigation Features
- **Go to Definition**: Navigate to component definitions, imports, templates
- **Find References**: Show all places where a component or variable is used
- **Find Implementations**: Find all stack implementations of a component
- **Symbol outline**: Tree view of configuration structure in current file
- **Breadcrumb navigation**: Show current location in configuration hierarchy
- **Quick open**: Fast file switching with fuzzy search
- **Recent files**: Track and quick-access recently edited configurations
- **Bookmark support**: Save and organize important configuration locations

#### Explorer Integration

##### Atmos Explorer Panel
- **Stack tree view**: Hierarchical view using `atmos list stacks` or server API
- **Component browser**: List all components with usage statistics and health status
- **Environment organizer**: Group stacks by environment, region, or custom criteria
- **Search and filter**: Find stacks/components by name, tags, or properties
- **Recent items**: Quick access to recently viewed/edited configurations
- **Favorites**: Pin frequently accessed stacks/components
- **Status indicators**: Show validation status and deployment health
- **Context menus**: Right-click actions for common operations

##### File Navigation
- **Smart file opening**: Open correct configuration file when selecting stacks/components
- **Related files**: Show related configuration files in sidebar
- **File templates**: Generate new stack/component configurations from templates
- **Import/export**: Import configurations from other projects or export for sharing
- **File diff**: Compare configurations between environments or versions
- **File history**: Track changes and revert to previous versions

#### Configuration Management

##### Real-time Validation
- **Syntax validation**: YAML syntax errors with precise error locations
- **Schema validation**: Atmos-specific validation rules and JSON schema compliance
- **Reference validation**: Verify component references, imports, and dependencies exist
- **Variable validation**: Check variable definitions, usage, and type consistency
- **Template validation**: Validate Go template syntax and function calls
- **Policy validation**: Check against OPA policies and organizational standards
- **Cross-stack validation**: Validate dependencies and references across stacks
- **Incremental validation**: Only validate changed files and dependencies

##### Rich Diagnostics
- **Error context**: Show exact error location with helpful descriptions
- **Quick fixes**: Suggest and apply automatic fixes where possible
- **Code actions**: Contextual actions for common configuration tasks
- **Validation on save**: Run full validation when files are saved
- **Batch validation**: Validate entire project with summary report
- **Problem panel**: Centralized view of all validation issues
- **Severity levels**: Categorize issues as errors, warnings, or information
- **Custom rules**: Define project-specific validation rules

#### Visual Tools

##### Dependency Visualization
- **Component dependencies**: Visual graph showing component relationships
- **Stack inheritance**: Show how stacks inherit from each other
- **Variable flow**: Trace variable values through inheritance chain
- **Template dependencies**: Show template file dependencies and inclusions
- **Interactive exploration**: Click nodes to navigate to definitions
- **Dependency analysis**: Identify circular dependencies and optimization opportunities
- **Impact analysis**: Show what changes when modifying configurations
- **Export capabilities**: Export dependency graphs as images or data files

##### Configuration Analysis
- **Environment comparison**: Compare stack configurations across environments
- **Variable comparison**: Show variable differences between environments and inheritance levels
- **Template preview**: Live preview of template rendering with variable substitution
- **Change impact**: Visualize what changes when modifying configurations
- **Configuration diff**: Side-by-side comparison of configurations
- **Merge conflict resolution**: Advanced tools for resolving configuration conflicts
- **Configuration lineage**: Track configuration changes over time
- **Resource estimation**: Estimate cloud resource costs and usage

##### Dashboard and Monitoring
- **Project health**: Overall project validation status and metrics
- **Deployment status**: Real-time status of infrastructure deployments
- **Resource usage**: Monitor cloud resource usage and costs
- **Activity timeline**: Track recent changes and operations
- **Performance metrics**: Monitor configuration parsing and validation performance
- **Usage analytics**: Track most-used components and configurations

#### Integrated Operations

##### Command Integration
- **Command palette**: All Atmos commands available via Ctrl/Cmd+Shift+P
- **Context menus**: Right-click operations on files and tree items
- **Status bar**: Show current stack/component context and server connection status
- **Notification integration**: Show operation status and results
- **Keyboard shortcuts**: Configurable shortcuts for common operations
- **Command history**: Track and repeat recent commands
- **Bulk operations**: Execute operations across multiple stacks/components
- **Operation templates**: Save and reuse common operation sequences

##### Terminal Integration
- **Integrated terminal**: Run Atmos commands in VS Code terminal with context
- **Output channels**: Dedicated channels for different operation types (terraform, helmfile, etc.)
- **Progress indicators**: Show progress for long-running CLI operations or server requests
- **Interactive prompts**: Handle Atmos prompts within VS Code interface
- **Log streaming**: Real-time streaming of operation logs
- **Terminal automation**: Auto-populate terminal with contextual commands
- **Session management**: Manage multiple terminal sessions for different environments
- **Command suggestions**: Intelligent command completion in terminal

### Advanced Features

#### Workflow Automation
- **One-click deployment**: Deploy components/stacks with single button click
- **Pre-deployment checks**: Automatic validation and approval workflows before deployment
- **Deployment pipelines**: Visual pipeline editor for complex deployment workflows
- **Rollback support**: Quick rollback to previous configurations or deployments
- **Scheduled operations**: Schedule terraform plans, validations, and other operations
- **GitOps integration**: Integrate with GitOps workflows and pull request automation
- **Approval workflows**: Multi-stage approval processes for production changes
- **Deployment gates**: Conditional deployment based on validation and testing results

#### AI-Powered Assistance
- **Configuration suggestions**: AI-powered suggestions for configuration improvements
- **Error explanation**: Intelligent explanation of complex validation errors
- **Code completion**: Context-aware completion suggestions based on patterns
- **Refactoring assistance**: Automated refactoring suggestions for better organization
- **Security recommendations**: AI-powered security analysis and recommendations
- **Performance optimization**: Suggestions for improving configuration performance
- **Best practice enforcement**: Automated checks against industry best practices
- **Documentation generation**: Auto-generate documentation from configurations

#### Template and Snippet Management
- **Custom snippets**: Pre-configured snippets for common Atmos patterns
- **Snippet sharing**: Share snippets across team members and projects
- **Template expansion**: Live preview of template expansion with variable substitution
- **Variable substitution**: Show resolved variable values inline while editing
- **Template debugging**: Step-through debugging of template rendering
- **Template validation**: Validate template syntax and function usage
- **Template library**: Curated library of community templates and patterns
- **Template versioning**: Version control for template changes and evolution

#### Team Collaboration
- **Shared workspaces**: Collaborate on configurations across team members
- **Real-time editing**: See live edits from other team members (server mode)
- **Comments and annotations**: Add contextual comments to configurations
- **Review integration**: Integration with code review processes and tools
- **Change notifications**: Notify team members of relevant configuration changes
- **Permission management**: Fine-grained permissions for different team roles
- **Workspace templates**: Create standardized workspace setups for teams
- **Knowledge sharing**: Share configuration patterns and best practices

#### Documentation Integration
- **Contextual help**: Show relevant Atmos documentation for current context
- **Component documentation**: Auto-generate and maintain component documentation
- **Usage examples**: Show examples of how components are used across stacks
- **Best practices**: Highlight configuration best practices and anti-patterns
- **Interactive tutorials**: Guided tutorials for learning Atmos concepts
- **Documentation search**: Search Atmos documentation directly from VS Code
- **Custom documentation**: Link to organization-specific documentation and runbooks
- **Documentation generation**: Generate documentation from code and configurations

#### Testing and Validation
- **Configuration testing**: Test framework for validating configuration behavior
- **Mock environments**: Create mock environments for testing configurations
- **Test automation**: Automated testing of configuration changes
- **Performance testing**: Test configuration parsing and validation performance
- **Security testing**: Automated security analysis of configurations
- **Integration testing**: Test integration with external systems and services
- **Test reporting**: Comprehensive test reports and coverage analysis
- **Continuous validation**: Automated validation in CI/CD pipelines

#### Extensibility and Customization
- **Custom commands**: Create and share custom Atmos commands
- **Plugin system**: Extensible architecture for community plugins
- **Theme customization**: Customize UI themes and appearance
- **Workspace customization**: Tailor extension behavior to project needs
- **Integration hooks**: Webhooks and event handlers for custom integrations
- **Custom validators**: Create project-specific validation rules
- **Automation scripts**: Custom scripts for common workflow automation
- **API integration**: Integrate with external tools and services

## Technical Architecture

### Extension Structure
- **TypeScript-based**: Modern TypeScript development with strict typing
- **Modular design**: Separate modules for different feature areas
- **Adaptive integration**: Automatic switching between CLI and server mode
- **Event-driven**: Reactive architecture for real-time updates

### Core Components

1. **Integration Manager**: Handles CLI vs server mode detection and switching
2. **CLI Manager**: Direct CLI command execution and output parsing
3. **Server Client**: WebSocket and REST client for server mode
4. **Configuration Parser**: YAML parsing and validation with caching
5. **Tree Provider**: Custom tree view providers for Atmos explorer
6. **Command Manager**: Unified command execution and integration
7. **Webview Provider**: Custom webviews for visual tools and dashboards
8. **Language Client**: Optional LSP client for server mode language features

### Integration Patterns

#### CLI Integration (Fallback Mode)
```typescript
// Command execution with caching
const cliManager = new AtmosCLIManager();
const stacks = await cliManager.listStacks({ format: 'json', cache: true });
const component = await cliManager.describeComponent(name, stack, { cache: true });

// File watching for cache invalidation
const watcher = new ConfigWatcher();
watcher.onConfigChange(() => cliManager.invalidateCache());
```

#### Server Integration (Enhanced Mode)
```typescript
// WebSocket connection for real-time updates
const serverClient = new AtmosServerClient();
await serverClient.connect();

// REST API for data retrieval
const stacks = await serverClient.getStacks();
const component = await serverClient.getComponent(name, stack);

// Real-time event handling
serverClient.onConfigurationChange((event) => {
    updateUI(event.changedFiles);
});
```

### Performance Optimization

#### CLI Mode Optimizations
- **Command caching**: Cache CLI outputs with TTL and file-based invalidation
- **Parallel execution**: Execute multiple CLI commands concurrently when possible
- **Background processing**: Run expensive operations in background threads
- **Debouncing**: Debounce file change events to reduce CLI call frequency
- **Smart invalidation**: Invalidate only affected cache entries on file changes

#### Server Mode Optimizations
- **Connection pooling**: Reuse WebSocket connections for multiple operations
- **Request batching**: Batch multiple API requests for efficiency
- **Stream processing**: Process large datasets using streaming APIs
- **Push updates**: Receive real-time updates instead of polling
- **Advanced caching**: Leverage server-side caching and incremental updates

### Dependencies

#### VS Code Extension APIs
- **Language Server Protocol**: For enhanced language support
- **Tree View API**: For custom explorer panels
- **Webview API**: For visual tools and dashboards
- **Command API**: For command palette integration
- **Diagnostic API**: For error/warning display
- **File System Watcher**: For monitoring configuration changes
- **Authentication API**: For server mode authentication

#### External Libraries
- **yaml**: YAML parsing and manipulation
- **ws**: WebSocket client for server mode
- **axios**: HTTP client for REST API calls
- **vis.js/d3.js**: Visualization library for dependency graphs
- **monaco-editor**: Enhanced editor features and language support
- **chokidar**: Enhanced file watching capabilities

### Performance Requirements
- **Startup time**: < 2 seconds extension activation
- **CLI operations**: < 1 second for typical commands
- **Server operations**: < 100ms for cached API calls
- **File parsing**: < 100ms for typical configuration files
- **UI responsiveness**: < 50ms for user interactions
- **Memory usage**: < 100MB for typical projects

## Implementation Plan

### Phase 1: CLI Foundation
**Priority**: MVP with CLI integration

- Project detection and workspace setup
- CLI integration framework with command execution and output parsing
- Basic YAML language support with syntax highlighting
- Simple explorer tree view using `atmos list` commands
- Basic command integration for describe/list operations
- File watching and cache invalidation

### Phase 2: Enhanced Language Support
**Priority**: Rich editing experience

- Advanced autocomplete using CLI queries for stack/component names
- Real-time validation using `atmos validate` commands
- Go-to-definition and find references via file analysis
- Error correction suggestions based on CLI validation output
- Template function support and validation

### Phase 3: Visual Tools & Analytics
**Priority**: Visual understanding and navigation

- Dependency graph visualization using CLI describe commands
- Configuration diff viewer using CLI data across environments
- Interactive stack inheritance visualization
- Dashboard with project health and deployment status
- Advanced navigation and search capabilities

### Phase 4: Server Mode Integration
**Priority**: Enhanced performance and real-time features

- Server detection and connection management
- WebSocket integration for real-time updates
- LSP client integration for advanced language features
- Performance optimization with server-side caching
- Advanced operation management and streaming

### Phase 5: Advanced Features
**Priority**: Workflow automation and team collaboration

- Workflow automation and deployment pipelines
- AI-powered assistance and suggestions
- Advanced template and snippet management
- Team collaboration and shared workspaces
- Comprehensive testing and validation framework

### Phase 6: Enterprise & Extensibility
**Priority**: Enterprise features and ecosystem

- Enterprise authentication and authorization
- Advanced security and compliance features
- Plugin system and extensibility framework
- Advanced monitoring and analytics
- Third-party integrations and marketplace

## CLI Integration Requirements

### Required Atmos CLI Commands
```bash
# Core operations
atmos list stacks --format json
atmos list components --format json
atmos describe config --format json
atmos describe component <name> --stack <stack> --format json
atmos describe stack <stack> --format json
atmos describe dependents <component> --format json

# Validation
atmos validate stacks --format json
atmos validate component <name> --stack <stack> --format json
atmos validate schema --format json

# Operations
atmos terraform plan <component> --stack <stack>
atmos terraform apply <component> --stack <stack>
atmos terraform destroy <component> --stack <stack>
atmos helmfile sync <component> --stack <stack>
atmos workflow <name> --stack <stack>

# Advanced features
atmos vendor pull --format json
atmos docs generate --format json
atmos atlantis generate repo-config --format json
```

### Server Mode API Integration
- REST API endpoints for all CLI operations
- WebSocket events for real-time updates
- LSP endpoints for language server features
- Authentication and session management
- Streaming APIs for large datasets and operations
