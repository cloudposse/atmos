– List all variables for a component
```shell
 $ atmos list vars <component>
```

– List specific variables using query
```shell
 $ atmos list vars <component> --query .vars.tags
```

– Filter by stack pattern
```shell
 $ atmos list vars <component> --stack '*-dev-*'
```

– Output in different formats
```shell
 $ atmos list vars <component> --format json
 $ atmos list vars <component> --format yaml
 $ atmos list vars <component> --format csv
 $ atmos list vars <component> --format tsv
```

– Include abstract components
```shell
 $ atmos list vars <component> --abstract
```

– Filter by stack and specific variables
```shell
 $ atmos list vars <component> --stack '*-ue2-*' --query .vars.region
```

– Disable Go template processing
```shell
 $ atmos list vars <component> --process-templates=false
```

– Disable YAML functions processing
```shell
 $ atmos list vars <component> --process-functions=false
```

- Stack patterns support glob matching (e.g., `*-dev-*`, `prod-*`, `*-{dev,staging}-*`)
