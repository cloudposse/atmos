package markdown

import _ "embed"

// AboutMarkdown contains the content for the about command.
//
//go:embed about.md
var AboutMarkdown string

// DevcontainerUsageMarkdown contains usage examples for the devcontainer command.
//
//go:embed atmos_devcontainer_usage.md
var DevcontainerUsageMarkdown string

// DevcontainerStartUsageMarkdown contains usage examples for the devcontainer start command.
//
//go:embed atmos_devcontainer_start_usage.md
var DevcontainerStartUsageMarkdown string

// DevcontainerStopUsageMarkdown contains usage examples for the devcontainer stop command.
//
//go:embed atmos_devcontainer_stop_usage.md
var DevcontainerStopUsageMarkdown string

// DevcontainerAttachUsageMarkdown contains usage examples for the devcontainer attach command.
//
//go:embed atmos_devcontainer_attach_usage.md
var DevcontainerAttachUsageMarkdown string

// DevcontainerListUsageMarkdown contains usage examples for the devcontainer list command.
//
//go:embed atmos_devcontainer_list_usage.md
var DevcontainerListUsageMarkdown string

// DevcontainerLogsUsageMarkdown contains usage examples for the devcontainer logs command.
//
//go:embed atmos_devcontainer_logs_usage.md
var DevcontainerLogsUsageMarkdown string

// DevcontainerExecUsageMarkdown contains usage examples for the devcontainer exec command.
//
//go:embed atmos_devcontainer_exec_usage.md
var DevcontainerExecUsageMarkdown string

// DevcontainerRemoveUsageMarkdown contains usage examples for the devcontainer remove command.
//
//go:embed atmos_devcontainer_remove_usage.md
var DevcontainerRemoveUsageMarkdown string

// DevcontainerRebuildUsageMarkdown contains usage examples for the devcontainer rebuild command.
//
//go:embed atmos_devcontainer_rebuild_usage.md
var DevcontainerRebuildUsageMarkdown string

// DevcontainerConfigUsageMarkdown contains usage examples for the devcontainer config command.
//
//go:embed atmos_devcontainer_config_usage.md
var DevcontainerConfigUsageMarkdown string

// DevcontainerShellUsageMarkdown contains usage examples for the devcontainer shell command.
//
//go:embed atmos_devcontainer_shell_usage.md
var DevcontainerShellUsageMarkdown string
