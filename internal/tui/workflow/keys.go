package workflow

import "github.com/charmbracelet/bubbles/key"

// ShortHelp returns keybindings to be shown in the mini help view. It's part of the key.Map interface.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Up,
		k.Down,
		k.Left,
		k.Right,
		k.Filter,
		k.ClearFilter,
		k.FlipWorkflowStepsView,
		k.Execute,
		k.Quit,
	}
}

// FullHelp returns keybindings for the expanded help view. It's part of the key.Map interface.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Filter, k.ClearFilter, k.FlipWorkflowStepsView},
		{k.Execute, k.Quit},
	}
}

type keyMap struct {
	Up                    key.Binding
	Down                  key.Binding
	Left                  key.Binding
	Right                 key.Binding
	Enter                 key.Binding
	Filter                key.Binding
	ClearFilter           key.Binding
	Quit                  key.Binding
	Escape                key.Binding
	Execute               key.Binding
	CtrlC                 key.Binding
	FlipWorkflowStepsView key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "left"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "right"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "enter"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
	ClearFilter: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "clear filter"),
	),
	Quit: key.NewBinding(
		key.WithKeys("esc", "ctrl+c"),
		key.WithHelp("esc/ctrl+c", "quit"),
	),
	CtrlC: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "esc"),
	),
	Execute: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "execute"),
	),
	FlipWorkflowStepsView: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "flip steps/workflow"),
	),
}
