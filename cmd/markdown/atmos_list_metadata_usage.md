– List all metadata
```shell
 $ atmos list metadata
```

– List specific metadata
```shell
 $ atmos list metadata --query '.component'
```

– Filter by stack pattern
```shell
 $ atmos list metadata --stack '*-dev-*'
```

– Output in different formats
```shell
 $ atmos list metadata --format json
 $ atmos list metadata --format yaml
 $ atmos list metadata --format csv
 $ atmos list metadata --format tsv
```

– Disable Go template processing
```shell
 $ atmos list metadata --process-templates=false
```

– Disable YAML functions processing
```shell
 $ atmos list metadata --process-functions=false
```

- Stack patterns support glob matching (e.g., `*-dev-*`, `prod-*`, `*-{dev,staging}-*`)
