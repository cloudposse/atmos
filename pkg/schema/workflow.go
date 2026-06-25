package schema

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ErrInvalidWorkflowContainer is returned when a workflow `container` value cannot be decoded.
var ErrInvalidWorkflowContainer = errors.New("invalid workflow container configuration")

// DescribeWorkflowsItem represents a workflow item in the describe workflows output.
type DescribeWorkflowsItem struct {
	File     string `yaml:"file" json:"file" mapstructure:"file"`
	Workflow string `yaml:"workflow" json:"workflow" mapstructure:"workflow"`
}

// ViewportConfig configures viewport display settings.
type ViewportConfig struct {
	Height int `yaml:"height,omitempty" json:"height,omitempty" mapstructure:"height"` // Lines.
	Width  int `yaml:"width,omitempty" json:"width,omitempty" mapstructure:"width"`    // Columns.
}

// ShowConfig configures automatic display features for workflows.
// All fields use *bool to enable tri-state logic: nil (inherit), true, false.
// This allows step-level settings to override workflow-level defaults via deep merge.
type ShowConfig struct {
	// Header auto-displays workflow description as styled header before first step.
	Header *bool `yaml:"header,omitempty" json:"header,omitempty" mapstructure:"header"`
	// Flags displays workflow-level flag values under header (e.g., "stack: dev").
	Flags *bool `yaml:"flags,omitempty" json:"flags,omitempty" mapstructure:"flags"`
	// Command shows step command before execution (with $ prefix).
	Command *bool `yaml:"command,omitempty" json:"command,omitempty" mapstructure:"command"`
	// Count shows step count prefix (e.g., "[1/3]").
	Count *bool `yaml:"count,omitempty" json:"count,omitempty" mapstructure:"count"`
	// Progress shows progress bar pinned to bottom (Docker-build style, TTY only).
	Progress *bool `yaml:"progress,omitempty" json:"progress,omitempty" mapstructure:"progress"`
}

// ContainerMount represents a volume mount for container steps.
type ContainerMount struct {
	Type     string `yaml:"type,omitempty" json:"type,omitempty" mapstructure:"type"` // bind, volume, tmpfs.
	Source   string `yaml:"source,omitempty" json:"source,omitempty" mapstructure:"source"`
	Target   string `yaml:"target,omitempty" json:"target,omitempty" mapstructure:"target"`
	ReadOnly bool   `yaml:"read_only,omitempty" json:"read_only,omitempty" mapstructure:"read_only"`
}

// ContainerPort represents a port mapping for container steps.
type ContainerPort struct {
	Host      int    `yaml:"host,omitempty" json:"host,omitempty" mapstructure:"host"`
	Container int    `yaml:"container,omitempty" json:"container,omitempty" mapstructure:"container"`
	Protocol  string `yaml:"protocol,omitempty" json:"protocol,omitempty" mapstructure:"protocol"`
}

// ContainerBuildBakeStep configures a Docker Buildx Bake build action.
type ContainerBuildBakeStep struct {
	File    string            `yaml:"file,omitempty" json:"file,omitempty" mapstructure:"file"`
	Files   []string          `yaml:"files,omitempty" json:"files,omitempty" mapstructure:"files"`
	Target  string            `yaml:"target,omitempty" json:"target,omitempty" mapstructure:"target"`
	Targets []string          `yaml:"targets,omitempty" json:"targets,omitempty" mapstructure:"targets"`
	Set     []string          `yaml:"set,omitempty" json:"set,omitempty" mapstructure:"set"`
	Vars    map[string]string `yaml:"vars,omitempty" json:"vars,omitempty" mapstructure:"vars"`
	Load    bool              `yaml:"load,omitempty" json:"load,omitempty" mapstructure:"load"`
	Push    bool              `yaml:"push,omitempty" json:"push,omitempty" mapstructure:"push"`
	Print   bool              `yaml:"print,omitempty" json:"print,omitempty" mapstructure:"print"`
}

// ContainerBuildStep configures a container image build action.
type ContainerBuildStep struct {
	Provider         string                  `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	RuntimeAutoStart bool                    `yaml:"runtime_auto_start,omitempty" json:"runtime_auto_start,omitempty" mapstructure:"runtime_auto_start"`
	Engine           string                  `yaml:"engine,omitempty" json:"engine,omitempty" mapstructure:"engine"`
	Context          string                  `yaml:"context,omitempty" json:"context,omitempty" mapstructure:"context"`
	Dockerfile       string                  `yaml:"dockerfile,omitempty" json:"dockerfile,omitempty" mapstructure:"dockerfile"`
	Tags             []string                `yaml:"tags,omitempty" json:"tags,omitempty" mapstructure:"tags"`
	BuildArgs        map[string]string       `yaml:"build_args,omitempty" json:"build_args,omitempty" mapstructure:"build_args"`
	Target           string                  `yaml:"target,omitempty" json:"target,omitempty" mapstructure:"target"`
	NoCache          bool                    `yaml:"no_cache,omitempty" json:"no_cache,omitempty" mapstructure:"no_cache"`
	Pull             bool                    `yaml:"pull,omitempty" json:"pull,omitempty" mapstructure:"pull"`
	Bake             *ContainerBuildBakeStep `yaml:"bake,omitempty" json:"bake,omitempty" mapstructure:"bake"`
}

// ContainerPushStep configures a container image push action.
type ContainerPushStep struct {
	Provider         string   `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	RuntimeAutoStart bool     `yaml:"runtime_auto_start,omitempty" json:"runtime_auto_start,omitempty" mapstructure:"runtime_auto_start"`
	Image            string   `yaml:"image,omitempty" json:"image,omitempty" mapstructure:"image"`
	Tags             []string `yaml:"tags,omitempty" json:"tags,omitempty" mapstructure:"tags"`
}

// ContainerInspectStep configures a container image inspect action that renders
// curated image metadata.
type ContainerInspectStep struct {
	Provider         string `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	RuntimeAutoStart bool   `yaml:"runtime_auto_start,omitempty" json:"runtime_auto_start,omitempty" mapstructure:"runtime_auto_start"`
	Image            string `yaml:"image,omitempty" json:"image,omitempty" mapstructure:"image"`
}

// ContainerRestart configures the runtime restart policy for a long-lived
// container component. It maps to the docker/podman `--restart` flag
// (`<policy>[:<max_retries>]`). MaxRetries is only meaningful for `on-failure`.
type ContainerRestart struct {
	Policy     string `yaml:"policy,omitempty" json:"policy,omitempty" mapstructure:"policy"`                // no, always, on-failure, unless-stopped.
	MaxRetries int    `yaml:"max_retries,omitempty" json:"max_retries,omitempty" mapstructure:"max_retries"` // on-failure only.
}

// ContainerHealthCheck configures a container health check, mirroring the Docker
// Compose `healthcheck` shape. It maps to the docker/podman `--health-*` flags
// (or `--no-healthcheck` when disabled). Test may be a bare string or a list whose
// first element is `NONE`, `CMD`, or `CMD-SHELL`; a bare string / unprefixed list
// is treated as `CMD-SHELL`. Duration fields accept Go duration strings (e.g. `30s`).
type ContainerHealthCheck struct {
	Test          []string `yaml:"test,omitempty" json:"test,omitempty" mapstructure:"test"`                               // string or [NONE|CMD|CMD-SHELL, ...].
	Interval      string   `yaml:"interval,omitempty" json:"interval,omitempty" mapstructure:"interval"`                   // e.g. 30s.
	Timeout       string   `yaml:"timeout,omitempty" json:"timeout,omitempty" mapstructure:"timeout"`                      // e.g. 10s.
	Retries       int      `yaml:"retries,omitempty" json:"retries,omitempty" mapstructure:"retries"`                      // consecutive failures before unhealthy.
	StartPeriod   string   `yaml:"start_period,omitempty" json:"start_period,omitempty" mapstructure:"start_period"`       // grace period before failures count.
	StartInterval string   `yaml:"start_interval,omitempty" json:"start_interval,omitempty" mapstructure:"start_interval"` // probe interval during the start period.
	Disable       bool     `yaml:"disable,omitempty" json:"disable,omitempty" mapstructure:"disable"`                      // disable any image healthcheck.
}

// ContainerRunStep configures a one-shot container run action.
type ContainerRunStep struct {
	Image             string                `yaml:"image,omitempty" json:"image,omitempty" mapstructure:"image"`
	Command           string                `yaml:"command,omitempty" json:"command,omitempty" mapstructure:"command"`
	Shell             string                `yaml:"shell,omitempty" json:"shell,omitempty" mapstructure:"shell"`
	Provider          string                `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	RuntimeAutoStart  bool                  `yaml:"runtime_auto_start,omitempty" json:"runtime_auto_start,omitempty" mapstructure:"runtime_auto_start"`
	Pull              string                `yaml:"pull,omitempty" json:"pull,omitempty" mapstructure:"pull"`
	Workspace         string                `yaml:"workspace,omitempty" json:"workspace,omitempty" mapstructure:"workspace"`
	WorkspaceReadOnly bool                  `yaml:"workspace_read_only,omitempty" json:"workspace_read_only,omitempty" mapstructure:"workspace_read_only"`
	Cleanup           string                `yaml:"cleanup,omitempty" json:"cleanup,omitempty" mapstructure:"cleanup"`
	User              string                `yaml:"user,omitempty" json:"user,omitempty" mapstructure:"user"`
	RunArgs           []string              `yaml:"run_args,omitempty" json:"run_args,omitempty" mapstructure:"run_args"`
	Mounts            []ContainerMount      `yaml:"mounts,omitempty" json:"mounts,omitempty" mapstructure:"mounts"`
	Ports             []ContainerPort       `yaml:"ports,omitempty" json:"ports,omitempty" mapstructure:"ports"`
	Restart           *ContainerRestart     `yaml:"restart,omitempty" json:"restart,omitempty" mapstructure:"restart"`
	HealthCheck       *ContainerHealthCheck `yaml:"healthcheck,omitempty" json:"healthcheck,omitempty" mapstructure:"healthcheck"`
}

// WorkflowContainer configures a workflow-level container-backed sandbox or a
// step-level container override. A YAML value of `false` disables inheritance.
type WorkflowContainer struct {
	Enabled           *bool             `yaml:"-" json:"-" mapstructure:"-"`
	Image             string            `yaml:"image,omitempty" json:"image,omitempty" mapstructure:"image"`
	Shell             string            `yaml:"shell,omitempty" json:"shell,omitempty" mapstructure:"shell"`
	Provider          string            `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	RuntimeAutoStart  bool              `yaml:"runtime_auto_start,omitempty" json:"runtime_auto_start,omitempty" mapstructure:"runtime_auto_start"`
	Pull              string            `yaml:"pull,omitempty" json:"pull,omitempty" mapstructure:"pull"`
	Workspace         string            `yaml:"workspace,omitempty" json:"workspace,omitempty" mapstructure:"workspace"`
	WorkspaceReadOnly bool              `yaml:"workspace_read_only,omitempty" json:"workspace_read_only,omitempty" mapstructure:"workspace_read_only"`
	Cleanup           string            `yaml:"cleanup,omitempty" json:"cleanup,omitempty" mapstructure:"cleanup"`
	User              string            `yaml:"user,omitempty" json:"user,omitempty" mapstructure:"user"`
	RunArgs           []string          `yaml:"run_args,omitempty" json:"run_args,omitempty" mapstructure:"run_args"`
	Mounts            []ContainerMount  `yaml:"mounts,omitempty" json:"mounts,omitempty" mapstructure:"mounts"`
	Ports             []ContainerPort   `yaml:"ports,omitempty" json:"ports,omitempty" mapstructure:"ports"`
	Env               map[string]string `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`
}

// UnmarshalYAML supports both object syntax and `container: false`.
func (c *WorkflowContainer) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var enabled bool
		if err := value.Decode(&enabled); err != nil {
			return fmt.Errorf("%w: container must be a mapping or boolean: %w", ErrInvalidWorkflowContainer, err)
		}
		c.Enabled = &enabled
		return nil
	case yaml.MappingNode:
		type workflowContainer WorkflowContainer
		var decoded workflowContainer
		if err := value.Decode(&decoded); err != nil {
			return fmt.Errorf("%w: container must be a mapping or boolean: %w", ErrInvalidWorkflowContainer, err)
		}
		*c = WorkflowContainer(decoded)
		return nil
	default:
		return fmt.Errorf("%w: container must be a mapping or boolean, got YAML node kind %d", ErrInvalidWorkflowContainer, value.Kind)
	}
}

// IsEnabled reports whether the container config should be applied.
func (c *WorkflowContainer) IsEnabled() bool {
	return c != nil && (c.Enabled == nil || *c.Enabled)
}

// WorkflowStep represents a single step in a workflow.
type WorkflowStep struct {
	// Existing fields.
	Name             string       `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	Command          string       `yaml:"command" json:"command" mapstructure:"command"`
	Stack            string       `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`
	Type             string       `yaml:"type,omitempty" json:"type,omitempty" mapstructure:"type"`
	WorkingDirectory string       `yaml:"working_directory,omitempty" json:"working_directory,omitempty" mapstructure:"working_directory"`
	Retry            *RetryConfig `yaml:"retry,omitempty" json:"retry,omitempty" mapstructure:"retry"`
	Identity         string       `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"`
	When             Condition    `yaml:"when,omitempty" json:"when,omitempty" mapstructure:"when"`
	// Interactive attaches host stdin to the step and lets the step handle Ctrl-C (like docker -i).
	Interactive bool `yaml:"interactive,omitempty" json:"interactive,omitempty" mapstructure:"interactive"`
	// Tty allocates a pseudo-terminal for the step (like docker -t). Combine with interactive for full terminal sessions.
	Tty bool `yaml:"tty,omitempty" json:"tty,omitempty" mapstructure:"tty"`

	// Interactive step fields.
	Prompt      string   `yaml:"prompt,omitempty" json:"prompt,omitempty" mapstructure:"prompt"`                // Prompt text for interactive types.
	Options     []string `yaml:"options,omitempty" json:"options,omitempty" mapstructure:"options"`             // Options for choose/filter.
	Default     string   `yaml:"default,omitempty" json:"default,omitempty" mapstructure:"default"`             // Default value.
	Placeholder string   `yaml:"placeholder,omitempty" json:"placeholder,omitempty" mapstructure:"placeholder"` // Input placeholder.
	Password    bool     `yaml:"password,omitempty" json:"password,omitempty" mapstructure:"password"`          // Mask input.
	Multiple    bool     `yaml:"multiple,omitempty" json:"multiple,omitempty" mapstructure:"multiple"`          // Allow multiple selection.
	Limit       int      `yaml:"limit,omitempty" json:"limit,omitempty" mapstructure:"limit"`                   // Selection limit.

	// Output/UI step fields.
	Content   string           `yaml:"content,omitempty" json:"content,omitempty" mapstructure:"content"`       // Content for output types (supports templates).
	Title     string           `yaml:"title,omitempty" json:"title,omitempty" mapstructure:"title"`             // Title for spin/pager.
	Data      []map[string]any `yaml:"data,omitempty" json:"data,omitempty" mapstructure:"data"`                // Data for table type.
	Columns   []string         `yaml:"columns,omitempty" json:"columns,omitempty" mapstructure:"columns"`       // Columns for table.
	Separator string           `yaml:"separator,omitempty" json:"separator,omitempty" mapstructure:"separator"` // Separator for join type (default: newline).

	// File picker fields.
	Path       string   `yaml:"path,omitempty" json:"path,omitempty" mapstructure:"path"`                   // Starting path for file picker.
	Extensions []string `yaml:"extensions,omitempty" json:"extensions,omitempty" mapstructure:"extensions"` // File extensions filter.

	// Display configuration.
	Output   string          `yaml:"output,omitempty" json:"output,omitempty" mapstructure:"output"`       // Output mode: viewport, raw, log, none.
	Height   int             `yaml:"height,omitempty" json:"height,omitempty" mapstructure:"height"`       // Height for write type (editor lines).
	Viewport *ViewportConfig `yaml:"viewport,omitempty" json:"viewport,omitempty" mapstructure:"viewport"` // Viewport settings for output mode.
	Timeout  string          `yaml:"timeout,omitempty" json:"timeout,omitempty" mapstructure:"timeout"`    // Timeout duration.
	Count    int             `yaml:"count,omitempty" json:"count,omitempty" mapstructure:"count"`          // Count for linebreak type.

	// Style step fields (like gum style).
	Foreground       string `yaml:"foreground,omitempty" json:"foreground,omitempty" mapstructure:"foreground"`                      // Foreground color.
	Background       string `yaml:"background,omitempty" json:"background,omitempty" mapstructure:"background"`                      // Background color.
	Border           string `yaml:"border,omitempty" json:"border,omitempty" mapstructure:"border"`                                  // Border style: none, hidden, normal, rounded, thick, double.
	BorderForeground string `yaml:"border_foreground,omitempty" json:"border_foreground,omitempty" mapstructure:"border_foreground"` // Border foreground color.
	BorderBackground string `yaml:"border_background,omitempty" json:"border_background,omitempty" mapstructure:"border_background"` // Border background color.
	Padding          string `yaml:"padding,omitempty" json:"padding,omitempty" mapstructure:"padding"`                               // Padding: "1" or "1 2" or "1 2 1 2" (top, right, bottom, left).
	Margin           string `yaml:"margin,omitempty" json:"margin,omitempty" mapstructure:"margin"`                                  // Margin: "1" or "1 2" or "1 2 1 2" (top, right, bottom, left).
	Width            int    `yaml:"width,omitempty" json:"width,omitempty" mapstructure:"width"`                                     // Fixed width.
	Align            string `yaml:"align,omitempty" json:"align,omitempty" mapstructure:"align"`                                     // Text alignment: left, center, right.
	Bold             bool   `yaml:"bold,omitempty" json:"bold,omitempty" mapstructure:"bold"`                                        // Bold text.
	Italic           bool   `yaml:"italic,omitempty" json:"italic,omitempty" mapstructure:"italic"`                                  // Italic text.
	Underline        bool   `yaml:"underline,omitempty" json:"underline,omitempty" mapstructure:"underline"`                         // Underline text.
	Strikethrough    bool   `yaml:"strikethrough,omitempty" json:"strikethrough,omitempty" mapstructure:"strikethrough"`             // Strikethrough text.
	Faint            bool   `yaml:"faint,omitempty" json:"faint,omitempty" mapstructure:"faint"`                                     // Faint/dim text.
	Markdown         bool   `yaml:"markdown,omitempty" json:"markdown,omitempty" mapstructure:"markdown"`                            // Render content as markdown.

	// Log step fields.
	Level  string            `yaml:"level,omitempty" json:"level,omitempty" mapstructure:"level"`    // Log level: trace, debug, info, warn, error.
	Fields map[string]string `yaml:"fields,omitempty" json:"fields,omitempty" mapstructure:"fields"` // Structured log fields (key-value pairs).

	// Say step fields.
	Voice []string `yaml:"voice,omitempty" json:"voice,omitempty" mapstructure:"voice"` // Ordered voice candidates; first one installed on the host wins (CSS font-family style).
	Rate  string   `yaml:"rate,omitempty" json:"rate,omitempty" mapstructure:"rate"`    // Speech rate: slow, normal, fast.
	Print string   `yaml:"print,omitempty" json:"print,omitempty" mapstructure:"print"` // Print policy: fallback (default), always, never.

	// Environment variables (supports templates).
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`

	// Env step type fields.
	Vars map[string]string `yaml:"vars,omitempty" json:"vars,omitempty" mapstructure:"vars"` // Variables to set for env step type.

	// Exit step type fields.
	Code int `yaml:"code,omitempty" json:"code,omitempty" mapstructure:"code"` // Exit code for exit step type.

	// HTTP step type fields (type: http; also accepts the alias type: webhook).
	URL     string            `yaml:"url,omitempty" json:"url,omitempty" mapstructure:"url"`             // Request URL (required, supports templates).
	Method  string            `yaml:"method,omitempty" json:"method,omitempty" mapstructure:"method"`    // HTTP method/verb: GET (default), POST, PUT, PATCH, DELETE, HEAD, OPTIONS.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty" mapstructure:"headers"` // Request headers (supports templates).
	Query   map[string]string `yaml:"query,omitempty" json:"query,omitempty" mapstructure:"query"`       // Query-string parameters (supports templates).
	Body    string            `yaml:"body,omitempty" json:"body,omitempty" mapstructure:"body"`          // Raw request body (supports templates); mutually exclusive with form.
	Form    map[string]string `yaml:"form,omitempty" json:"form,omitempty" mapstructure:"form"`          // Form/JSON body params; mutually exclusive with body.
	Expect  *HTTPExpect       `yaml:"expect,omitempty" json:"expect,omitempty" mapstructure:"expect"`    // Success criteria; defaults to any 2xx.

	// Container step fields.
	Action            string                `yaml:"action,omitempty" json:"action,omitempty" mapstructure:"action"` // build, push, run, inspect.
	Build             *ContainerBuildStep   `yaml:"build,omitempty" json:"build,omitempty" mapstructure:"build"`
	Push              *ContainerPushStep    `yaml:"push,omitempty" json:"push,omitempty" mapstructure:"push"`
	Run               *ContainerRunStep     `yaml:"run,omitempty" json:"run,omitempty" mapstructure:"run"`
	Inspect           *ContainerInspectStep `yaml:"inspect,omitempty" json:"inspect,omitempty" mapstructure:"inspect"`
	RuntimeAutoStart  bool                  `yaml:"runtime_auto_start,omitempty" json:"runtime_auto_start,omitempty" mapstructure:"runtime_auto_start"`
	Image             string                `yaml:"image,omitempty" json:"image,omitempty" mapstructure:"image"`                                           // Container image to run.
	Shell             string                `yaml:"shell,omitempty" json:"shell,omitempty" mapstructure:"shell"`                                           // Shell used to execute command in container.
	Provider          string                `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`                                  // docker, podman, or empty for auto-detect.
	Pull              string                `yaml:"pull,omitempty" json:"pull,omitempty" mapstructure:"pull"`                                              // missing, always, never.
	Workspace         string                `yaml:"workspace,omitempty" json:"workspace,omitempty" mapstructure:"workspace"`                               // Container workspace path.
	WorkspaceReadOnly bool                  `yaml:"workspace_read_only,omitempty" json:"workspace_read_only,omitempty" mapstructure:"workspace_read_only"` // Mount workspace read-only.
	Cleanup           string                `yaml:"cleanup,omitempty" json:"cleanup,omitempty" mapstructure:"cleanup"`                                     // always, on_success, never.
	User              string                `yaml:"user,omitempty" json:"user,omitempty" mapstructure:"user"`                                              // Container user.
	RunArgs           []string              `yaml:"run_args,omitempty" json:"run_args,omitempty" mapstructure:"run_args"`                                  // Runtime-specific create args.
	Mounts            []ContainerMount      `yaml:"mounts,omitempty" json:"mounts,omitempty" mapstructure:"mounts"`                                        // Extra container mounts.
	Ports             []ContainerPort       `yaml:"ports,omitempty" json:"ports,omitempty" mapstructure:"ports"`                                           // Port mappings.
	Container         *WorkflowContainer    `yaml:"container,omitempty" json:"container,omitempty" mapstructure:"container"`                               // Workflow container override or false to run on host.

	// Outputs declares named outputs derived from the step result.
	Outputs map[string]string `yaml:"outputs,omitempty" json:"outputs,omitempty" mapstructure:"outputs"`

	// Show configuration for this step (overrides workflow-level show settings).
	Show *ShowConfig `yaml:"show,omitempty" json:"show,omitempty" mapstructure:"show"`

	// DryRun is set by executors and is not read from user configuration.
	DryRun bool `yaml:"-" json:"-" mapstructure:"-"`
}

// HTTPExpect defines success criteria for an http step.
// When unset, any 2xx response is considered a success.
type HTTPExpect struct {
	// Status lists acceptable HTTP status codes. When set, the response status must be in this list.
	Status []int `yaml:"status,omitempty" json:"status,omitempty" mapstructure:"status"`
	// Response lists regular expressions; when set, the response body must match at least one.
	// Patterns may be written as /.../ literals (surrounding slashes are stripped) or bare regex strings.
	Response []string `yaml:"response,omitempty" json:"response,omitempty" mapstructure:"response"`
}

// WorkflowDefinition represents a complete workflow with steps.
type WorkflowDefinition struct {
	Description      string `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	WorkingDirectory string `yaml:"working_directory,omitempty" json:"working_directory,omitempty" mapstructure:"working_directory"`
	// Dependencies lists external tools required for this workflow to execute successfully.
	Dependencies *Dependencies      `yaml:"dependencies,omitempty" json:"dependencies,omitempty" mapstructure:"dependencies"`
	Steps        []WorkflowStep     `yaml:"steps" json:"steps" mapstructure:"steps"`
	Stack        string             `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`
	Env          map[string]string  `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`
	Container    *WorkflowContainer `yaml:"container,omitempty" json:"container,omitempty" mapstructure:"container"`

	// Output mode fields.
	Output   string          `yaml:"output,omitempty" json:"output,omitempty" mapstructure:"output"`       // Default output mode for steps.
	Viewport *ViewportConfig `yaml:"viewport,omitempty" json:"viewport,omitempty" mapstructure:"viewport"` // Default viewport settings.
	Show     *ShowConfig     `yaml:"show,omitempty" json:"show,omitempty" mapstructure:"show"`             // Default show settings for steps.
}

type WorkflowConfig map[string]WorkflowDefinition

type WorkflowManifest struct {
	Name        string         `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	Workflows   WorkflowConfig `yaml:"workflows" json:"workflows" mapstructure:"workflows"`
}
