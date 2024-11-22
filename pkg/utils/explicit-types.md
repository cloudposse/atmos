// Go templates

a: '{{ (atmos.Component c1 s1).outputs.a }}'
b: '{{ toJSON (atmos.Component c1 s1).outputs.private_subnet_ids }}'
c: !template {{ toJSON (atmos.Component c1 s1).outputs.private_subnet_ids }}

atmos describe component

1. We load and process all YAML files (including all imports), create a Go map in memory, find the final values (after deep-merging) -> 
     we have the final Go map for the component in the stack or stacks
2. If templating is enabled, we convert the final map to YAML again
3. We process the YAML using the Go template engine - it will evaluate all tokens and call our template functions
4. We convert the processed YAML back to Go map


// YAML explicit type functions

a: !terraform.output c1 s1 a
b: !!str sdsd

private_subnet_ids: !terraform.output c1 s1 private_subnet_ids

1. We load and process all YAML files (including all imports), create a Go map in memory, find the final values (after deep-merging) -> 
     we have the final Go map for the component in the stack or stacks
2. We convert the final map to YAML again
3. We decode the YAML and find all the tags (explicit types are tags), and we call the corresponding functions, and set the values


I want to implement:
a: !exec command args


type TaggedField struct {
Tag   string
Value string
}

// Implement UnmarshalYAML for custom tag preservation
func (t *TaggedField) UnmarshalYAML(node *yaml.Node) error {
// Preserve the tag and value
t.Tag = node.Tag
t.Value = node.Value
return nil
}


// TODO
1. Finish the Go code for YAML explicit types
2. !template
3. !exec command args
4. !prepend, !append on lists


