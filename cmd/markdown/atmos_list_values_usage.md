– List all values for a component
```shell
 $ atmos list values <component>
```

– List only variables for a component
```shell
 $ atmos list values <component> --query .vars
```

– List settings for a specific component in a stack
```shell
 $ atmos list values <component> --query .settings --stack 'plat-ue2-*'
```

– Include abstract components
```shell
 $ atmos list values <component> --abstract
```

– Limit number of columns
```shell
 $ atmos list values <component> --max-columns 5
```

– Output in different formats
```shell
 $ atmos list values <component> --format json
 $ atmos list values <component> --format yaml
 $ atmos list values <component> --format csv
 $ atmos list values <component> --format tsv
```

– Filter stacks and include abstract components
```shell
 $ atmos list values <component> --stack '*-prod-*' --abstract
```

– Custom query with specific stack pattern
```shell
 $ atmos list values <component> --query .vars.tags --stack '*-ue2-*'
```

– Apply a custom query
```shell
 $ atmos list values <component> --query '.vars.region'
```

– Filter by stack pattern
```shell
 $ atmos list values <component> --stack '*-ue2-*'
```

– Limit the number of stacks displayed
```shell
 $ atmos list values <component> --max-columns 3
```

– Disable Go template processing
```shell
 $ atmos list values <component> --process-templates=false
```

– Disable YAML functions processing
```shell
 $ atmos list values <component> --process-functions=false
```

- Stack patterns support glob matching (e.g., `*-dev-*`, `prod-*`, `*-{dev,staging}-*`)
