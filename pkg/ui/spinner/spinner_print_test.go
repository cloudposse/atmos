package spinner

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// heldSpinnerCommands keeps ordinary model commands pending until the test ends.
// This exercises a valid slow-command schedule in the real Bubble Tea event loop:
// direct print messages must not wait on, or be overtaken by, asynchronous commands.
type heldSpinnerCommands struct {
	manualSpinnerModel
	release <-chan struct{}
}

func (m *heldSpinnerCommands) Init() tea.Cmd { return nil }

func (m *heldSpinnerCommands) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.manualSpinnerModel.Update(msg)
	m.manualSpinnerModel = model.(manualSpinnerModel)
	if cmd == nil {
		return m, nil
	}
	if _, stopping := msg.(manualStopMsg); stopping {
		return m, cmd
	}
	return m, func() tea.Msg {
		<-m.release
		return cmd()
	}
}

func startPrintTestProgram(t *testing.T, model tea.Model) (*Spinner, *bytes.Buffer, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	output := &bytes.Buffer{}
	program := tea.NewProgram(model, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(output),
		tea.WithoutSignalHandler(), tea.WithFPS(1))
	s := &Spinner{isTTY: true, program: program, done: make(chan struct{})}
	runErr := make(chan error, 1)
	go func() {
		_, err := program.Run()
		runErr <- err
		close(s.done)
	}()
	t.Cleanup(func() {
		program.Kill()
		<-s.done
	})
	return s, output, runErr
}

func TestSpinnerPrintln_FlushesInOrderBeforeShutdown(t *testing.T) {
	for _, finish := range []string{"stop", "success", "error"} {
		t.Run(finish, func(t *testing.T) {
			release := make(chan struct{})
			t.Cleanup(func() { close(release) })
			s, output, runErr := startPrintTestProgram(t, &heldSpinnerCommands{
				manualSpinnerModel: newManualSpinnerModel("Watching events"), release: release,
			})
			lines := make([]string, 32)
			for i := range lines {
				lines[i] = fmt.Sprintf("resource-%02d CREATE_COMPLETE", i)
				s.Println(lines[i])
			}
			s.Println("last-resource CREATE_FAILED: exact failure reason")
			switch finish {
			case "success":
				s.Success("Finished watching")
			case "error":
				s.Error("Finished watching")
			default:
				s.Stop()
			}
			require.NoError(t, <-runErr)
			text := strings.ReplaceAll(output.String(), "\r\n", "\n")
			want := strings.Join(lines, "\n") + "\nlast-resource CREATE_FAILED: exact failure reason\n"
			require.Contains(t, text, want, "all permanent lines must be rendered in submission order")
			if finish != "stop" {
				require.Greater(t, strings.Index(text, "Finished watching"), strings.Index(text, "exact failure reason"))
			}
		})
	}
}

func TestSpinnerPrintln_AfterProgramExitDoesNotBlock(t *testing.T) {
	s, _, runErr := startPrintTestProgram(t, newManualSpinnerModel("Watching events"))
	// Ctrl+C can exit Bubble Tea before the operation notices and stops its spinner.
	s.program.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	<-s.done
	require.NoError(t, <-runErr)
	returned := make(chan struct{})
	go func() {
		s.Println("late event after terminal exit")
		s.Stop()
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("printing after program exit must not block")
	}
}
