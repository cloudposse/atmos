package picker

import (
	"errors"
	"fmt"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"io"
	"strings"
)

const (
	listHeight   = 10
	defaultWidth = 20
)

var (
	titleStyle        = lipgloss.NewStyle().MarginLeft(2)
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	paginationStyle   = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	helpStyle         = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)
	quitTextStyle     = lipgloss.NewStyle().Margin(1, 0, 2, 4)
)

type item string

func (i item) FilterValue() string { return "" }

type itemDelegate struct{}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, i)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

type SimplePicker struct {
	list     list.Model
	choice   string
	quitting bool
}

func NewSimplePicker(Title string, choices []string) *SimplePicker {
	p := &SimplePicker{}
	var items []list.Item

	for _, option := range choices {
		items = append(items, item(option))
	}
	p.list = list.New(items, itemDelegate{}, defaultWidth, listHeight)
	p.list.Title = Title
	p.list.Styles.Title = titleStyle
	p.list.SetShowStatusBar(false)
	p.list.SetFilteringEnabled(false)
	p.list.Styles.Title = titleStyle
	p.list.Styles.PaginationStyle = paginationStyle
	p.list.Styles.HelpStyle = helpStyle
	return p
}

func (p *SimplePicker) Init() tea.Cmd {
	return nil
}

func (p *SimplePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "enter":
			return p, tea.Quit
		}
	}

	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return p, cmd
}

func (p *SimplePicker) View() string {
	if p.choice != "" {
		return quitTextStyle.Render(fmt.Sprintf("%s selected", p.choice))
	}
	if p.quitting {
		return quitTextStyle.Render("Quitting...")
	}
	return "\n" + p.list.View()
}

func (p *SimplePicker) Choose() (string, error) {
	m, err := tea.NewProgram(p).Run()
	if err != nil {
		return "", err
	}
	i := m.(*SimplePicker).list.SelectedItem()
	s, ok := i.(item)
	if !ok {
		return "", errors.New("SimplePicker.Choose() failed to convert list.Item to string")
	}
	return string(s), nil
}
