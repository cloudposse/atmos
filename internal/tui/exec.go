package tui

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type Spice int

const (
	Mild Spice = iota + 1
	Medium
	Hot
)

func (s Spice) String() string {
	switch s {
	case Mild:
		return "Mild "
	case Medium:
		return "Medium-Spicy "
	case Hot:
		return "Spicy-Hot "
	default:
		return ""
	}
}

type Order struct {
	Burger       Burger
	Side         string
	Name         string
	Instructions string
	Execute      bool
}

type Burger struct {
	Type     string
	Toppings []string
	Spice    Spice
}

// ExecuteExecCmd executes `exec` command
func ExecuteExecCmd(cmd *cobra.Command, args []string) error {
	var burger Burger
	var order = Order{Burger: burger}

	// Should we run in accessible mode?
	accessible, _ := strconv.ParseBool(os.Getenv("ATMOS_TUI_ACCESSIBLE"))

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Options(huh.NewOptions(
					"1",
					"2",
					"3",
					"4",
				)...).
				Title("Choose your burger").
				Description("At Charm we truly have a burger for everyone.").
				Validate(func(t string) error {
					if t == "Fishburger" {
						return fmt.Errorf("no fish today, sorry")
					}
					return nil
				}).
				Value(&order.Burger.Type),

			huh.NewMultiSelect[string]().
				Title("Toppings").
				Description("Choose up to 4.").
				Options(
					huh.NewOption("Lettuce", "Lettuce").Selected(true),
					huh.NewOption("Tomatoes", "Tomatoes").Selected(true),
					huh.NewOption("Charm Sauce", "Charm Sauce"),
					huh.NewOption("Jalapeños", "Jalapeños"),
					huh.NewOption("Cheese", "Cheese"),
					huh.NewOption("Vegan Cheese", "Vegan Cheese"),
					huh.NewOption("Nutella", "Nutella"),
				).
				Validate(func(t []string) error {
					if len(t) <= 0 {
						return fmt.Errorf("at least one topping is required")
					}
					return nil
				}).
				Value(&order.Burger.Toppings).
				Filterable(true).
				Limit(4),

			huh.NewSelect[Spice]().
				Title("Spice level").
				Options(
					huh.NewOption("Mild", Mild).Selected(true),
					huh.NewOption("Medium", Medium),
					huh.NewOption("Hot", Hot),
				).
				Value(&order.Burger.Spice),

			huh.NewSelect[string]().
				Options(huh.NewOptions("Fries", "Disco Fries", "R&B Fries", "Carrots")...).
				Value(&order.Side).
				Title("Sides").
				Description("You get one free side with this order."),

			huh.NewText().
				Value(&order.Instructions).
				Placeholder("Just put it in the mailbox please").
				Title("Special Instructions").
				Description("Anything we should know?").
				CharLimit(400).
				Lines(1),
		),

		huh.NewGroup(
			huh.NewConfirm().
				Title("Execute?").
				Value(&order.Execute).
				Affirmative("Yes").
				Negative("No"),
		),
	).WithAccessible(accessible)

	err := form.Run()
	if err != nil {
		return err
	}

	if order.Execute {
		var sb strings.Builder
		keyword := func(s string) string {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render(s)
		}

		fmt.Fprintf(&sb,
			"%s\n\nOne %s%s, topped with %s with %s on the side.",
			lipgloss.NewStyle().Bold(true).Render("BURGER RECEIPT"),
			keyword(order.Burger.Spice.String()),
			keyword(order.Burger.Type),
			keyword(order.Side),
		)

		fmt.Println(
			lipgloss.NewStyle().
				Width(40).
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("63")).
				Padding(1, 2).
				Render(sb.String()),
		)
	}

	return nil
}

const listHeight = 14

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

type model struct {
	list     list.Model
	choice   string
	quitting bool
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		return m, nil

	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "enter":
			i, ok := m.list.SelectedItem().(item)
			if ok {
				m.choice = string(i)
			}
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.choice != "" {
		return quitTextStyle.Render(fmt.Sprintf("%s? Sounds good to me.", m.choice))
	}
	if m.quitting {
		return quitTextStyle.Render("Not hungry? That’s cool.")
	}
	return "\n" + m.list.View()
}

// ExecuteExecCmd3 executes `exec` command
func ExecuteExecCmd3(cmd *cobra.Command, args []string) error {
	items := []list.Item{
		item("Ramen"),
		item("Tomato Soup"),
		item("Hamburgers"),
		item("Cheeseburgers"),
		item("Currywurst"),
		item("Okonomiyaki"),
		item("Pasta"),
		item("Fillet Mignon"),
		item("Caviar"),
		item("Just Wine"),
	}

	const defaultWidth = 20

	l := list.New(items, itemDelegate{}, defaultWidth, listHeight)
	l.Title = "What do you want for dinner?"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = titleStyle
	l.Styles.PaginationStyle = paginationStyle
	l.Styles.HelpStyle = helpStyle

	m := model{list: l}

	if _, err := tea.NewProgram(m).Run(); err != nil {
		return err
	}

	return nil
}
