- Open console with default identity

```shell
atmos auth console
```

- Open console with specific identity

```shell
atmos auth console --identity prod-admin
```

- Open AWS S3 console (using alias)

```shell
atmos auth console --destination s3
```

- Open AWS EC2 console (using alias)

```shell
atmos auth console --destination ec2
```

- Open AWS Lambda console (using full URL)

```shell
atmos auth console --destination https://console.aws.amazon.com/lambda
```

- Print URL without opening browser (useful for scripts)

```shell
atmos auth console --print-only
```

- Generate URL but don't auto-open browser

```shell
atmos auth console --no-open
```

- Specify custom session duration (AWS max: 12h)

```shell
atmos auth console --duration 2h
```

- Custom issuer name (shows in AWS console URL)

```shell
atmos auth console --issuer my-org
```

- Combine options for specific service with longer session

```shell
atmos auth console --identity prod-admin --destination cloudformation --duration 4h
```
