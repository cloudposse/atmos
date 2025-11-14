# Secrets Masking Implementation

**Status**: Implemented
**Version**: 1.0
**Last Updated**: 2025-11-02

## Architecture

### Two-Layer Design

```
┌─────────────────────────────────────────┐
│          Command Layer                   │
│  (cmd/, internal/exec/)                 │
└───────────────┬─────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────┐
│          UI Layer                        │
│  pkg/ui/ - Formatting & Icons           │
│  - ui.Success(), ui.Error(), etc.       │
└───────────────┬─────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────┐
│          I/O Layer                       │
│  pkg/io/ - Streams & Masking            │
│  - io.Data (stdout)                     │
│  - io.UI (stderr)                       │
│  - Automatic masking                    │
└───────────────┬─────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────┐
│      Terminal / Pipe / File              │
└─────────────────────────────────────────┘
```

### Component Architecture

```
pkg/io/
├── context.go         # I/O context with TTY detection
├── masker.go          # Masking engine (interface + impl)
├── global.go          # Global writers for third-party libs
├── streams.go         # Stream wrapper (stdout/stderr/stdin)
└── config.go          # Configuration from flags/env

pkg/ui/
├── ui.go              # UI output functions
├── theme/             # Colors and styles
└── markdown.go        # Markdown rendering
```

## Masking Flow

### 1. Initialization (cmd/root.go)

```go
func init() {
    cobra.OnInitialize(func() {
        // Initialize I/O context with masking
        if err := io.Initialize(); err != nil {
            // Falls back to unmasked os.Stdout/os.Stderr
        }
    })
}
```

### 2. Pattern Registration (pkg/io/global.go)

```go
func Initialize() error {
    // Create I/O context
    globalContext, err := NewContext()
    if err != nil {
        return err
    }

    // Register built-in patterns
    registerCommonSecrets(globalContext.Masker())

    // Set global writers
    Data = globalContext.Streams().Output()  // Masked stdout
    UI = globalContext.Streams().Error()     // Masked stderr

    return nil
}

func registerCommonSecrets(masker Masker) {
    // Register environment variables
    registerEnvValue(masker, "AWS_ACCESS_KEY_ID", true)
    registerEnvValue(masker, "AWS_SECRET_ACCESS_KEY", true)
    registerEnvValue(masker, "GITHUB_TOKEN", true)
    // ... more env vars

    // Register hardcoded patterns
    patterns := []string{
        `ghp_[A-Za-z0-9]{36}`,                    // GitHub PAT (classic)
        `github_pat_[A-Za-z0-9]{22}_[A-Za-z0-9]{59}`, // GitHub PAT (new)
        `gho_[A-Za-z0-9]{36}`,                    // GitHub OAuth
        `glpat-[A-Za-z0-9_-]{20}`,                // GitLab PAT
        `sk-[A-Za-z0-9]{48}`,                     // OpenAI API key
        `AKIA[0-9A-Z]{16}`,                       // AWS Access Key ID
        `Bearer\s+[A-Za-z0-9\-\._~\+\/]+=*`,      // Bearer token
    }

    for _, pattern := range patterns {
        masker.RegisterPattern(pattern)
    }
}
```

### 3. Masking Engine (pkg/io/masker.go)

```go
type maskerImpl struct {
    mu       sync.RWMutex
    patterns []string
    literals map[string]bool
}

func (m *maskerImpl) Mask(data []byte) []byte {
    m.mu.RLock()
    defer m.mu.RUnlock()

    result := data

    // 1. Mask literal values (exact match + encodings)
    for literal := range m.literals {
        result = maskLiteralWithEncodings(result, literal)
    }

    // 2. Mask regex patterns
    for _, pattern := range m.patterns {
        re := regexp.MustCompile(pattern)
        result = re.ReplaceAll(result, []byte("***MASKED***"))
    }

    return result
}

func maskLiteralWithEncodings(data []byte, literal string) []byte {
    result := data

    // Mask literal
    result = bytes.ReplaceAll(result, []byte(literal), []byte("***MASKED***"))

    // Mask URL-encoded
    urlEncoded := url.QueryEscape(literal)
    result = bytes.ReplaceAll(result, []byte(urlEncoded), []byte("***MASKED***"))

    // Mask base64
    b64 := base64.StdEncoding.EncodeToString([]byte(literal))
    result = bytes.ReplaceAll(result, []byte(b64), []byte("***MASKED***"))

    // Mask hex
    hexEncoded := hex.EncodeToString([]byte(literal))
    result = bytes.ReplaceAll(result, []byte(hexEncoded), []byte("***MASKED***"))

    return result
}
```

### 4. Stream Wrapper (pkg/io/streams.go)

```go
type maskedWriter struct {
    writer io.Writer
    masker Masker
}

func (w *maskedWriter) Write(p []byte) (n int, err error) {
    // Mask the data
    masked := w.masker.Mask(p)

    // Write masked data
    return w.writer.Write(masked)
}
```

### 5. Global Writers (pkg/io/global.go)

```go
var (
    // Data is stdout with automatic masking
    Data io.Writer = os.Stdout  // Safe default

    // UI is stderr with automatic masking
    UI io.Writer = os.Stderr    // Safe default
)

// MaskWriter wraps any io.Writer with masking
func MaskWriter(w io.Writer) io.Writer {
    if globalContext == nil {
        Initialize()
    }
    return &maskedWriter{
        writer: w,
        masker: globalContext.Masker(),
    }
}
```

## Configuration

### Flag Binding (cmd/root.go)

```go
func init() {
    RootCmd.PersistentFlags().Bool("mask", true,
        "Enable automatic masking of sensitive data in output (use --mask=false to disable)")
}
```

### Config Loading (pkg/io/context.go)

```go
func buildConfig() *Config {
    cfg := &Config{
        RedirectStderr: viper.GetString("redirect-stderr"),
    }

    // Check if --mask flag was set
    if viper.IsSet("mask") {
        cfg.DisableMasking = !viper.GetBool("mask")
    } else {
        // Default: masking enabled
        cfg.DisableMasking = false
    }

    return cfg
}
```

### Masking Enable/Disable

```go
func NewContext() (Context, error) {
    config := buildConfig()

    var masker Masker
    if config.DisableMasking {
        masker = &noopMasker{}  // Passthrough
    } else {
        masker = NewMasker()    // Active masking
    }

    // ... create streams with masker
}
```

## Error Handling

### Safe Defaults

```go
var (
    // Safe defaults: usable even before Initialize()
    Data io.Writer = os.Stdout
    UI io.Writer = os.Stderr

    globalContext Context
    initOnce      sync.Once
    initErr       error
)

func Initialize() error {
    initOnce.Do(func() {
        globalContext, initErr = NewContext()
        if initErr != nil {
            // Keep safe defaults on error
            Data = os.Stdout
            UI = os.Stderr
            return
        }

        // Upgrade to masked writers
        Data = globalContext.Streams().Output()
        UI = globalContext.Streams().Error()
    })

    return initErr
}
```

### Error Propagation

```go
func RegisterPattern(pattern string) error {
    if pattern == "" {
        return nil
    }

    if globalContext == nil {
        if err := Initialize(); err != nil {
            return fmt.Errorf("failed to initialize global I/O context: %w", err)
        }
    }

    if globalContext == nil {
        return errUtils.ErrIOContextNotInitialized
    }

    return globalContext.Masker().RegisterPattern(pattern)
}
```

## Performance

### Lazy Compilation

Patterns are stored as strings and compiled to regex on first use:

```go
type maskerImpl struct {
    patterns         []string           // Uncompiled
    compiledPatterns []compiledPattern  // Lazily compiled
    compiled         bool
}

func (m *maskerImpl) Mask(data []byte) []byte {
    if !m.compiled {
        m.compilePatterns()
        m.compiled = true
    }
    // Use compiled patterns
}
```

### Benchmarks

```
BenchmarkMaskingNoSecrets-8       500000    2847 ns/op    # No secrets
BenchmarkMaskingSingleSecret-8    200000    8234 ns/op    # 1 secret
BenchmarkMaskingMultipleSecrets-8 100000   15421 ns/op    # 5 secrets
```

**Performance Impact:**
- No secrets: <3μs per operation
- With secrets: <16μs per operation
- Negligible for terminal output

## Thread Safety

### Global Writers

```go
// Safe: io.Writer is goroutine-safe
var Data io.Writer = os.Stdout
var UI io.Writer = os.Stderr

// Safe: sync.Once ensures single initialization
initOnce.Do(func() {
    globalContext, initErr = NewContext()
})
```

### Masker

```go
type maskerImpl struct {
    mu       sync.RWMutex  // Protects patterns and literals
    patterns []string
    literals map[string]bool
}

func (m *maskerImpl) RegisterPattern(pattern string) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.patterns = append(m.patterns, pattern)
    return nil
}

func (m *maskerImpl) Mask(data []byte) []byte {
    m.mu.RLock()
    defer m.mu.RUnlock()
    // ... masking logic
}
```

## Testing

### Unit Tests (pkg/io/global_test.go)

```go
func TestGlobalWriters_AutomaticMasking(t *testing.T) {
    // Test that writes are automatically masked
    var buf bytes.Buffer

    // Redirect global writer to buffer for testing
    originalData := Data
    Data = MaskWriter(&buf)
    defer func() { Data = originalData }()

    // Write secret
    fmt.Fprintf(Data, "API_KEY=%s", "sk-test123...")

    // Verify masked
    assert.Contains(t, buf.String(), "***MASKED***")
    assert.NotContains(t, buf.String(), "sk-test123")
}
```

### Integration Tests (pkg/io/context_test.go)

```go
func TestContext_MaskingEnabled(t *testing.T) {
    ctx, err := NewContext()
    require.NoError(t, err)

    ctx.Masker().RegisterPattern(`sk-[A-Za-z0-9]+`)

    var buf bytes.Buffer
    writer := &maskedWriter{
        writer: &buf,
        masker: ctx.Masker(),
    }

    writer.Write([]byte("key: sk-abc123"))

    assert.Equal(t, "key: ***MASKED***", buf.String())
}
```

## Deployment

### Flag Availability

The `--mask` flag is available on **all commands** via root command's PersistentFlags:

```go
// cmd/root.go
RootCmd.PersistentFlags().Bool("mask", true, "...")
```

All subcommands inherit this flag automatically through Cobra's flag propagation.

### Help Output

The `--mask` flag appears in the **Global Flags** section of all command help:

```
Global Flags:
  --mask    Enable automatic masking of sensitive data
            in output (use --mask=false to disable)
            (default true)
```

### Default Behavior

- Masking is **enabled by default** (`--mask=true`)
- Users must explicitly disable with `--mask=false`
- All output is automatically masked unless disabled

## See Also

- [README](README.md) - Overview and usage
- [Future Considerations](future-considerations.md) - Pattern library integration
- [I/O Handling Strategy PRD](../io-handling-strategy.md) - Architecture decisions
