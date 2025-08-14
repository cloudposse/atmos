package auth

import (
	"github.com/charmbracelet/huh"
	"github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/schema"
	"os"
)

func IsInDocker() bool {
	if _, err := os.Stat("/.dockerenv"); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// pickKeyFromMap presents a simple picker to the user, where The user is asked to choose
// a key from the map
//
// If the user cancels the picker, an error is returned.
func pickKeyFromMap(Map map[string]any) (string, error) {
	items := []string{}
	for k, _ := range Map {
		items = append(items, k)
	}
	choice := ""
	selector := huh.NewSelect[string]().
		Value(&choice).
		OptionsFunc(func() []huh.Option[string] {
			return huh.NewOptions(items...)
		}, nil).
		Height(8).
		Title("Choose an Identity").
		WithTheme(utils.NewAtmosHuhTheme())
	selector.Run()
	return choice, nil
}

// Todo, would be cool to have huh model that shows info of the identity
func pickIdentity(identities map[string]schema.Identity) (string, error) {
	items := []string{}
	for k, _ := range identities {
		items = append(items, k)
	}
	choice := ""
	selector := huh.NewSelect[string]().
		Value(&choice).
		OptionsFunc(func() []huh.Option[string] {
			return huh.NewOptions(items...)
		}, nil).
		Height(8).
		Title("Choose an Identity").
		WithTheme(utils.NewAtmosHuhTheme())

	selector.Run()
	return choice, nil
}
