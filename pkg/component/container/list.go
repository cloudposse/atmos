package container

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	ctr "github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	runningDot = "●"

	statusRunning = "running"
	statusStopped = "stopped"
	statusUnknown = "unknown"
)

// instanceRow is a single container component instance and its running state.
type instanceRow struct {
	stack     string
	component string
	image     string
	status    string // running | stopped | unknown
	running   bool
}

// ExecuteList lists all container components across stacks (optionally filtered
// by --stack) with their running state, discovered by label. This is the
// container-specific listing; running state is intentionally NOT surfaced by the
// generic `atmos list components` (which treats all component kinds uniformly).
func ExecuteList(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "container.ExecuteList")()

	info.ComponentType = cfg.ContainerComponentType
	atmosConfig, err := initCliConfig(*info, true)
	if err != nil {
		return emptyListOrError(err)
	}

	stacksMap, err := e.ExecuteDescribeStacks(
		&atmosConfig, info.Stack, nil,
		[]string{cfg.ContainerComponentType}, nil,
		false, false, false, false, nil, nil,
	)
	if err != nil {
		return emptyListOrError(err)
	}

	rows := collectContainerInstances(stacksMap)
	if len(rows) == 0 {
		ui.Info("No container components found")
		return nil
	}

	// Detect the runtime once (non-fatal) and annotate each row's running state.
	runtime, rtErr := detectRuntime(ctx, atmosConfig.Container.Runtime.Provider, atmosConfig.Container.Runtime.AutoStart)
	if rtErr != nil {
		runtime = nil
	}
	annotateRunningState(ctx, runtime, rows)

	return renderInstanceTable(rows)
}

// emptyListOrError degrades a "no stacks/imports" error (e.g. running outside an
// Atmos project) into a clean empty listing, and propagates any other error.
func emptyListOrError(err error) error {
	if errors.Is(err, errUtils.ErrFailedToFindImport) || errors.Is(err, errUtils.ErrNoStacksFound) {
		ui.Info("No container components found")
		return nil
	}
	return err
}

// collectContainerInstances walks the described stacks and returns every
// (non-abstract) container component instance, sorted by stack then component.
func collectContainerInstances(stacksMap map[string]any) []instanceRow {
	var rows []instanceRow
	for stackName, stackData := range stacksMap {
		stackMap, ok := stackData.(map[string]any)
		if !ok {
			continue
		}
		componentsMap, ok := stackMap[cfg.ComponentsSectionName].(map[string]any)
		if !ok {
			continue
		}
		containerMap, ok := componentsMap[cfg.ContainerComponentType].(map[string]any)
		if !ok {
			continue
		}
		for component, compData := range containerMap {
			if isAbstractComponent(compData) {
				continue // abstract components are blueprints, not deployable instances
			}
			rows = append(rows, instanceRow{
				stack:     stackName,
				component: component,
				image:     imageFromComponent(compData),
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].stack != rows[j].stack {
			return rows[i].stack < rows[j].stack
		}
		return rows[i].component < rows[j].component
	})
	return rows
}

// isAbstractComponent reports whether a component section is marked
// `metadata.type: abstract` (a non-deployable blueprint).
func isAbstractComponent(compData any) bool {
	compMap, ok := compData.(map[string]any)
	if !ok {
		return false
	}
	metadata, ok := compMap["metadata"].(map[string]any)
	if !ok {
		return false
	}
	t, _ := metadata["type"].(string)
	return t == "abstract"
}

// imageFromComponent extracts the first-class top-level `image` from a component
// section, if present.
func imageFromComponent(compData any) string {
	compMap, ok := compData.(map[string]any)
	if !ok {
		return ""
	}
	image, _ := compMap["image"].(string)
	return image
}

// annotateRunningState fills each row's running state via label discovery. When
// no runtime is available, rows are marked "unknown".
func annotateRunningState(ctx context.Context, runtime ctr.Runtime, rows []instanceRow) {
	for i := range rows {
		if runtime == nil {
			rows[i].status = statusUnknown
			continue
		}
		in, found, err := ctr.FindInstance(ctx, runtime, rows[i].stack, cfg.ContainerComponentType, rows[i].component)
		switch {
		case err != nil:
			rows[i].status = statusUnknown
		case found && ctr.IsContainerRunning(in.Status):
			rows[i].status = statusRunning
			rows[i].running = true
		default:
			rows[i].status = statusStopped
		}
	}
}

// renderInstanceTable prints the container instances as an aligned table to the
// data channel, with a colored dot indicator on a TTY.
func renderInstanceTable(rows []instanceRow) error {
	tty := terminal.New().IsTTY(terminal.Stdout)

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "  \tSTACK\tCOMPONENT\tIMAGE\tSTATUS")
	for _, row := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			statusDot(row.status, tty), row.stack, row.component, row.image, row.status)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return data.Write(buf.String())
}

// statusDot returns a colored dot for a TTY (green=running, gray otherwise) or a
// space on non-TTY output so machine consumers read the STATUS column instead.
func statusDot(status string, tty bool) string {
	if !tty {
		return " "
	}
	color := theme.ColorDarkGray
	if status == statusRunning {
		color = theme.GetSuccessColor()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(runningDot)
}
