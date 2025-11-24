– List all vendor configurations
```shell
 $ atmos list vendor
```

– List vendor configurations with a specific output format
```shell
 $ atmos list vendor --format json
```

– List vendor configurations filtered by component name pattern
```shell
 $ atmos list vendor --stack "vpc*"
```

– List vendor configurations with comma-separated CSV format
```shell
 $ atmos list vendor --format csv
```

– List vendor configurations with tab-separated TSV format
```shell
 $ atmos list vendor --format tsv
```

– List vendor configurations with a custom delimiter
```shell
 $ atmos list vendor --format csv --delimiter "|"
```
