- List all providers and identities

```
$ atmos auth list
```

- List all providers

```
$ atmos auth list --providers
```

- List specific provider

```
$ atmos auth list --providers aws-sso
```

- List multiple providers

```
$ atmos auth list --providers aws-sso,okta
```

- List all identities

```
$ atmos auth list --identities
```

- List specific identities

```
$ atmos auth list --identities admin,dev,prod
```

- Show in tree format

```
$ atmos auth list --format tree
```

- Show identities with chains in tree format

```
$ atmos auth list --identities --format tree
```

- Export as JSON

```
$ atmos auth list --format json
```

- Export specific provider as YAML

```
$ atmos auth list --providers aws-sso --format yaml
```

- Generate Graphviz DOT diagram

```
# Install Graphviz (if not already installed)
$ brew install graphviz

# Generate diagram
$ atmos auth list --format graphviz > auth.dot
$ dot -Tpng auth.dot -o auth.png

# Or generate directly to SVG
$ atmos auth list --format graphviz | dot -Tsvg > auth.svg
```

- Generate Mermaid diagram

```
$ atmos auth list --format mermaid
```

- Generate Markdown with embedded Mermaid diagram

```
$ atmos auth list --format markdown > auth.md
```

- Filter by profile

```
$ atmos auth list --profile production
```
