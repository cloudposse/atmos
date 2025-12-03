# remark-stack-example

A Remark plugin for Docusaurus that transforms YAML code blocks with the `stack-example` meta tag into tabbed multi-format views showing equivalent YAML, JSON, and HCL configurations.

## Usage

In your MDX files, use the `stack-example` meta tag on YAML code blocks:

~~~markdown
```yaml stack-example
settings:
  region: !env AWS_REGION
  timeout: 30
```
~~~

This will be automatically transformed into a tabbed component showing:
- **YAML**: The original YAML with Atmos function syntax
- **JSON**: Equivalent JSON with interpolation syntax
- **HCL**: Equivalent HCL with function call syntax

## Function Syntax Translation

The plugin automatically translates Atmos function syntax between formats:

| YAML | JSON | HCL |
|------|------|-----|
| `!env VAR` | `${env:VAR}` | `atmos_env("VAR")` |
| `!exec "cmd"` | `${exec:cmd}` | `atmos_exec("cmd")` |
| `!template "..."` | `${template:...}` | `atmos_template("...")` |
| `!repo-root` | `${repo-root}` | `atmos_repo_root()` |
| `!terraform.output ...` | `${terraform.output:...}` | `atmos_terraform_output(...)` |
| `!terraform.state ...` | `${terraform.state:...}` | `atmos_terraform_state(...)` |
| `!store provider/key` | `${store:provider/key}` | `atmos_store("provider", "key")` |

## Direct Component Usage

You can also use the `StackExample` component directly in MDX:

```jsx
import StackExample from '@site/src/components/StackExample';

<StackExample
  yaml={`settings:
  region: !env AWS_REGION`}
  json={`{
  "settings": {
    "region": "\${env:AWS_REGION}"
  }
}`}
  hcl={`settings = {
  region = atmos_env("AWS_REGION")
}`}
/>
```

## Installation

The plugin is automatically included in the Atmos documentation website. It requires:

- `js-yaml` for YAML parsing
- `unist-util-visit` for AST traversal
