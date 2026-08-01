# WhatGate end-to-end: trust-scope filtering (M3).
# Verifies that a client's trust scope changes which exits it may use:
#   1) conservative + stranger exit   -> DENY  (no eligible exit)
#   2) conservative + same-group exit -> ALLOW (egress succeeds)
#   3) open + stranger exit           -> ALLOW
#
# Prereq: go build -o bin/ ./...
# Run:    pwsh scripts/e2e-trust-scope.ps1

$ErrorActionPreference = 'Stop'
$bin  = Join-Path (Split-Path -Parent $PSScriptRoot) 'bin'
$logs = Join-Path $env:TEMP 'whatgate-e2e'
New-Item -ItemType Directory -Force -Path $logs | Out-Null

# Kill strays so ports are free and no ghost exits linger in the directory.
Get-Process whatgate,coordinator -EA SilentlyContinue | Stop-Process -Force -EA SilentlyContinue
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

function Run-Scenario($idx,$scope,$exitGroup,$clientGroup,$expectSuccess,$label){
    $port=8090+$idx; $eport=45700+$idx; $cport=45720+$idx; $socks=1090+$idx
    $co=Join-Path $logs "ts-co$idx.out"; $eo=Join-Path $logs "ts-e$idx.out"; $cl=Join-Path $logs "ts-c$idx.out"
    Remove-Item $co,"$co.err",$eo,"$eo.err",$cl,"$cl.err" -EA SilentlyContinue
    $procs=@()
    try{
        $p=Start-Process "$bin\coordinator.exe" -ArgumentList '-addr',"127.0.0.1:$port",'-invite','welcome' -RedirectStandardOutput $co -RedirectStandardError "$co.err" -WindowStyle Hidden -PassThru; $procs+=$p
        if(-not (Wait-ForLine $co 'coordinator listening' 10)){ throw "coord $idx not up" }

        $eArgs=@('-listen',"/ip4/127.0.0.1/tcp/$eport",'-coordinator',"http://127.0.0.1:$port",'-invite','welcome','-exit','-region','JP')
        if($exitGroup){ $eArgs+=@('-group',$exitGroup,'-group-secret','famkey') }
        $pe=Start-Process "$bin\whatgate.exe" -ArgumentList $eArgs -RedirectStandardOutput $eo -RedirectStandardError "$eo.err" -WindowStyle Hidden -PassThru; $procs+=$pe
        if(-not (Wait-ForLine $eo 'exit: ENABLED' 15)){ throw "exit $idx not ready" }
        Start-Sleep -Seconds 2

        $cArgs=@('-listen',"/ip4/127.0.0.1/tcp/$cport",'-coordinator',"http://127.0.0.1:$port",'-invite','welcome','-to','JP','-trust-scope',$scope,'-socks',"127.0.0.1:$socks")
        if($clientGroup){ $cArgs+=@('-group',$clientGroup,'-group-secret','famkey') }
        $pc=Start-Process "$bin\whatgate.exe" -ArgumentList $cArgs -RedirectStandardOutput $cl -RedirectStandardError "$cl.err" -WindowStyle Hidden -PassThru; $procs+=$pc
        $ready=Wait-ForLine $cl 'SOCKS5 proxy on' 12

        $succeeded=$false; $detail=""
        if($ready){
            $ip=(& curl.exe -s --max-time 15 --socks5-hostname "127.0.0.1:$socks" https://api.ipify.org | Select-Object -First 1)
            $succeeded=[bool]($ip -match '^\d+\.\d+\.\d+\.\d+$')
            $detail=if($succeeded){"egress=$ip"}else{"blocked"}
        } else {
            $why=Wait-ForLine "$cl.err" 'trust scope' 3
            $detail=if($why){"rejected"}else{"no exit"}
        }
        $ok=($succeeded -eq $expectSuccess)
        $mark=if($ok){"PASS"}else{"FAIL"}
        Write-Host ("[{0}] {1,-40} expect={2,-6} got={3,-6} {4}" -f $mark,$label,$expectSuccess,$succeeded,$detail)
        return $ok
    } finally {
        for($i=$procs.Count-1;$i -ge 0;$i--){ $x=$procs[$i]; if($x -and -not $x.HasExited){ Stop-Process -Id $x.Id -Force -EA SilentlyContinue } }
        Start-Sleep -Milliseconds 300
    }
}

$r=@()
$r += Run-Scenario 1 'conservative' ''    ''    $false 'conservative + stranger -> DENY'
$r += Run-Scenario 2 'conservative' 'fam' 'fam' $true  'conservative + same-group -> ALLOW'
$r += Run-Scenario 3 'open'         ''    ''    $true  'open + stranger -> ALLOW'

Write-Host ""
if($r -contains $false){ Write-Host "OVERALL: FAIL"; exit 1 } else { Write-Host "OVERALL: SUCCESS" }
