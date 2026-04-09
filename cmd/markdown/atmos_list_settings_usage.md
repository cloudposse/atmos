– List all settings
```shell
 $ atmos list settings
```

– List specific settings
```shell
 $ atmos list settings --query '.terraform'
```

– Filter by stack pattern
```shell
 $ atmos list settings --stack '*-dev-*'
```

– Output in different formats
```shell
 $ atmos list settings --format json
 $ atmos list settings --format yaml
 $ atmos list settings --format csv
 $ atmos list settings --format tsv
```

– Disable Go template processing
```shell
 $ atmos list settings --process-templates=false
```

– Disable YAML functions processing
```shell
 $ atmos list settings --process-functions=false
```

- Stack patterns support glob matching (e.g., `*-dev-*`, `prod-*`, `*-{dev,staging}-*`)
