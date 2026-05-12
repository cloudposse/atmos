- List all providers and identities

```shell
$ atmos auth list
```

- List all providers

```shell
$ atmos auth list --providers
```

- List specific provider

```shell
$ atmos auth list --providers aws-sso
```

- List multiple providers

```shell
$ atmos auth list --providers aws-sso,okta
```

- List all identities

```shell
$ atmos auth list --identities
```

- List specific identities

```shell
$ atmos auth list --identities admin,dev,prod
```

- Show in tree format

```shell
$ atmos auth list --format tree
```

- Show identities with chains in tree format

```shell
$ atmos auth list --identities --format tree
```

- Export as JSON

```shell
$ atmos auth list --format json
```

- Export specific provider as YAML

```shell
$ atmos auth list --providers aws-sso --format yaml
```

- Generate Graphviz DOT diagram

```shell
# Install Graphviz (if not already installed)
$ brew install graphviz

# Generate diagram
$ atmos auth list --format graphviz > auth.dot
$ dot -Tpng auth.dot -o auth.png

# Or generate directly to SVG
$ atmos auth list --format graphviz | dot -Tsvg > auth.svg
```

- Generate Mermaid diagram

```shell
$ atmos auth list --format mermaid
```

- Generate Markdown with embedded Mermaid diagram

```shell
$ atmos auth list --format markdown > auth.md
```

- Filter by profile

```shell
$ atmos auth list --profile production
```
