- Enable profiler with configuration file

```
# atmos.yaml
profiler:
  enabled: true
  host: "localhost"
  port: 6060
```

- Enable profiler server with command-line flags

```
$ atmos terraform plan vpc -s plat-ue2-dev --profiler-enabled
profiler available at: http://localhost:6060/debug/pprof/
```

- Write CPU profile to file (recommended for CLI tools)

```
$ atmos terraform plan vpc -s plat-ue2-dev --profile-file=cpu.prof
CPU profiling started file=cpu.prof
CPU profiling completed file=cpu.prof
```

- Analyze CPU profile file

```
# Interactive text mode
$ go tool pprof cpu.prof

# Web interface (requires Graphviz: brew install graphviz)
$ go tool pprof -http=:8080 cpu.prof

# Direct text output
$ go tool pprof -top cpu.prof
```

- Access profiler web interface (when using server mode)

```
$ open http://localhost:6060/debug/pprof/
```

- Capture CPU profile from server for analysis

```
$ go tool pprof http://localhost:6060/debug/pprof/profile
```

- Capture memory profile from server for analysis

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
