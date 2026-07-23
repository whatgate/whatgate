# WhatGate end-to-end: ExitGuard protection (M5).
# Verifies an exit refuses traffic it should not serve:
#   1) exit conservative + stranger requester -> DENY
#   2) exit conservative + same-group         -> ALLOW
#   3) exit open + blocked domain             -> DENY
#   4) exit open + allowed domain             -> ALLOW
#
# Prereq: go build -o bin/ ./...
# Run:    pwsh scripts/e2e-exit-guard.ps1

$ErrorActionPreference = 'Stop'
$bin  = Join-Path (Split-Path -Parent $PSScriptRoot) 'bin'
$logs = Join-Path $env:TEMP 'whatgate-e2e'
New-Item -ItemType Directory -Force -Path $logs | Out-Null

Get-Process node,coordinator -EA SilentlyContinue | Stop-Process -Force -EA SilentlyContinue
Start-Sleep -Milliseconds 400

function Wait-ForLine($path,$pattern,$timeoutSec){
    $deadline=(Get-Date).AddSeconds($timeoutSec)
    while((Get-Date) -lt $deadline){
        if(Test-Path $path){
            $m=Select-String -Path $path -Pattern $pattern -EA SilentlyContinue | Select-Object -First 1
            if($m){return $m.Line}
        }
        Start-Sleep -Milliseconds 200
    }
    return $null
}

function Run-Guard($idx,$exitExtra,$clientExtra,$curlHost,$expectSuccess,$label){
    $port=8090+$idx; $eport=45700+$idx; $cport=45720+$idx; $socks=1090+$idx
    $co=Join-Path $logs "eg-co$idx.out"; $eo=Join-Path $logs "eg-e$idx.out"; $cl=Join-Path $logs "eg-c$idx.out"
    Remove-Item $co,"$co.err",$eo,"$eo.err",$cl,"$cl.err" -EA SilentlyContinue
    $procs=@()
    try{
        $p=Start-Process "$bin\coordinator.exe" -ArgumentList '-addr',"127.0.0.1:$port",'-invite','welcome' -RedirectStandardOutput $co -RedirectStandardError "$co.err" -WindowStyle Hidden -PassThru; $procs+=$p
        if(-not (Wait-ForLine $co 'coordinator listening' 10)){ throw "coord $idx not up" }

        $eArgs=@('-listen',"/ip4/127.0.0.1/tcp/$eport",'-coordinator',"http://127.0.0.1:$port",'-invite','welcome','-exit','-region','JP')+$exitExtra
        $pe=Start-Process "$bin\node.exe" -ArgumentList $eArgs -RedirectStandardOutput $eo -RedirectStandardError "$eo.err" -WindowStyle Hidden -PassThru; $procs+=$pe
        if(-not (Wait-ForLine $eo 'exit: ENABLED' 15)){ throw "exit $idx not ready" }
        Start-Sleep -Seconds 2

        $cArgs=@('-listen',"/ip4/127.0.0.1/tcp/$cport",'-coordinator',"http://127.0.0.1:$port",'-invite','welcome','-to','JP','-socks',"127.0.0.1:$socks")+$clientExtra
        $pc=Start-Process "$bin\node.exe" -ArgumentList $cArgs -RedirectStandardOutput $cl -RedirectStandardError "$cl.err" -WindowStyle Hidden -PassThru; $procs+=$pc
        $ready=Wait-ForLine $cl 'SOCKS5 proxy on' 12

        $succeeded=$false; $detail=""
        if($ready){
            $ip=(& curl.exe -s --max-time 15 --socks5-hostname "127.0.0.1:$socks" "https://$curlHost" | Select-Object -First 1)
            $succeeded=[bool]($ip -match '^\d+\.\d+\.\d+\.\d+$')
            $detail=if($succeeded){"egress=$ip"}else{"blocked"}
        } else {
            $detail="client never got an exit"
        }
        $ok=($succeeded -eq $expectSuccess)
        $mark=if($ok){"PASS"}else{"FAIL"}
        Write-Host ("[{0}] {1,-44} curl={2,-14} expect={3,-6} got={4,-6} {5}" -f $mark,$label,$curlHost,$expectSuccess,$succeeded,$detail)
        return $ok
    } finally {
        for($i=$procs.Count-1;$i -ge 0;$i--){ $x=$procs[$i]; if($x -and -not $x.HasExited){ Stop-Process -Id $x.Id -Force -EA SilentlyContinue } }
        Start-Sleep -Milliseconds 300
    }
}

$r=@()
$r += Run-Guard 1 @('-exit-scope','conservative')                          @('-trust-scope','open')                 'api.ipify.org' $false 'exit conservative + stranger -> DENY'
$r += Run-Guard 2 @('-exit-scope','conservative','-group','fam')           @('-trust-scope','open','-group','fam')   'api.ipify.org' $true  'exit conservative + same-group -> ALLOW'
$r += Run-Guard 3 @('-exit-scope','open','-block-domains','api.ipify.org') @('-trust-scope','open')                  'api.ipify.org' $false 'exit open + blocked domain -> DENY'
$r += Run-Guard 4 @('-exit-scope','open','-block-domains','api.ipify.org') @('-trust-scope','open')                  'ifconfig.me'   $true  'exit open + allowed domain -> ALLOW'

Write-Host ""
if($r -contains $false){ Write-Host "OVERALL: FAIL"; exit 1 } else { Write-Host "OVERALL: SUCCESS" }
