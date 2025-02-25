– List all values for a component
```
 $ atmos list values <component>
```

– List only variables for a component
```
 $ atmos list values <component> --query .vars
```

– List settings for a specific component in a stack
```
 $ atmos list values <component> --query .settings --stack 'plat-ue2-*'
```

– Include abstract components
```
 $ atmos list values <component> --abstract
```

– Limit number of columns
```
 $ atmos list values <component> --max-columns 5
```

– Output in different formats
```
 $ atmos list values <component> --format json
 $ atmos list values <component> --format yaml
 $ atmos list values <component> --format csv
 $ atmos list values <component> --format tsv
```

– Filter stacks and include abstract components
```
 $ atmos list values <component> --stack '*-prod-*' --abstract
```

– Custom query with specific stack pattern
```
 $ atmos list values <component> --query .vars.tags --stack '*-ue2-*'
```
- Stack patterns support glob matching (e.g., `*-dev-*`, `prod-*`, `*-{dev,staging}-*`)

