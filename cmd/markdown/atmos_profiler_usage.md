- Enable profiler with configuration file

```
# atmos.yaml
profiler:
  enabled: true
  host: "localhost"
  port: 6060
```

- Enable profiler with command-line flags

```
$ atmos terraform plan vpc -s plat-ue2-dev --profiler-enabled
pprof profiler available at: http://localhost:6060/debug/pprof/
```

- Access profiler web interface

```
$ open http://localhost:6060/debug/pprof/
```

- Capture CPU profile for analysis

```
$ go tool pprof http://localhost:6060/debug/pprof/profile
```

- Capture memory profile for analysis

```
$ go tool pprof http://localhost:6060/debug/pprof/heap
```

- Generate web visualization of performance data

```
$ go tool pprof -http=:8080 http://localhost:6060/debug/pprof/profile
```

- Run with custom profiler host and port

```
$ atmos terraform apply vpc -s plat-ue2-prod --profiler-enabled --profiler-host=0.0.0.0 --profiler-port=8060
```