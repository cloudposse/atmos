# Watchdog for step-security/harden-runner's Windows post-step race.
#
# harden-runner's Windows agent enforces egress with a DNS proxy on
# 127.0.0.1:53 and repoints every adapter's DNS server to it. Its post step
# waits at most 10 seconds for the agent to finish, then kills it. When the
# agent is still inside "restoring system DNS" at that moment, the adapter is
# left pointing at a proxy that no longer exists; the runner can no longer
# resolve the Actions service and never reports job completion.
#
# This script polls once a second and repairs DNS only when all three hold:
#   1. C:\agent\post_event.json exists (harden-runner's post step has begun),
#   2. the agent process from C:\agent\agent.pid is gone (or the pid file is),
#   3. an IPv4 interface still lists 127.0.0.1 as a DNS server.
# Condition 1 guarantees we never undo enforcement while it is live: an agent
# crash mid-job keeps the job fail-closed exactly as it is today.
param(
    [string]$AgentDir = 'C:\agent',
    [string]$LogPath = "$env:TEMP\atmos-dns-guard.log",
    [int]$MaxSeconds = 7200
)

function Write-Log([string]$Message) {
    $line = "{0} {1}" -f (Get-Date -Format 'o'), $Message
    Add-Content -LiteralPath $LogPath -Value $line -ErrorAction SilentlyContinue
}

function Test-AgentAlive {
    $pidFile = Join-Path $AgentDir 'agent.pid'
    if (-not (Test-Path -LiteralPath $pidFile)) { return $false }
    $raw = (Get-Content -LiteralPath $pidFile -ErrorAction SilentlyContinue | Select-Object -First 1)
    $agentPid = 0
    if (-not [int]::TryParse(($raw -as [string]).Trim(), [ref]$agentPid)) { return $false }
    return $null -ne (Get-Process -Id $agentPid -ErrorAction SilentlyContinue)
}

function Get-LoopbackDnsInterfaces {
    Get-DnsClientServerAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object { $_.ServerAddresses -contains '127.0.0.1' }
}

Write-Log "dns guard started (agentDir=$AgentDir)"
$postEvent = Join-Path $AgentDir 'post_event.json'
$deadline = (Get-Date).AddSeconds($MaxSeconds)

while ((Get-Date) -lt $deadline) {
    Start-Sleep -Seconds 1
    if (-not (Test-Path -LiteralPath $postEvent)) { continue }
    if (Test-AgentAlive) { continue }

    # Give harden-runner's own graceful path a moment: the agent normally
    # restores DNS just before it exits.
    Start-Sleep -Seconds 1
    $bad = @(Get-LoopbackDnsInterfaces)
    if ($bad.Count -eq 0) {
        Write-Log 'post step finished and DNS is healthy; nothing to do'
        break
    }
    foreach ($iface in $bad) {
        Write-Log ("harden-runner agent exited with DNS still on 127.0.0.1 for interface '{0}' (index {1}); resetting" -f $iface.InterfaceAlias, $iface.InterfaceIndex)
        try {
            Set-DnsClientServerAddress -InterfaceIndex $iface.InterfaceIndex -ResetServerAddresses -ErrorAction Stop
        } catch {
            Write-Log ("reset failed for interface index {0}: {1}" -f $iface.InterfaceIndex, $_.Exception.Message)
        }
    }
    Clear-DnsClientCache -ErrorAction SilentlyContinue
    $check = Resolve-DnsName -Name 'pipelines.actions.githubusercontent.com' -Type A -ErrorAction SilentlyContinue
    Write-Log ("DNS reset done; pipelines.actions.githubusercontent.com resolves: {0}" -f ($null -ne $check))
    break
}
Write-Log 'dns guard exiting'
