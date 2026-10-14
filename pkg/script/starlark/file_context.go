package starlark

import (
	"path/filepath"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/cloudposse/atmos/pkg/script"
)

func fileContext(file *script.File, args []string) (starlark.Value, starlark.Value) {
	values := []starlark.Value{}
	var location starlark.Value = starlark.None
	if file != nil {
		if file.Args != nil {
			args = file.Args
		}
		if !file.Stdin {
			location = starlarkstruct.FromStringDict(starlark.String("script"), starlark.StringDict{
				"path": starlark.String(file.Path), "directory": starlark.String(filepath.Dir(file.Path)),
			})
		}
	}
	for _, arg := range args {
		values = append(values, starlark.String(arg))
	}
	result := starlark.NewList(values)
	result.Freeze()
	return result, location
}
