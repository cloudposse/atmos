– List all settings
```
 $ atmos list settings
```

– List specific settings
```
 $ atmos list settings --query '.terraform'
```

– Filter by stack pattern
```
 $ atmos list settings --stack '*-dev-*'
```

– Output in different formats
```
 $ atmos list settings --format json
 $ atmos list settings --format yaml
 $ atmos list settings --format csv
 $ atmos list settings --format tsv
```

– Disable Go template processing
```
 $ atmos list settings --process-templates=false
```

– Disable YAML functions processing
```
 $ atmos list settings --process-functions=false
```

- Stack patterns support glob matching (e.g., `*-dev-*`, `prod-*`, `*-{dev,staging}-*`)
