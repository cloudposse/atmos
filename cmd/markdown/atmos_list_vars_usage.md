– List all variables for a component
```
 $ atmos list vars <component>
```

– List specific variables using query
```
 $ atmos list vars <component> --query .vars.tags
```

– Filter by stack pattern
```
 $ atmos list vars <component> --stack '*-dev-*'
```

– Output in different formats
```
 $ atmos list vars <component> --format json
 $ atmos list vars <component> --format yaml
 $ atmos list vars <component> --format csv
 $ atmos list vars <component> --format tsv
```

– Include abstract components
```
 $ atmos list vars <component> --abstract
```

– Filter by stack and specific variables
```
 $ atmos list vars <component> --stack '*-ue2-*' --query .vars.region
```

– Disable Go template processing
```
 $ atmos list vars <component> --process-templates=false
```

– Disable YAML functions processing
```
 $ atmos list vars <component> --process-functions=false
```

- Stack patterns support glob matching (e.g., `*-dev-*`, `prod-*`, `*-{dev,staging}-*`)

