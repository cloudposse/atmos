package starlark

import (
	"path/filepath"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/cloudposse/atmos/pkg/script"
)

func fileContext(file *script.File) (starlark.Value, starlark.Value) {
	values := []starlark.Value{}
	var location starlark.Value = starlark.None
	if file != nil {
		for _, arg := range file.Args {
			values = append(values, starlark.String(arg))
		}
		location = starlarkstruct.FromStringDict(starlark.String("script"), starlark.StringDict{
			"path": starlark.String(file.Path), "directory": starlark.String(filepath.Dir(file.Path)),
		})
	}
	args := starlark.NewList(values)
	args.Freeze()
	return args, location
}
