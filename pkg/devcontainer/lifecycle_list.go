package devcontainer

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	tableWidth = 92 // Width for devcontainer list table including indicator column.
)

// List lists all available devcontainers with running status.
func (m *Manager) List(atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "devcontainer.List")()

	configs, err := m.configLoader.LoadAllConfigs(atmosConfig)
	if err != nil {
		return err
	}

	if len(configs) == 0 {
		_ = ui.Infof("No devcontainers configured")
		return nil
	}

	// Get runtime and list running containers.
	runtime, err := m.runtimeDetector.DetectRuntime("")
	if err != nil {
		return fmt.Errorf("%w: failed to initialize container runtime: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	ctx := context.Background()
	runningContainers, err := runtime.List(ctx, nil)
	if err != nil {
		return fmt.Errorf("%w: failed to list containers: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	// Build set of running devcontainer names.
	runningNames := make(map[string]bool)
	for _, c := range runningContainers {
		if IsAtmosDevcontainer(c.Name) {
			if name, _ := ParseContainerName(c.Name); name != "" {
				if c.Status == "running" {
					runningNames[name] = true
				}
			}
		}
	}

	// Render the table using lipgloss.
	renderListTable(configs, runningNames, runningContainers)
	return nil
}

// getPortsForDisplay returns the appropriate port string for display.
// For running containers, uses actual runtime ports from container.Info.
// For stopped containers, returns configured ports.
func getPortsForDisplay(name string, config *Config, runningContainers []container.Info, runningNames map[string]bool) string {
	defer perf.Track(nil, "devcontainer.getPortsForDisplay")()

	// Use actual runtime ports for running containers.
	if !runningNames[name] {
		// Show configured ports for stopped containers.
		ports, _ := ParsePorts(config.ForwardPorts, config.PortsAttributes)
		return FormatPortBindings(ports)
	}

	// Find matching running container.
	for _, c := range runningContainers {
		if !IsAtmosDevcontainer(c.Name) {
			continue
		}
		if containerName, _ := ParseContainerName(c.Name); containerName != name {
			continue
		}
		if len(c.Ports) > 0 {
			return FormatPortBindings(c.Ports)
		}
		break
	}

	// No ports in runtime info - show configured ports.
	ports, _ := ParsePorts(config.ForwardPorts, config.PortsAttributes)
	return FormatPortBindings(ports)
}

// renderListTable renders devcontainer list as a formatted table.
func renderListTable(configs map[string]*Config, runningNames map[string]bool, runningContainers []container.Info) {
	// Sort names for consistent output.
	var names []string
	for name := range configs {
		names = append(names, name)
	}
	sort.Strings(names)

	// Build table rows.
	var rows []string
	for _, name := range names {
		config := configs[name]

		// Determine status indicator.
		indicator := " "
		if runningNames[name] {
			indicator = theme.Styles.NewVersion.Render("●") // Green dot for running.
		}

		// Get image name.
		image := config.Image
		if image == "" && config.Build != nil {
			image = fmt.Sprintf("(build: %s)", config.Build.Dockerfile)
		}

		// Get ports using helper function.
		portsStr := getPortsForDisplay(name, config, runningContainers, runningNames)

		// Format row.
		row := fmt.Sprintf("%s %-20s %-40s %-30s", indicator, name, image, portsStr)
		rows = append(rows, row)
	}

	// Print header with bold styling.
	headerStyle := lipgloss.NewStyle().Bold(true)
	fmt.Printf("%s %-20s %-40s %-30s\n", " ",
		headerStyle.Render("NAME"),
		headerStyle.Render("IMAGE"),
		headerStyle.Render("PORTS"))

	// Print separator.
	fmt.Println(strings.Repeat("─", tableWidth))

	// Print rows.
	for _, row := range rows {
		fmt.Println(row)
	}
}
