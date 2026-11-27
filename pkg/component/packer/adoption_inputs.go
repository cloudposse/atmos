package packer

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	stackadoption "github.com/cloudposse/atmos/pkg/stack/adoption"
)

// Read assignment names without evaluating native expressions or printing input values.
func reportPackerInputs(r *stackadoption.Registration, dir string, variables map[string]bool) error {
	inputs, err := automaticPackerInputs(dir)
	if err != nil {
		return err
	}
	var unknown []string
	for _, name := range append(inputs, r.VarFiles...) {
		attrs, err := packerAssignments(r.Project, dir, name)
		if err != nil {
			return err
		}
		for key := range attrs {
			if _, ok := variables[key]; ok {
				variables[key] = false
			} else {
				unknown = append(unknown, key)
			}
		}
	}
	runtime := missingPackerEnvironment(variables)
	slices.Sort(unknown)
	if len(unknown) > 0 {
		fmt.Fprintln(r.Output, "Inputs without a Packer variable declaration:", strings.Join(slices.Compact(unknown), ", "))
	}
	if len(runtime) > 0 {
		fmt.Fprintln(r.Output, "Required Packer inputs: set", strings.Join(runtime, ", "), "when running Packer, or select a --var-file that supplies them.")
	}
	return nil
}

func packerAssignments(project, dir, name string) (hcl.Attributes, error) {
	path := name
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("variable file %q (relative to %s): %w", name, dir, err)
	}
	if _, err := stackadoption.RelativeDirectory(project, path); err != nil {
		return nil, err
	}
	file, err := parsePackerFile(path)
	if err != nil {
		return nil, err
	}
	attrs, diags := file.Body.JustAttributes()
	if diags.HasErrors() {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrInvalidComponent, diags.Error())
	}
	return attrs, nil
}

func missingPackerEnvironment(variables map[string]bool) []string {
	var runtime []string
	for name, required := range variables {
		if !required {
			continue
		}
		env := "PKR_VAR_" + name
		if _, ok := os.LookupEnv(env); !ok {
			runtime = append(runtime, env)
		}
	}
	slices.Sort(runtime)
	return runtime
}

func automaticPackerInputs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var inputs []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && (strings.HasSuffix(name, ".auto.pkrvars.hcl") || strings.HasSuffix(name, ".auto.pkrvars.json")) {
			inputs = append(inputs, name)
		}
	}
	return inputs, nil
}
