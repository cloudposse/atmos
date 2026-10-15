$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$output = Join-Path $env:RUNNER_TEMP 'windows-reserve-diagnostics'
New-Item -ItemType Directory -Force $output | Out-Null
Get-CimInstance Win32_Processor | Format-List Name, Manufacturer | Out-String | Tee-Object "$output/host.txt"
Get-CimInstance Win32_OperatingSystem | Format-List Caption, Version, BuildNumber | Out-String | Tee-Object -Append "$output/host.txt"
Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class ContextSize {
    [DllImport("kernel32.dll", SetLastError = true)]
    public static extern bool InitializeContext(IntPtr buffer, uint flags, ref IntPtr context, ref uint length);
    [DllImport("kernel32.dll")]
    public static extern ulong GetEnabledXStateFeatures();
}
'@
$context = [IntPtr]::Zero
$length = [uint32]0
$ok = [ContextSize]::InitializeContext([IntPtr]::Zero, 0x10005f, [ref]$context, [ref]$length)
if ($ok -or $length -eq 0 -or [Runtime.InteropServices.Marshal]::GetLastWin32Error() -ne 122) {
    throw 'Unexpected InitializeContext sizing result'
}
"Context bytes: $length; XSTATE: $([ContextSize]::GetEnabledXStateFeatures())" | Tee-Object -Append "$output/host.txt"
$version = go env GOVERSION
if ($LASTEXITCODE -ne 0 -or $version -ne 'go1.26.9') { throw "Unexpected toolchain: $version" }
$goroot = go env GOROOT
go version | Tee-Object "$output/toolchain.txt"
$original = Join-Path $goroot 'src/runtime/stack.go'
$source = [IO.File]::ReadAllText($original)
$needle = 'stackSystem = goos.IsWindows*4096 + goos.IsPlan9*512 + goos.IsIos*goarch.IsArm64*1024'
if ([regex]::Matches($source, [regex]::Escape($needle)).Count -ne 1) { throw 'Runtime source does not match the diagnostic patch' }
$patched = Join-Path $output 'stack.go'
[IO.File]::WriteAllText($patched, $source.Replace($needle, $needle.Replace('IsWindows*4096', 'IsWindows*16384')))
$overlay = Join-Path $output 'overlay.json'
@{ Replace = @{ $original = $patched } } | ConvertTo-Json -Depth 4 | Set-Content $overlay
Get-FileHash $original, $patched | Format-List | Out-String | Tee-Object "$output/source-hashes.txt"
$probe = Join-Path $output 'probe'
New-Item -ItemType Directory -Force $probe | Out-Null
Set-Content "$probe/go.mod" "module reserveprobe`n`ngo 1.26.0"
@'
package main

import (
    "fmt"
    "runtime"
    "sync"
)

// Each fresh goroutine faults with its initial stack while other goroutines
// keep adjacent heap spans live. No unsafe pointers or external code are used.
//go:noinline
func fault() {
    defer func() {
        if recover() == nil {
            panic("nil write did not panic")
        }
    }()
    var p *int
    *p = 1
}

func main() {
    stop := make(chan struct{})
    var workers sync.WaitGroup
    for i := 0; i < 2; i++ {
        workers.Add(1)
        go func() {
            defer workers.Done()
            live := make([]*[64]*int, 4096)
            for {
                select {
                case <-stop:
                    runtime.KeepAlive(live)
                    return
                default:
                }
                for j := range live {
                    live[j] = new([64]*int)
                    live[j][j%64] = new(int)
                }
            }
        }()
    }
    for round := 0; round < 40; round++ {
        var faults sync.WaitGroup
        for j := 0; j < 64; j++ {
            faults.Add(1)
            go func() {
                defer faults.Done()
                for k := 0; k < 100; k++ {
                    fault()
                }
            }()
        }
        faults.Wait()
        runtime.GC()
    }
    close(stop)
    workers.Wait()
    fmt.Println("probe completed")
}
'@ | Set-Content "$probe/main.go"
Push-Location $probe
$env:CGO_ENABLED = '0'
$env:GOFLAGS = ''
go build -o "$output/stock.exe" .
if ($LASTEXITCODE -ne 0) { throw 'Stock probe build failed' }
$env:GOFLAGS = "-overlay=$overlay"
go build -o "$output/reserve.exe" .
if ($LASTEXITCODE -ne 0) { throw 'Reserve probe build failed' }
Pop-Location
$results = @()
for ($round = 0; $round -lt 3; $round++) {
    foreach ($mode in @('stock', 'reserve')) {
        $process = Start-Process -FilePath "$output/$mode.exe" -PassThru -NoNewWindow `
            -RedirectStandardOutput "$output/$mode-$round.out" -RedirectStandardError "$output/$mode-$round.err"
        if (-not $process.WaitForExit(120000)) {
            $process.Kill($true)
            $process.WaitForExit()
            $code = 124
        } else {
            $process.WaitForExit()
            $code = $process.ExitCode
        }
        $results += @{ mode = $mode; round = $round; exitCode = $code; contextBytes = $length }
        $results | ConvertTo-Json | Set-Content "$output/results.json"
        if ($mode -eq 'reserve' -and $code -ne 0) { throw "Reserve probe failed: $code" }
    }
}
# Keep the actual stack workflow available to acceptance's matrix verifier.
Push-Location source
$env:CGO_ENABLED = '1'
go test -count=10 -parallel=2 -timeout=15m ./internal/ci/acceptance *> "$output/acceptance-baseline.log"
if ($LASTEXITCODE -ne 0) { throw 'Acceptance baseline failed' }
go test -race -count=5 -parallel=2 -timeout=15m ./internal/ci/acceptance *> "$output/acceptance-race.log"
if ($LASTEXITCODE -ne 0) { throw 'Acceptance race failed' }
$env:CGO_ENABLED = '0'
go test -gcflags=all=-d=checkptr=2 -count=5 -parallel=2 -timeout=15m ./internal/ci/acceptance *> "$output/acceptance-checkptr.log"
if ($LASTEXITCODE -ne 0) { throw 'Acceptance checkptr failed' }
go test -short -timeout=15m -run 'Stack|Exception|Fault|Panic|Signal|Goroutine' runtime *> "$output/runtime.log"
if ($LASTEXITCODE -ne 0) { throw 'Runtime regression suite failed' }
Pop-Location
