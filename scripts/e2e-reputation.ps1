# WhatGate end-to-end: reputation gate (M-Phase 2.6/2.7).
# Proves the abuse loop closes: hitting a blocked destination lowers the
# requester's reputation, and an exit with -min-reputation then refuses that
# requester even for ALLOWED destinations — while a fresh peer is unaffected.
#
#   1) abuser curls a blocked domain            -> refused, reputation -> -10
#   2) abuser curls an ALLOWED domain           -> refused (reputation below threshold)
#   3) a fresh control peer curls that domain   -> succeeds (reputation 0)
#
# Prereq: go build -o bin/ ./...
# Run:    pwsh scripts/e2e-reputation.ps1

$ErrorActionPreference = 'Stop'
$bin  = Join-Path (Split-Path -Parent $PSScriptRoot) 'bin'
$logs = Join-Path $env:TEMP 'whatgate-e2e'
New-Item -ItemType Directory -Force -Path $logs | Out-Null

Get-Process node,coordinator -EA SilentlyContinue | Stop-Process -Force -EA SilentlyContinue
Start-Sleep -Milliseconds 400

function Wait-ForLine($path,$pattern,$timeoutSec){
    $deadline=(Get-Date).AddSeconds($timeoutSec)
    while((Get-Date) -lt $deadline){
        if(Test-Path $path){ $m=Select-String -Path $path -Pattern $pattern -EA SilentlyContinue | Select-Object -First 1; if($m){return $m.Line} }
        Start-Sleep -Milliseconds 200
    }
    return $null
}
function Egress($socks,$target){ (& curl.exe -s --max-time 12 --socks5-hostname "127.0.0.1:$socks" "https://$target" | Select-Object -First 1) }

$procs=@()
try{
    $p=Start-Process "$bin\coordinator.exe" -ArgumentList '-addr','127.0.0.1:8097','-invite','welcome' -RedirectStandardOutput "$logs\rep-co.out" -RedirectStandardError "$logs\rep-co.err" -WindowStyle Hidden -PassThru; $procs+=$p
    if(-not (Wait-ForLine "$logs\rep-co.out" 'coordinator listening' 10)){ throw "coordinator not up" }

    $e=Start-Process "$bin\node.exe" -ArgumentList '-listen','/ip4/127.0.0.1/tcp/45770','-coordinator','http://127.0.0.1:8097','-invite','welcome','-exit','-region','JP','-block-domains','api.ipify.org','-min-reputation','0' -RedirectStandardOutput "$logs\rep-e.out" -RedirectStandardError "$logs\rep-e.err" -WindowStyle Hidden -PassThru; $procs+=$e
    if(-not (Wait-ForLine "$logs\rep-e.out" 'exit: ENABLED' 15)){ throw "exit not ready" }
    Start-Sleep -Seconds 2

    $c1=Start-Process "$bin\node.exe" -ArgumentList '-listen','/ip4/127.0.0.1/tcp/45771','-coordinator','http://127.0.0.1:8097','-invite','welcome','-to','JP','-trust-scope','open','-socks','127.0.0.1:1086' -RedirectStandardOutput "$logs\rep-c1.out" -RedirectStandardError "$logs\rep-c1.err" -WindowStyle Hidden -PassThru; $procs+=$c1
    $c2=Start-Process "$bin\node.exe" -ArgumentList '-listen','/ip4/127.0.0.1/tcp/45772','-coordinator','http://127.0.0.1:8097','-invite','welcome','-to','JP','-trust-scope','open','-socks','127.0.0.1:1087' -RedirectStandardOutput "$logs\rep-c2.out" -RedirectStandardError "$logs\rep-c2.err" -WindowStyle Hidden -PassThru; $procs+=$c2
    if(-not (Wait-ForLine "$logs\rep-c1.out" 'SOCKS5 proxy on' 15)){ throw "client1 not ready" }
    if(-not (Wait-ForLine "$logs\rep-c2.out" 'SOCKS5 proxy on' 15)){ throw "client2 not ready" }
    $id1=((Select-String -Path "$logs\rep-c1.out" -Pattern 'peer id: (\S+)').Matches.Groups[1].Value)

    $r=@()
    $abused = Egress 1086 'api.ipify.org'
    $ok1 = -not ($abused -match '^\d+\.\d+\.\d+\.\d+$'); $r+=$ok1
    Write-Host ("[{0}] abuser -> blocked domain refused" -f $(if($ok1){"PASS"}else{"FAIL"}))
    Start-Sleep -Seconds 2
    $rep = & curl.exe -s "http://127.0.0.1:8097/reputation?peer=$id1"
    $ok2 = ($rep -match '-10'); $r+=$ok2
    Write-Host ("[{0}] abuser reputation dropped: {1}" -f $(if($ok2){"PASS"}else{"FAIL"}),$rep)

    $blockedNow = Egress 1086 'ifconfig.me'
    $ok3 = -not ($blockedNow -match '^\d+\.\d+\.\d+\.\d+$'); $r+=$ok3
    Write-Host ("[{0}] abuser -> ALLOWED domain now refused (low reputation)" -f $(if($ok3){"PASS"}else{"FAIL"}))

    $control = Egress 1087 'ifconfig.me'
    $ok4 = ($control -match '^\d+\.\d+\.\d+\.\d+$'); $r+=$ok4
    Write-Host ("[{0}] fresh peer -> ALLOWED domain succeeds: {1}" -f $(if($ok4){"PASS"}else{"FAIL"}),$control)

    Write-Host ""
    if($r -contains $false){ Write-Host "OVERALL: FAIL"; exit 1 } else { Write-Host "OVERALL: SUCCESS - reputation gate closes the abuse loop" }
}
finally{
    for($i=$procs.Count-1;$i -ge 0;$i--){ $x=$procs[$i]; if($x -and -not $x.HasExited){ Stop-Process -Id $x.Id -Force -EA SilentlyContinue } }
}
