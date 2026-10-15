$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$output = Join-Path $env:RUNNER_TEMP 'windows-nested-build-environment'
New-Item -ItemType Directory -Force $output | Out-Null
Copy-Item internal/ci/acceptance/run.go source/internal/ci/acceptance/run.go
Copy-Item internal/ci/acceptance/precompiled_env_test.go source/internal/ci/acceptance/precompiled_env_test.go
Get-CimInstance Win32_Processor | Format-List Name, Manufacturer | Out-String | Set-Content "$output/host.txt"
Push-Location source
$env:CGO_ENABLED = '1'
$env:GOFIPS140 = 'off'
go env CGO_ENABLED GOFIPS140 GOVERSION | Set-Content "$output/ambient-env.txt"
go test ./internal/ci/acceptance -run '^TestPrecompiledTestsPreserveBuildEnvironment$' -count=1 -timeout 10m *> "$output/propagation.log"
if ($LASTEXITCODE -ne 0) { throw 'Precompiled environment propagation regression failed' }
$bad = Join-Path $output 'ambient-atmos.exe'
go build -buildvcs=false -o $bad . *> "$output/ambient-build.log"
if ($LASTEXITCODE -ne 0) { throw 'Ambient CLI build failed' }
& $bad version *> "$output/ambient-version.log"
$ambientExit = $LASTEXITCODE
if ($ambientExit -ne 0 -and -not (Select-String -Path "$output/ambient-version.log" -SimpleMatch 'fatal: morestack on g0' -Quiet)) {
    throw 'Ambient CLI failed for an unexpected reason'
}
$env:CGO_ENABLED = '0'
$env:GOFIPS140 = 'latest'
$good = Join-Path $output 'release-env-atmos.exe'
go build -buildvcs=false -o $good . *> "$output/release-build.log"
if ($LASTEXITCODE -ne 0) { throw 'Release-environment CLI build failed' }
& $good version *> "$output/release-version.log"
if ($LASTEXITCODE -ne 0) { throw 'Release-environment CLI startup failed' }
go test ./tests -run '^(TestDescribeCommandsWithIdentityFlag|TestProfileFlagIntegration)$' -count=1 -timeout 15m *> "$output/real-cli-tests.log"
if ($LASTEXITCODE -ne 0) { throw 'Real CLI subprocess regressions failed' }
@{ ambientCGO = 1; ambientExit = $ambientExit; fixedCGO = 0; fixedExit = 0; propagation = 'passed'; realCLI = 'passed' } | ConvertTo-Json | Set-Content "$output/results.json"
Pop-Location
