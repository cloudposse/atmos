package atmos

import "github.com/charmbracelet/bubbles/key"

// ShortHelp returns keybindings to be shown in the mini help view. It's part of the key.Map interface
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Filter, k.Quit}
}

// FullHelp returns keybindings for the expanded help view. It's part of the key.Map interface
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Filter},
		{k.Down, k.ClearFilter},
		{k.Left, k.Execute},
		{k.Right, k.Quit},
		{k.FlipStacksComponents},
	}
}

type keyMap struct {
	Up                   key.Binding
	Down                 key.Binding
	Right                key.Binding
	Left                 key.Binding
	Enter                key.Binding
	Filter               key.Binding
	ClearFilter          key.Binding
	Quit                 key.Binding
	Escape               key.Binding
	Execute              key.Binding
	FlipStacksComponents key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "move up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "move down"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "r"),
		key.WithHelp("→/r", "move right"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/l", "move left"),
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
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q/ctrl+c", "quit"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "esc"),
	),
	Execute: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "execute"),
	),
	FlipStacksComponents: key.NewBinding(
		key.WithKeys("f"),
		key.WithHelp("f", "flip componentsStacksMap/stacksComponentsMap"),
	),
}
