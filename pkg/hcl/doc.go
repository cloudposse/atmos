// Package hcl provides address-based HCL editing for Terraform component
// files, preserving comments and formatting.
//
// It wraps github.com/minamijoyo/hcledit/editor, the same engine behind the
// hcledit CLI, and mirrors pkg/yaml's Get/Set/Delete-style API so structural
// edits to .tf files follow the same shape as structural edits to YAML stack
// manifests.
//
// Example usage:
//
//	out, err := hcl.SetAttribute(content, "resource.aws_instance.web.instance_type", `"t3.micro"`)
//	if err != nil {
//	    return err
//	}
package hcl
