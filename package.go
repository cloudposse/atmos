package main

import (
	"fmt"
	"math/rand"
	"os"

	"github.com/charmbracelet/lipgloss"
)

// MaxVersionNumber is the maximum random version number component.
const MaxVersionNumber = 10

var packages = []string{ //nolint:unused
	"vegeutils",
	"libgardening",
	"currykit",
	"spicerack",
	"fullenglish",
	"eggy",
	"bad-kitty",
	"chai",
	"hojicha",
	"libtacos",
	"babys-monads",
	"libpurring",
	"currywurst-devel",
	"xmodmeow",
	"licorice-utils",
	"cashew-apple",
	"rock-lobster",
	"standmixer",
	"coffee-CUPS",
	"libesszet",
	"zeichenorientierte-benutzerschnittstellen",
	"schnurrkit",
	"old-socks-devel",
	"jalapeño",
	"molasses-utils",
	"xkohlrabi",
	"party-gherkin",
	"snow-peas",
	"libyuzu",
}

// Style definitions matching the toolchain look and feel.
var (
	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00ff00")).
			Bold(true)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#87ceeb"))

	packageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffffff"))
)

func getPackages() []string { //nolint:unused
	pkgs := make([]string, len(packages))
	copy(pkgs, packages)

	rand.Shuffle(len(pkgs), func(i, j int) {
		pkgs[i], pkgs[j] = pkgs[j], pkgs[i]
	})

	for k := range pkgs {
		pkgs[k] += fmt.Sprintf("-%d.%d.%d", rand.Intn(MaxVersionNumber), rand.Intn(MaxVersionNumber), rand.Intn(MaxVersionNumber)) //nolint:gosec
	}
	return pkgs
}

// PrintSuccess prints a success message with green checkmark styling.
func PrintSuccess(message string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", successStyle.Render("✅"), message)
}

// PrintInfo prints an info message with blue styling.
func PrintInfo(message string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", infoStyle.Render("ℹ️"), message)
}

// PrintPackage prints a package name with consistent styling.
func PrintPackage(pkg string) {
	fmt.Fprintf(os.Stderr, "%s\n", packageStyle.Render(pkg))
}
