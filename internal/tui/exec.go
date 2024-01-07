package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

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
	Discount     bool
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
				Options(huh.NewOptions("Charmburger Classic", "Chickwich", "Fishburger", "Charmpossible™ Burger")...).
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
				Value(&order.Discount).
				Affirmative("Yes").
				Negative("No"),
		),
	).WithAccessible(accessible)

	err := form.Run()
	if err != nil {
		return err
	}

	{
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
